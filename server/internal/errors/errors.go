package errors

import (
	stderrors "errors"
	"fmt"
)

var (
	ErrNotFound     = stderrors.New("not found")
	ErrConflict     = stderrors.New("conflict")
	ErrInvalid      = stderrors.New("invalid")
	ErrUnavailable  = stderrors.New("unavailable")
	ErrUnauthorized = stderrors.New("unauthorized")
)

// New 用统一错误类型包装说明。
func New(kind error, format string, args ...any) error {
	if format == "" {
		return kind
	}
	return fmt.Errorf("%w: %s", kind, fmt.Sprintf(format, args...))
}

// NotFound 构造 not found 错误。
func NotFound(format string, args ...any) error {
	return New(ErrNotFound, format, args...)
}

// Conflict 构造 conflict 错误。
func Conflict(format string, args ...any) error {
	return New(ErrConflict, format, args...)
}

// Invalid 构造 invalid 错误。
func Invalid(format string, args ...any) error {
	return New(ErrInvalid, format, args...)
}

// Unavailable 构造 unavailable 错误。
func Unavailable(format string, args ...any) error {
	return New(ErrUnavailable, format, args...)
}

// Unauthorized 构造 unauthorized 错误。
func Unauthorized(format string, args ...any) error {
	return New(ErrUnauthorized, format, args...)
}

// Is 判断 err 是否包装了 target。
func Is(err, target error) bool { return stderrors.Is(err, target) }

// As 把 err 解到 target 类型。
func As(err error, target any) bool { return stderrors.As(err, target) }

// Unwrap 取出被包装的下一层错误。
func Unwrap(err error) error { return stderrors.Unwrap(err) }

// IsNotFound 判断是否为 not found。
func IsNotFound(err error) bool { return Is(err, ErrNotFound) }

// IsConflict 判断是否为 conflict。
func IsConflict(err error) bool { return Is(err, ErrConflict) }

// IsInvalid 判断是否为 invalid。
func IsInvalid(err error) bool { return Is(err, ErrInvalid) }

// IsUnavailable 判断是否为 unavailable。
func IsUnavailable(err error) bool { return Is(err, ErrUnavailable) }

// IsUnauthorized 判断是否为 unauthorized。
func IsUnauthorized(err error) bool { return Is(err, ErrUnauthorized) }
