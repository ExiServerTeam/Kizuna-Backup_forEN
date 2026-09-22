package main

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// runRemoteCommand は SSH 経由でリモートコマンドを実行する。
//
// シェル経由ではなく exec.Command で直接 ssh を呼ぶため、
// ローカル側でのシェルインジェクションは発生しない。
// ただし、command 引数自体はリモート側でシェル解釈されるため、
// 呼び出し側で shellQuote を通した安全な文字列のみを渡すこと。
func (b *BackupExecutor) runRemoteCommand(ctx context.Context, command string) (string, error) {
	if b.config.RemoteDest == "" {
		return "", fmt.Errorf("remote_dest が設定されていません")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	args := []string{
		"-p", strconv.Itoa(b.config.SSHPort),
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "ConnectTimeout=10",
	}
	if b.config.SSHKey != "" {
		args = append(args, "-i", b.config.SSHKey)
	}
	args = append(args, b.config.RemoteDest, command)

	cmd := exec.CommandContext(ctx, "ssh", args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("SSH コマンド失敗 (exit=%d): %s",
				ee.ExitCode(), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("SSH 実行失敗: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// remotePathJoin はリモート側（Linux）のパスを結合する。
func remotePathJoin(parts ...string) string {
	var cleaned []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		cleaned = append(cleaned, strings.Trim(p, "/"))
	}
	if len(cleaned) == 0 {
		return "/"
	}
	return "/" + strings.Join(cleaned, "/")
}

// shellQuote はシェルのシングルクォートで安全に囲む。
// グロブ展開したくない文字列（ファイル名、ディレクトリ名）に使う。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// globPatternRe は、グロブパターンとして安全な文字だけで構成されて
// いるかを検証する。英数字、ドット、ハイフン、アンダースコア、
// およびグロブメタ文字の * と ? のみ許可する。
var globPatternRe = regexp.MustCompile(`^[A-Za-z0-9._*?-]+$`)

// rotateRemote はリモート側の世代管理を行う。
//
// リモートに SSH で接続し、指定パターンにマッチするファイルを
// 更新時刻の古い順に削除する。keep <= 0 のときは何もせずスキップする。
// 戻り値は実際に削除した件数。
//
// 注意: pattern はグロブ展開が必要なため、シェルのシングルクォートで
// 囲んではならない。代わりに、pattern が安全な文字だけで構成されて
// いるかを globPatternRe で検証する。
func (b *BackupExecutor) rotateRemote(ctx context.Context, remoteDir, pattern string, keep int) (int, error) {
	if keep <= 0 {
		if b.logger != nil {
			b.logger.Info("remote_rotation_skip",
				"リモート世代管理スキップ (keep <= 0: 無制限)",
				map[string]interface{}{
					"remote_dir": remoteDir,
					"pattern":    pattern,
				})
		}
		return 0, nil
	}

	if !globPatternRe.MatchString(pattern) {
		return 0, fmt.Errorf("不正なグロブパターン: %q", pattern)
	}

	// ls -1t は更新時刻の新しい順にソートする（-t）。
	// tail -n +N で N 行目以降を取得する。
	// keep+1 行目以降 = 削除対象。
	// pattern はグロブ展開が必要なため、シングルクォートで囲まない。
	// 代わりに、上の globPatternRe で安全性を検証済み。
	remoteCmd := fmt.Sprintf(
		`cd %s && ls -1t %s 2>/dev/null | tail -n +%d`,
		shellQuote(remoteDir),
		pattern,
		keep+1,
	)
	out, err := b.runRemoteCommand(ctx, remoteCmd)
	if err != nil {
		return 0, fmt.Errorf("リモート世代管理の対象取得失敗: %w", err)
	}
	if out == "" {
		if b.logger != nil {
			b.logger.Info("remote_rotation_not_needed",
				"リモート世代管理不要 (保持数以内)",
				map[string]interface{}{
					"remote_dir": remoteDir,
					"pattern":    pattern,
					"keep":       keep,
				})
		}
		return 0, nil
	}

	files := strings.Split(out, "\n")
	deleted := 0
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return deleted, err
		}
		fullPath := remotePathJoin(remoteDir, f)
		if _, err := b.runRemoteCommand(ctx, "rm -f "+shellQuote(fullPath)); err != nil {
			return deleted, fmt.Errorf("リモートファイル削除失敗 (%s): %w", fullPath, err)
		}
		deleted++
		if b.logger != nil {
			b.logger.Info("remote_rotate_delete",
				fmt.Sprintf("リモート世代管理で削除: %s", f),
				map[string]interface{}{"path": fullPath})
		}
	}
	return deleted, nil
}
