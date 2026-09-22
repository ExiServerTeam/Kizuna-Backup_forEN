package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// transferToRemote はローカルの .tar.gz を rsync でリモートに転送する。
func (b *BackupExecutor) transferToRemote(ctx context.Context, localPath string) error {
	if b.config.RemoteDest == "" {
		return fmt.Errorf("remote_dest が設定されていません")
	}
	if b.config.RemoteDir == "" {
		return fmt.Errorf("remote_dir が設定されていません")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	remotePath := remotePathJoin(b.config.RemoteDir, filepath.Base(localPath))

	sshParts := []string{
		"ssh",
		"-p", strconv.Itoa(b.config.SSHPort),
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "ConnectTimeout=10",
	}
	if b.config.SSHKey != "" {
		sshParts = append(sshParts, "-i", b.config.SSHKey)
	}
	sshCmd := strings.Join(sshParts, " ")

	args := []string{
		"-avz",
		"--partial",
		"--timeout=600",
		"-e", sshCmd,
		localPath,
		b.config.RemoteDest + ":" + remotePath,
	}

	cmd := exec.CommandContext(ctx, "rsync", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync 失敗: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// verifyRemoteHash はリモート側で sha256sum を実行し、ローカルのハッシュと照合する。
func (b *BackupExecutor) verifyRemoteHash(ctx context.Context, localPath, remotePath string) error {
	localHash, err := SHA256File(localPath)
	if err != nil {
		return fmt.Errorf("ローカルハッシュ計算失敗: %w", err)
	}

	remoteCmd := "sha256sum " + shellQuote(remotePath)
	out, err := b.runRemoteCommand(ctx, remoteCmd)
	if err != nil {
		return fmt.Errorf("リモートハッシュ取得失敗: %w", err)
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return fmt.Errorf("リモートハッシュの出力が空です")
	}
	remoteHash := fields[0]

	if !strings.EqualFold(localHash, remoteHash) {
		return fmt.Errorf("ハッシュ不一致: local=%s remote=%s", localHash, remoteHash)
	}
	return nil
}
