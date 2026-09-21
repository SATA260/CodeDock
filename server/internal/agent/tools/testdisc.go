package tools

import (
	"os"
	"path/filepath"
	"strings"
)

// IsExistingTestFile 判定目标是否为工作区内已存在的测试文件。
func IsExistingTestFile(workspaceRoot, filePath string) bool {
	if strings.TrimSpace(workspaceRoot) == "" || strings.TrimSpace(filePath) == "" {
		return false
	}
	if !isTestFileName(filePath) {
		return false
	}
	abs := filePath
	if !filepath.IsAbs(filePath) {
		abs = filepath.Join(workspaceRoot, filePath)
	}
	info, err := os.Stat(abs)
	return err == nil && !info.IsDir()
}

// isTestFileName 按常见测试文件后缀判断。
func isTestFileName(filePath string) bool {
	base := strings.ToLower(filepath.Base(filePath))
	switch {
	case strings.HasSuffix(base, "_test.go"):
		return true
	case strings.HasSuffix(base, ".test.ts"), strings.HasSuffix(base, ".test.tsx"), strings.HasSuffix(base, ".test.js"):
		return true
	case strings.HasSuffix(base, ".spec.ts"), strings.HasSuffix(base, ".spec.tsx"), strings.HasSuffix(base, ".spec.js"):
		return true
	default:
		return false
	}
}
