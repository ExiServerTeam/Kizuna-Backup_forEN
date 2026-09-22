package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// ScheduleMode はスケジューリング方式を表す。
type ScheduleMode int

const (
	ScheduleModeInterval ScheduleMode = iota // interval_sec による固定間隔
	ScheduleModeCron                         // schedule_expr による cron 式
)

// Schedule は次回実行時刻の判定を行う。
//
// 使い方:
//  1. NewSchedule で生成
//  2. 起動直後に Reset(now) を呼ぶ（次回予定を計算）
//  3. 定期実行ループで ShouldRun(now) を呼ぶ
//  4. 手動実行した場合は MarkRun(now) を呼ぶ
type Schedule struct {
	mode     ScheduleMode
	interval time.Duration
	schedule cron.Schedule

	mu      sync.RWMutex
	lastRun time.Time
	nextRun time.Time
	started bool
}

// cronParser は 5 フィールドの標準 cron 式を受け付けるパーサ。
var cronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
)

// NewSchedule は設定から Schedule を生成する。
// scheduleExpr が空やったら interval モード、そうでなければ cron モード。
func NewSchedule(intervalSec int, scheduleExpr string) (*Schedule, error) {
	s := &Schedule{}

	if strings.TrimSpace(scheduleExpr) != "" {
		parsed, err := cronParser.Parse(scheduleExpr)
		if err != nil {
			return nil, fmt.Errorf("schedule_expr の解析失敗 (%q): %w", scheduleExpr, err)
		}
		s.mode = ScheduleModeCron
		s.schedule = parsed
		return s, nil
	}

	if intervalSec <= 0 {
		return nil, fmt.Errorf("interval_sec は 1 以上を指定してください: %d", intervalSec)
	}
	s.mode = ScheduleModeInterval
	s.interval = time.Duration(intervalSec) * time.Second
	return s, nil
}

// Mode は現在のモードを返す。
func (s *Schedule) Mode() ScheduleMode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

// Reset はスケジュール状態を初期化する。
// 起動直後に呼ぶことで「次回実行予定」を now 基準で再計算する。
// RunOnStart の実行可否とは独立して呼べる。
//
// Reset を呼んだ後、ShouldRun は「次の予定時刻」を基準に判定する。
// Reset を呼ばん場合、ShouldRun の初回呼び出しで暗黙的に初期化される。
func (s *Schedule) Reset(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = true
	s.lastRun = now
	if s.mode == ScheduleModeCron {
		s.nextRun = s.schedule.Next(now)
	} else {
		s.nextRun = now.Add(s.interval)
	}
}

// ShouldRun は「今この瞬間に実行すべきか」を判定する。
//
// interval モード:
//   - 初回呼び出しで true を返す（従来の挙動を維持）
//   - 前回実行から interval 以上経過しとれば true
//
// cron モード:
//   - Reset 未呼び出しの場合、初回は false を返して次回予定だけ計算
//   - Reset 呼び出し済みの場合、次回予定時刻を過ぎとれば true
func (s *Schedule) ShouldRun(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.mode {
	case ScheduleModeInterval:
		if s.lastRun.IsZero() {
			s.lastRun = now
			s.nextRun = now.Add(s.interval)
			return true
		}
		if now.Sub(s.lastRun) >= s.interval {
			s.lastRun = now
			s.nextRun = now.Add(s.interval)
			return true
		}
		return false

	case ScheduleModeCron:
		if !s.started {
			// 暗黙の初回処理: Reset が呼ばれとらん場合のフォールバック
			s.started = true
			s.lastRun = now
			s.nextRun = s.schedule.Next(now)
			return false
		}
		if s.nextRun.IsZero() {
			s.nextRun = s.schedule.Next(s.lastRun)
		}
		if !now.Before(s.nextRun) {
			s.lastRun = now
			s.nextRun = s.schedule.Next(now)
			return true
		}
		return false
	}
	return false
}

// MarkRun は「実行した」ことを明示的に記録する。
// 手動実行時に呼ぶことで、次回予定時刻を正しく更新できる。
//
// MarkRun を単独で呼んでも、以降の ShouldRun が正しく動くよう、
// started フラグも同時に設定する。
func (s *Schedule) MarkRun(at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = true
	s.lastRun = at
	switch s.mode {
	case ScheduleModeCron:
		s.nextRun = s.schedule.Next(at)
	case ScheduleModeInterval:
		s.nextRun = at.Add(s.interval)
	}
}

// NextRun は次回実行予定時刻を返す。
// interval モードでも次回予定を返すさかい、ダッシュボード表示に使える。
func (s *Schedule) NextRun() time.Time {
	// ロックの外で time.Now() を取得（ロック保持時間を最小化）
	now := time.Now()

	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.nextRun.IsZero() {
		return s.nextRun
	}
	base := s.lastRun
	if base.IsZero() {
		base = now
	}
	if s.mode == ScheduleModeCron {
		return s.schedule.Next(base)
	}
	return base.Add(s.interval)
}

// Describe は人間可読な説明を返す。
func (s *Schedule) Describe() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	switch s.mode {
	case ScheduleModeCron:
		return "cron式"
	default:
		return fmt.Sprintf("固定間隔 %s", formatInterval(s.interval))
	}
}

// formatInterval は秒単位の interval を人間可読な形式に変換する。
func formatInterval(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%.1f日", d.Hours()/24)
	case d >= time.Hour:
		return fmt.Sprintf("%.1f時間", d.Hours())
	case d >= time.Minute:
		return fmt.Sprintf("%.1f分", d.Minutes())
	default:
		return fmt.Sprintf("%d秒", int(d.Seconds()))
	}
}
