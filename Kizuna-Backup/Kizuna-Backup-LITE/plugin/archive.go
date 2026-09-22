package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CreateArchive は targetDir を tar.gz に圧縮する
// 戻り値: 生成したファイルのサイズ（バイト）
func CreateArchive(targetDir, outputPath string) (int64, error) {
	info, err := os.Stat(targetDir)
	if err != nil {
		return 0, fmt.Errorf("対象ディレクトリ確認失敗: %w", err)
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("対象はディレクトリではありません: %s", targetDir)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("出力ファイル作成失敗: %w", err)
	}

	var totalSize int64
	func() {
		defer out.Close()

		gzw := gzip.NewWriter(out)
		defer gzw.Close()

		tw := tar.NewWriter(gzw)
		defer tw.Close()

		parent := filepath.Dir(targetDir)

		err = filepath.Walk(targetDir, func(path string, fi os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			rel, err := filepath.Rel(parent, path)
			if err != nil {
				return err
			}

			hdr, err := tar.FileInfoHeader(fi, "")
			if err != nil {
				return err
			}
			hdr.Name = rel
			hdr.ModTime = fi.ModTime()

			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}

			if fi.IsDir() {
				return nil
			}

			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()

			n, err := io.Copy(tw, f)
			if err != nil {
				return err
			}
			totalSize += n
			return nil
		})
	}()

	if err != nil {
		return 0, fmt.Errorf("アーカイブ作成失敗: %w", err)
	}

	// 圧縮後の実際のサイズを取得
	if stat, statErr := os.Stat(outputPath); statErr == nil {
		totalSize = stat.Size()
	}

	return totalSize, nil
}

// formatBytes は人間が読みやすい単位に変換する
func formatBytes(b int64) string {
	const k = 1024
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v := float64(b)
	i := 0
	for v >= k && i < len(units)-1 {
		v /= k
		i++
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}
