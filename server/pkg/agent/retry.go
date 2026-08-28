package agent

import (
	"context"
	"errors"
	"math"
	"time"
)

var (
	// ErrNonRetryable 表示该错误不得重试。
	ErrNonRetryable = errors.New("non-retryable")
	// ErrPermissionDenied 表示工具权限被拒绝。
	ErrPermissionDenied = errors.New("permission denied")
	// ErrInvalidArguments 表示工具参数不合法。
	ErrInvalidArguments = errors.New("invalid arguments")
	// ErrApprovalRequired 表示工具需要审批（不是错误，但不可当重试）。
	ErrApprovalRequired = errors.New("approval required")
)

// Retryable 判断错误是否可按重试策略再次尝试。
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, ErrNonRetryable) || errors.Is(err, ErrPermissionDenied) || errors.Is(err, ErrInvalidArguments) || errors.Is(err, ErrApprovalRequired) {
		return false
	}
	return true
}

// Backoff 计算第 attempt 次失败后的等待时间。attempt 从 1 开始。
func Backoff(cfg RetryConfig, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := cfg.InitialBackoff
	if delay <= 0 {
		delay = time.Millisecond
	}
	if cfg.Multiplier <= 0 {
		return delay
	}
	factor := math.Pow(cfg.Multiplier, float64(attempt-1))
	delay = time.Duration(float64(delay) * factor)
	if cfg.MaxBackoff > 0 && delay > cfg.MaxBackoff {
		delay = cfg.MaxBackoff
	}
	return delay
}

// ShouldRetry 判断是否还能再试一次。attempt 为即将进行的尝试次数（从 1 开始）。
func ShouldRetry(cfg RetryConfig, attempt int, err error) bool {
	if !Retryable(err) {
		return false
	}
	max := cfg.MaxAttempts
	if max <= 0 {
		max = 1
	}
	return attempt < max
}
