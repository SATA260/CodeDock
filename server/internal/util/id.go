package util

import (
	"strings"

	"github.com/google/uuid"
)

// NewUUID 返回标准 UUID 字符串。
func NewUUID() string {
	return uuid.NewString()
}

// NewID 返回去掉连字符的 UUID，用作业务实体 ID。
func NewID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}
