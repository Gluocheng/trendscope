// Package model 定义 TrendScope 的核心数据模型。
package model

import (
	"errors"
	"time"
)

// 包级错误定义。
var (
	ErrInvalidWindow = errors.New("invalid window, must be day|week|month")
)

// Window 表示时间窗口,用于雷达图和仓库榜单的查询范围。
type Window string

const (
	WindowDay   Window = "day"
	WindowWeek  Window = "week"
	WindowMonth Window = "month"
)

// ParseWindow 将字符串解析为 Window,非法值返回 error。
func ParseWindow(s string) (Window, error) {
	switch Window(s) {
	case WindowDay, WindowWeek, WindowMonth:
		return Window(s), nil
	default:
		return "", ErrInvalidWindow
	}
}

// Durations 返回窗口对应的时间跨度(用于计算创建时间过滤起点)。
func (w Window) Durations() time.Duration {
	switch w {
	case WindowDay:
		return 24 * time.Hour
	case WindowWeek:
		return 7 * 24 * time.Hour
	case WindowMonth:
		return 30 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

// AllWindows 返回全部窗口,用于抓取调度。
func AllWindows() []Window { return []Window{WindowDay, WindowWeek, WindowMonth} }

// Repo 表示一次快照中的仓库记录。
type Repo struct {
	ID           int64     `json:"id"`
	FullName     string    `json:"full_name"`
	Owner        string    `json:"owner"`
	Name         string    `json:"name"`
	Stars        int       `json:"stars"`
	Language     string    `json:"language"`
	Description  string    `json:"description"`
	HTMLURL      string    `json:"html_url"`
	CreatedAt    time.Time `json:"created_at"`
	SnapshotTime time.Time `json:"snapshot_time"`
}

// Snapshot 表示一次抓取产生的快照索引。
type Snapshot struct {
	ID        int64     `json:"id"`
	Window    Window    `json:"window"`
	CreatedAt time.Time `json:"created_at"`
}

// LanguageScore 表示某个语言在窗口内的活跃度分数。
type LanguageScore struct {
	Language string  `json:"language"`
	Score    int64   `json:"score"`
	Count    int     `json:"count"`
	AvgStars float64 `json:"avg_stars"`
}
