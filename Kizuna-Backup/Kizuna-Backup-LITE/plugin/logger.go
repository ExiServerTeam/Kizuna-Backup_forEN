package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ============================================================
// FileLogger は JSON Lines 形式でログを出力する
//
// 出力例:
//
//	{"ts":"2026-09-17T13:00:00+09:00","level":"INFO","event":"start","message":"バックアップ開始"}
//
// ============================================================
type FileLogger struct {
	mu   sync.Mutex
	file *os.File
	path string
}

func NewFileLogger(path string) (*FileLogger, error) {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("ログディレクトリ作成失敗: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("ログファイルオープン失敗: %w", err)
	}
	return &FileLogger{file: f, path: path}, nil
}

// log は1行の JSON を書き込む
func (l *FileLogger) log(level, event, message string, extra map[string]interface{}) {
	entry := map[string]interface{}{
		"ts":      time.Now().Format(time.RFC3339),
		"level":   level,
		"event":   event,
		"message": message,
	}
	for k, v := range extra {
		entry[k] = v
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.file.Write(append(data, '\n'))
}

func (l *FileLogger) Info(event, message string, extra map[string]interface{}) {
	l.log("INFO", event, message, extra)
}

func (l *FileLogger) Warn(event, message string, extra map[string]interface{}) {
	l.log("WARN", event, message, extra)
}

func (l *FileLogger) Error(event, message string, extra map[string]interface{}) {
	l.log("ERROR", event, message, extra)
}

func (l *FileLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
