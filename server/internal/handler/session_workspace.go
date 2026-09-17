package handler

import (
	"os"
	"path/filepath"
	"strings"

	cderr "codedock/internal/errors"
)

const implicitWorkspaceID = "default"

func expandUserPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return p
		}
		if p == "~" {
			return home
		}
		return filepath.Join(home, filepath.FromSlash(p[2:]))
	}
	return p
}

// freezeSessionWorkspace 创建会话时冻结工作目录。显式路径必须是已存在的目录；空或 default 用进程回落。
func freezeSessionWorkspace(workspaceID, fallback string) (string, error) {
	trimmed := expandUserPath(workspaceID)
	if trimmed == "" || strings.EqualFold(trimmed, implicitWorkspaceID) {
		if fallback == "" {
			return ".", nil
		}
		return fallback, nil
	}
	info, err := os.Stat(trimmed)
	if err != nil {
		return "", cderr.Invalid("workspace directory not found")
	}
	if !info.IsDir() {
		return "", cderr.Invalid("workspace is not a directory")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", cderr.Invalid("%s", err.Error())
	}
	return abs, nil
}
