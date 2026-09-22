package main

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

var remoteDestRe = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+$`)

// LiteConfig は Kizuna-Backup LITE プラグインの設定を表す。
type LiteConfig struct {
	Mode         string
	TargetDir    string
	RemoteDest   string
	RemoteDir    string
	SSHKey       string
	SSHPort      int
	LogPath      string
	LocalTempDir string
	KeepLocal    bool
	RotationKeep int
	RunOnStart   bool
	IntervalSec  int
	ScheduleExpr string
	DryRun       bool
}

// DefaultConfig はデフォルト値を埋めた LiteConfig を返す。
func DefaultConfig() *LiteConfig {
	return &LiteConfig{
		Mode:         "archive",
		SSHPort:      22,
		LogPath:      "./logs/kizuna-backup-lite.log",
		LocalTempDir: "/tmp",
		RotationKeep: 5,
		RunOnStart:   false,
		IntervalSec:  3600,
		ScheduleExpr: "",
		DryRun:       false,
	}
}

// ParseConfig は modules.json の config マップを LiteConfig に変換する。
func ParseConfig(raw map[string]interface{}) (*LiteConfig, error) {
	c := DefaultConfig()

	if v, ok := raw["mode"].(string); ok && v != "" {
		c.Mode = v
	}
	if v, ok := raw["target_dir"].(string); ok {
		c.TargetDir = v
	}
	if v, ok := raw["remote_dest"].(string); ok {
		c.RemoteDest = v
	}
	if v, ok := raw["remote_dir"].(string); ok {
		c.RemoteDir = v
	}
	if v, ok := raw["ssh_key"].(string); ok {
		c.SSHKey = v
	}
	if v, ok := raw["ssh_port"].(float64); ok {
		c.SSHPort = int(v)
	}
	if v, ok := raw["log_path"].(string); ok && v != "" {
		c.LogPath = v
	}
	if v, ok := raw["local_temp_dir"].(string); ok && v != "" {
		c.LocalTempDir = v
	}
	if v, ok := raw["keep_local"].(bool); ok {
		c.KeepLocal = v
	}
	if v, ok := raw["rotation_keep"].(float64); ok {
		c.RotationKeep = int(v)
	}
	if v, ok := raw["run_on_start"].(bool); ok {
		c.RunOnStart = v
	}
	if v, ok := raw["interval_sec"].(float64); ok {
		c.IntervalSec = int(v)
	}
	if v, ok := raw["schedule_expr"].(string); ok {
		c.ScheduleExpr = v
	}
	if v, ok := raw["dry_run"].(bool); ok {
		c.DryRun = v
	}

	return c, nil
}

// Validate は設定の整合性を検査する。
func (c *LiteConfig) Validate() error {
	if c.TargetDir == "" {
		return fmt.Errorf("target_dir は必須です")
	}
	if c.SSHPort < 1 || c.SSHPort > 65535 {
		return fmt.Errorf("ssh_port は 1〜65535 の範囲で指定してください: %d", c.SSHPort)
	}

	hasCron := strings.TrimSpace(c.ScheduleExpr) != ""
	hasInterval := c.IntervalSec > 0
	if !hasCron && !hasInterval {
		return fmt.Errorf("interval_sec または schedule_expr のどちらかを指定してください")
	}
	if hasCron {
		if _, err := cronParser.Parse(c.ScheduleExpr); err != nil {
			return fmt.Errorf("schedule_expr の形式が不正です (%q): %w", c.ScheduleExpr, err)
		}
	} else {
		if c.IntervalSec < 10 || c.IntervalSec > 86400 {
			return fmt.Errorf("interval_sec は 10〜86400 の範囲で指定してください: %d", c.IntervalSec)
		}
	}

	switch c.Mode {
	case "archive":
		if c.LocalTempDir == "" {
			return fmt.Errorf("local_temp_dir は archive モードで必須です")
		}
	case "sync":
		if c.RemoteDest == "" {
			return fmt.Errorf("sync モードでは remote_dest が必須です")
		}
		if c.RemoteDir == "" {
			return fmt.Errorf("sync モードでは remote_dir が必須です")
		}
	default:
		return fmt.Errorf("mode は archive または sync を指定してください: %q", c.Mode)
	}

	if c.RemoteDest != "" {
		if !remoteDestRe.MatchString(c.RemoteDest) {
			return fmt.Errorf("remote_dest は user@host 形式で指定してください: %s", c.RemoteDest)
		}
	}
	if c.RemoteDir != "" {
		if !strings.HasPrefix(c.RemoteDir, "/") {
			return fmt.Errorf("remote_dir は絶対パスで指定してください: %s", c.RemoteDir)
		}
		c.RemoteDir = path.Clean(c.RemoteDir)
	}
	if c.SSHKey != "" {
		if !strings.HasPrefix(c.SSHKey, "/") {
			return fmt.Errorf("ssh_key は絶対パスで指定してください: %s", c.SSHKey)
		}
	}

	return nil
}
