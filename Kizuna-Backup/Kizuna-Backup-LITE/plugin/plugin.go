package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"Kizuna-Eye/pkg/module"
)

const pluginName = "kizuna_backup_lite"

type BackupLitePlugin struct {
	logger  module.Logger
	fileLog *FileLogger

	mu       sync.RWMutex
	config   *LiteConfig
	executor *BackupExecutor
	schedule *Schedule

	ctx    context.Context
	cancel context.CancelFunc

	initialRunDone bool
}

func NewPluginModule(logger module.Logger) module.PluginModule {
	return &BackupLitePlugin{
		logger: logger,
	}
}

func (p *BackupLitePlugin) Name() string { return pluginName }

func (p *BackupLitePlugin) DisplayName() string {
	return "Kizuna-Backup LITE"
}

func (p *BackupLitePlugin) Description() string {
	return "Kizuna-Backup LITE バックアッププラグイン"
}

func (p *BackupLitePlugin) Init(ctx context.Context) error {
	p.ctx, p.cancel = context.WithCancel(ctx)
	if p.logger != nil {
		p.logger.Info("Kizuna-Backup LITE プラグイン初期化")
	}
	return nil
}

func (p *BackupLitePlugin) Interval() time.Duration {
	return 30 * time.Second
}

func (p *BackupLitePlugin) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	p.mu.RLock()
	cfg := p.config
	exec := p.executor
	sched := p.schedule
	done := p.initialRunDone
	p.mu.RUnlock()

	if exec == nil || sched == nil {
		return fmt.Errorf("プラグインが初期化されていません")
	}

	now := time.Now()

	if !done {
		p.mu.Lock()
		p.initialRunDone = true
		p.mu.Unlock()

		sched.Reset(now)

		if cfg.RunOnStart {
			return exec.Run(ctx)
		}
		return nil
	}

	if !sched.ShouldRun(now) {
		return nil
	}

	return exec.Run(ctx)
}

func (p *BackupLitePlugin) Configure(config interface{}) error {
	raw, ok := config.(map[string]interface{})
	if !ok {
		return fmt.Errorf("設定型が不正です: map[string]interface{} が必要")
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		return err
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("設定検証失敗: %w", err)
	}

	sched, err := NewSchedule(cfg.IntervalSec, cfg.ScheduleExpr)
	if err != nil {
		return fmt.Errorf("スケジュール初期化失敗: %w", err)
	}

	fl, err := NewFileLogger(cfg.LogPath)
	if err != nil {
		return err
	}

	p.mu.Lock()
	p.config = cfg
	p.fileLog = fl
	p.executor = NewBackupExecutor(cfg, fl)
	p.schedule = sched
	p.initialRunDone = false
	p.mu.Unlock()

	fl.Info("configured", "設定を反映しました", map[string]interface{}{
		"mode":          cfg.Mode,
		"target_dir":    cfg.TargetDir,
		"interval_sec":  cfg.IntervalSec,
		"schedule_expr": cfg.ScheduleExpr,
		"schedule_mode": sched.Describe(),
	})

	if p.logger != nil {
		p.logger.Info("Kizuna-Backup LITE 設定完了: mode=%s target=%s interval=%ds schedule=%s",
			cfg.Mode, cfg.TargetDir, cfg.IntervalSec, sched.Describe())
	}
	return nil
}

func (p *BackupLitePlugin) Start(ctx context.Context) error {
	if p.logger != nil {
		p.logger.Info("Kizuna-Backup LITE プラグイン起動")
	}
	return nil
}

func (p *BackupLitePlugin) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}
	p.mu.Lock()
	if p.fileLog != nil {
		_ = p.fileLog.Close()
	}
	p.mu.Unlock()

	if p.logger != nil {
		p.logger.Info("Kizuna-Backup LITE プラグイン停止")
	}
	return nil
}

func (p *BackupLitePlugin) Health(ctx context.Context) error {
	return nil
}

func (p *BackupLitePlugin) HealthInterval() time.Duration {
	return 60 * time.Second
}

func (p *BackupLitePlugin) Shutdown(ctx context.Context) error {
	return p.Stop()
}

func (p *BackupLitePlugin) GetBackupStatus() *module.BackupStatus {
	p.mu.RLock()
	exec := p.executor
	sched := p.schedule
	p.mu.RUnlock()

	if exec == nil {
		return &module.BackupStatus{Name: pluginName, Status: "unknown"}
	}
	st := exec.GetStatus()
	if sched != nil {
		if next := sched.NextRun(); !next.IsZero() {
			st.NextRun = next.Format(time.RFC3339)
		}
	}
	return st
}

func (p *BackupLitePlugin) RunBackup(ctx context.Context) error {
	p.mu.RLock()
	exec := p.executor
	sched := p.schedule
	p.mu.RUnlock()

	if exec == nil {
		return fmt.Errorf("プラグインが初期化されていません")
	}
	if sched != nil {
		sched.MarkRun(time.Now())
	}
	return exec.Run(ctx)
}

func (p *BackupLitePlugin) GetConfigFields() []module.ConfigField {
	minInterval := 10.0
	maxInterval := 86400.0
	minRotation := 0.0
	maxRotation := 1000.0
	minSSHPort := 1.0
	maxSSHPort := 65535.0

	return []module.ConfigField{
		{Key: "mode", Label: "バックアップモード", Type: module.FieldSelect,
			Options: []string{"archive", "sync"}, Default: "archive", Required: true,
			Group: "基本設定", Hint: "archive: tar.gz形式で圧縮 / sync: rsyncで同期します。"},
		{Key: "target_dir", Label: "対象ディレクトリ", Type: module.FieldText,
			Required: true, Placeholder: "/home/user/data", Group: "基本設定",
			Hint: "バックアップ対象のディレクトリを絶対パスで指定してください。"},
		{Key: "interval_sec", Label: "実行間隔（秒）", Type: module.FieldNumber,
			Default: "3600", Required: false, Min: &minInterval, Max: &maxInterval,
			Hint: "schedule_expr が空の場合のみ有効です。", Group: "基本設定"},
		{Key: "schedule_expr", Label: "cron式（上級者向け）", Type: module.FieldText,
			Placeholder: "0 3 * * *", Required: false, Group: "基本設定",
			Hint: "指定するとinterval_secより優先されます。例: 毎日3時 = 0 3 * * *"},
		{Key: "run_on_start", Label: "起動時に実行", Type: module.FieldCheckbox,
			Default: "false", Group: "基本設定",
			Hint: "有効にすると、プラグイン起動時に一度バックアップを実行します。"},
		{Key: "local_temp_dir", Label: "一時ファイル保存先", Type: module.FieldText,
			Default: "/tmp", Required: true, Group: "詳細設定",
			Hint: "アーカイブを一時的に保存するディレクトリを指定してください。"},
		{Key: "log_path", Label: "ログファイルパス", Type: module.FieldText,
			Default: "./logs/kizuna-backup-lite.log", Group: "詳細設定",
			Hint: "バックアップ実行ログの出力先を指定します。"},
		{Key: "keep_local", Label: "ローカルに保持", Type: module.FieldCheckbox,
			Default: "false", Group: "詳細設定",
			Hint: "有効にするとローカルにもアーカイブを残せます。"},
		{Key: "rotation_keep", Label: "世代保持数", Type: module.FieldNumber,
			Default: "5", Min: &minRotation, Max: &maxRotation, Group: "詳細設定",
			Hint: "0 を指定すると無制限になります。"},
		{Key: "dry_run", Label: "DRY-RUNモード", Type: module.FieldCheckbox,
			Default: "false", Group: "詳細設定",
			Hint: "有効にすると実際のファイル操作を行わずにテストが可能です。"},
		{Key: "remote_dest", Label: "転送先ホスト", Type: module.FieldText,
			Placeholder: "user@192.168.0.100", Group: "SSH転送",
			Hint: "モードが sync の場合のみ必須です。"},
		{Key: "remote_dir", Label: "転送先ディレクトリ", Type: module.FieldText,
			Placeholder: "/backup/kizuna", Group: "SSH転送",
			Hint: "転送先のディレクトリを絶対パスで指定してください。"},
		{Key: "ssh_port", Label: "SSHポート", Type: module.FieldNumber,
			Default: "22", Min: &minSSHPort, Max: &maxSSHPort, Group: "SSH転送",
			Hint: "通常は 22 のままで問題ありません。"},
		{Key: "ssh_key", Label: "SSH秘密鍵パス", Type: module.FieldPassword,
			Placeholder: "/home/user/.ssh/id_ed25519", Group: "SSH転送",
			Hint: "パスワード認証の場合は空欄のままにしてください。"},
	}
}

var Plugin = &BackupLitePlugin{}

func main() {
	_ = Plugin
	_ = Plugin.GetConfigFields
	_ = Plugin.DisplayName
	_ = Plugin.Name
	_ = Plugin.Description
	_ = Plugin.Interval
	_ = Plugin.RunBackup
}
