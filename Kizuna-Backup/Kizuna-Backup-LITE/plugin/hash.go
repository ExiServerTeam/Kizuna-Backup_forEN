package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// SHA256File はファイルの SHA-256 ハッシュを16進文字列で返す
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("ファイルオープン失敗: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("ハッシュ計算失敗: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// SHA256Bytes はバイト列の SHA-256 ハッシュを16進文字列で返す
func SHA256Bytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
