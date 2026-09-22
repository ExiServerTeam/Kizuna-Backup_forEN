package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"Kizuna-Eye/pkg/module"
)

// ============================================================
// BackupExecutor はバックアップ実行を統括する
// ============================================================
type BackupExecutor struct {
	config *LiteConfig
	logger *FileLogger

	mu         sync.RWMutex
	status     *module.BackupStatus
	lastResult *BackupResult
}

// BackupResult はこのパッケージ内で使う実行結果
type BackupResult struct {
	StartedAt  time.Time
	FinishedAt time.Time
	Status     string
	Size       int64
	Error      string
}

// NewBackupExecutor は BackupExecutor を生成する
func NewBackupExecutor(cfg *LiteConfig, logger *FileLogger) *BackupExecutor {
	return &BackupExecutor{
		config: cfg,
		logger: logger,
		status: &module.BackupStatus{
			Name:   pluginName,
			Status: "unknown",
		},
	}
}

// Run はバックアップを1回実行する
//
// TODO(Phase 8-2以降): running状態のタイムアウト保護。
// 現状は status.Status == "running" のとき無条件に拒否するが、
// プロセスがクラッシュして running が残ると永久に実行不能になる。
// module.BackupStatus に StartedAt を追加し、一定時間を超えたら
// 異常終了とみなして running を解除する仕組みを入れること。
func (b *BackupExecutor) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	b.mu.Lock()
	if b.status != nil && b.status.Status == "running" {
		b.mu.Unlock()
		return fmt.Errorf("バックアップ実行中")
	}
	b.status = &module.BackupStatus{
		Name:   pluginName,
		Status: "running",
	}
	b.mu.Unlock()

	startedAt := time.Now()
	if b.logger != nil {
		b.logger.Info("start", "バックアップ開始", map[string]interface{}{
			"target": b.config.TargetDir,
			"mode":   b.config.Mode,
		})
	}

	result := &BackupResult{StartedAt: startedAt}

	// DRY-RUN
	if b.config.DryRun {
		if b.logger != nil {
			b.logger.Info("dry_run", "DRY-RUN モード: 実際の処理は行いません", nil)
		}
		select {
		case <-time.After(1 * time.Second):
		case <-ctx.Done():
			result.FinishedAt = time.Now()
			result.Status = "failed"
			result.Error = ctx.Err().Error()
			b.finish(result)
			return ctx.Err()
		}
		result.FinishedAt = time.Now()
		result.Status = "success"
		b.finish(result)
		return nil
	}

	var execErr error
	switch b.config.Mode {
	case "archive":
		execErr = b.runArchive(ctx, result)
	case "sync":
		execErr = b.runSync(ctx, result)
	default:
		execErr = fmt.Errorf("不明なモード: %s", b.config.Mode)
	}

	result.FinishedAt = time.Now()

	if execErr != nil {
		result.Status = "failed"
		result.Error = execErr.Error()
		if b.logger != nil {
			b.logger.Error("failed", fmt.Sprintf("バックアップ失敗: %v", execErr), nil)
		}
		b.finish(result)
		return execErr
	}

	result.Status = "success"
	if b.logger != nil {
		b.logger.Info("success",
			fmt.Sprintf("バックアップ成功 (所要時間 %v)",
				result.FinishedAt.Sub(result.StartedAt).Round(time.Second)),
			map[string]interface{}{
				"size":         result.Size,
				"duration_sec": result.FinishedAt.Sub(result.StartedAt).Seconds(),
			})
	}
	b.finish(result)
	return nil
}

// runArchive は tar.gz 圧縮 → SHA-256 計算 → リモート転送 → 世代管理 までを行う
//
// 世代管理でエラーが出ても、バックアップ本体は成功として扱う。
// 理由: 圧縮とハッシュ計算が完了した時点で、アーカイブは既に完全な状態で存在する。
// 古いファイル1個の削除失敗を理由に「バックアップ失敗」と報告すると、
// 運用者に誤ったシグナルを送ることになる。
func (b *BackupExecutor) runArchive(ctx context.Context, result *BackupResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	ts := time.Now().Format("20060102_150405")
	base := filepath.Base(strings.TrimRight(b.config.TargetDir, "/"))
	if base == "" || base == "." {
		base = "backup"
	}
	filename := fmt.Sprintf("%s_%s.tar.gz", base, ts)
	outputPath := filepath.Join(b.config.LocalTempDir, filename)
	archiveComplete := false
	defer func() {
		if !archiveComplete {
			_ = os.Remove(outputPath)
		}
	}()

	if b.logger != nil {
		b.logger.Info("archive_start", "圧縮開始", map[string]interface{}{
			"target": b.config.TargetDir,
			"output": outputPath,
		})
	}

	size, err := CreateArchive(b.config.TargetDir, outputPath)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	hash, err := SHA256File(outputPath)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// サイズを確定してから、アーカイブ完成とみなす。
	// この順序により、「サイズはあるのにファイルがない」という
	// 矛盾した状態が生まれない。
	result.Size = size
	archiveComplete = true

	if b.logger != nil {
		b.logger.Info("archive_complete",
			fmt.Sprintf("圧縮完了: %s", formatBytes(size)),
			map[string]interface{}{
				"size":   size,
				"sha256": hash,
				"path":   outputPath,
			})
	}

	// ★ Phase 8-2: リモート転送
	transferred := false
	if b.config.RemoteDest != "" && b.config.RemoteDir != "" {
		if b.logger != nil {
			b.logger.Info("transfer_start", "転送開始", map[string]interface{}{
				"dest": b.config.RemoteDest,
				"dir":  b.config.RemoteDir,
			})
		}
		if err := b.transferToRemote(ctx, outputPath); err != nil {
			return fmt.Errorf("転送失敗: %w", err)
		}
		remotePath := remotePathJoin(b.config.RemoteDir, filepath.Base(outputPath))
		if err := b.verifyRemoteHash(ctx, outputPath, remotePath); err != nil {
			return fmt.Errorf("ハッシュ検証失敗: %w", err)
		}
		transferred = true
		if b.logger != nil {
			b.logger.Info("transfer_complete", "転送完了", map[string]interface{}{
				"dest": b.config.RemoteDest,
				"path": remotePath,
			})
		}
	}

	// ★ Phase 8-3: ローカル世代管理
	// バックアップ本体の成功と世代管理の成功は別レイヤとして扱う。
	// 世代管理でエラーが出ても、バックアップ全体を失敗扱いにはしない。
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.config.RotationKeep > 0 {
		pattern := fmt.Sprintf("%s_*.tar.gz", base)
		deletedCount, rotErr := b.rotateLocal(b.config.LocalTempDir, pattern, b.config.RotationKeep)
		if rotErr != nil {
			if b.logger != nil {
				b.logger.Error("rotation_failed",
					fmt.Sprintf("世代管理失敗 (バックアップは成功): %v", rotErr),
					map[string]interface{}{
						"dir":     b.config.LocalTempDir,
						"pattern": pattern,
						"keep":    b.config.RotationKeep,
					})
			}
		} else if b.logger != nil {
			b.logger.Info("rotation_complete",
				fmt.Sprintf("世代管理完了: 保持数 %d / 削除件数 %d", b.config.RotationKeep, deletedCount),
				map[string]interface{}{
					"dir":           b.config.LocalTempDir,
					"pattern":       pattern,
					"keep":          b.config.RotationKeep,
					"deleted_count": deletedCount,
				})
		}
	}

	// ★ Phase 8-2: リモート世代管理
	if transferred && b.config.RotationKeep > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		pattern := fmt.Sprintf("%s_*.tar.gz", base)
		deletedCount, rotErr := b.rotateRemote(ctx, b.config.RemoteDir, pattern, b.config.RotationKeep)
		if rotErr != nil {
			if b.logger != nil {
				b.logger.Error("remote_rotation_failed",
					fmt.Sprintf("リモート世代管理失敗 (バックアップは成功): %v", rotErr),
					map[string]interface{}{
						"remote_dir": b.config.RemoteDir,
						"pattern":    pattern,
						"keep":       b.config.RotationKeep,
					})
			}
		} else if b.logger != nil {
			b.logger.Info("remote_rotation_complete",
				fmt.Sprintf("リモート世代管理完了: 保持数 %d / 削除件数 %d", b.config.RotationKeep, deletedCount),
				map[string]interface{}{
					"remote_dir":    b.config.RemoteDir,
					"pattern":       pattern,
					"keep":          b.config.RotationKeep,
					"deleted_count": deletedCount,
				})
		}
	}

	// ★ Phase 8-2: keep_local=false かつ転送成功時はローカルアーカイブを削除
	if transferred && !b.config.KeepLocal {
		if err := os.Remove(outputPath); err != nil {
			if b.logger != nil {
				b.logger.Error("local_cleanup_failed",
					fmt.Sprintf("ローカルアーカイブ削除失敗: %v", err),
					map[string]interface{}{"path": outputPath})
			}
		} else if b.logger != nil {
			b.logger.Info("local_cleanup", "ローカルアーカイブ削除",
				map[string]interface{}{"path": outputPath})
		}
	}

	return nil
}

// runSync は rsync による差分同期（Phase 8-4 で実装予定）
//
// 引数の ctx と result は、将来の実装で使う予定。
// 現時点では未使用だが、シグネチャを固定しておく。
func (b *BackupExecutor) runSync(ctx context.Context, result *BackupResult) error {
	_ = ctx    // 将来の実装で使う
	_ = result // 将来の実装で使う
	return fmt.Errorf("sync モードは未実装です")
}

// finish は実行結果を status に反映する
func (b *BackupExecutor) finish(result *BackupResult) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastResult = result
	b.status = &module.BackupStatus{
		Name:    pluginName,
		LastRun: result.FinishedAt.Format(time.RFC3339),
		Status:  result.Status,
		Size:    result.Size,
	}
}

// GetStatus は最新のステータスを返す
//
// 読み取り専用の操作やさかい、RLock() を使う。
// Dashboard や監視機能から頻繁に呼ばれる可能性があるため、
// 書き込みロックと分離することで並行性を高める。
func (b *BackupExecutor) GetStatus() *module.BackupStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.status == nil {
		return &module.BackupStatus{Name: pluginName, Status: "unknown"}
	}
	copied := *b.status
	return &copied
}

// rotateLocal はローカルファイルの世代管理（直近N個保持）
//
// keep <= 0 のときは「無制限」とみなし、何もせずスキップする。
// スキップしたことをログに残すことで、後から
// 「世代管理が動いていない」と誤解されるのを防ぐ。
//
// 戻り値の deletedCount は、実際に削除したファイルの件数を返す。
// ログに deleted_count として記録することで、後から
// 世代管理の動作を検証しやすくする。
func (b *BackupExecutor) rotateLocal(dir, pattern string, keep int) (int, error) {
	if keep <= 0 {
		if b.logger != nil {
			b.logger.Info("rotation_skip",
				"世代管理スキップ (keep <= 0: 無制限)",
				map[string]interface{}{
					"dir":     dir,
					"pattern": pattern,
				})
		}
		return 0, nil
	}

	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return 0, fmt.Errorf("世代管理対象の検索失敗: %w", err)
	}
	if len(matches) <= keep {
		if b.logger != nil {
			b.logger.Info("rotation_not_needed",
				"世代管理不要 (保持数以内)",
				map[string]interface{}{
					"dir":     dir,
					"pattern": pattern,
					"keep":    keep,
					"count":   len(matches),
				})
		}
		return 0, nil
	}

	// ファイル名に 20060102_150405 形式の固定幅タイムスタンプが入っとるため、
	// 辞書順ソート = 時系列順ソート になる。
	// ModTime() はコピーや移動で狂う可能性があるので使わない。
	sort.Strings(matches)
	toDelete := matches[:len(matches)-keep]

	deletedCount := 0
	for _, m := range toDelete {
		if b.logger != nil {
			b.logger.Info("rotate_delete",
				fmt.Sprintf("世代管理で削除: %s", filepath.Base(m)),
				map[string]interface{}{
					"path": m,
				})
		}
		if err := os.Remove(m); err != nil {
			return deletedCount, fmt.Errorf("世代管理ファイル削除失敗 (%s): %w", m, err)
		}
		deletedCount++
	}
	return deletedCount, nil
}
