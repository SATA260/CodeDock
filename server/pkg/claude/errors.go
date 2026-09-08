package claude

import (
	"errors"
	"fmt"
)

var (
	errNotFound     = errors.New("not found")
	errConflict     = errors.New("conflict")
	errInvalid      = errors.New("invalid")
	errUnavailable  = errors.New("unavailable")
	errUnauthorized = errors.New("unauthorized")
	errArchived     = errors.New("archived")
)

func wrapErr(kind error, format string, args ...any) error {
	if format == "" {
		return kind
	}
	return fmt.Errorf("%w: %s", kind, fmt.Sprintf(format, args...))
}
