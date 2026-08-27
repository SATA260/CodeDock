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

func NotFound(format string, args ...any) error {
	return New(ErrNotFound, format, args...)
}

func Conflict(format string, args ...any) error {
	return New(ErrConflict, format, args...)
}

func Invalid(format string, args ...any) error {
	return New(ErrInvalid, format, args...)
}

func Unavailable(format string, args ...any) error {
	return New(ErrUnavailable, format, args...)
}

func Unauthorized(format string, args ...any) error {
	return New(ErrUnauthorized, format, args...)
}

func Is(err, target error) bool     { return stderrors.Is(err, target) }
func As(err error, target any) bool { return stderrors.As(err, target) }
func Unwrap(err error) error        { return stderrors.Unwrap(err) }

func IsNotFound(err error) bool     { return Is(err, ErrNotFound) }
func IsConflict(err error) bool     { return Is(err, ErrConflict) }
func IsInvalid(err error) bool      { return Is(err, ErrInvalid) }
func IsUnavailable(err error) bool  { return Is(err, ErrUnavailable) }
func IsUnauthorized(err error) bool { return Is(err, ErrUnauthorized) }
