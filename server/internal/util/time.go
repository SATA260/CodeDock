package util

import "time"

const (
	TimeRFC3339 = time.RFC3339
	TimeClock   = "15:04:05.000"
)

// Now 返回当前 UTC 时间。
func Now() time.Time {
	return time.Now().UTC()
}

// FormatTime 按 RFC3339 格式化时间。
func FormatTime(t time.Time) string {
	return t.UTC().Format(TimeRFC3339)
}

// FormatClock 按日志时钟格式格式化时间。
func FormatClock(t time.Time) string {
	return t.UTC().Format(TimeClock)
}
