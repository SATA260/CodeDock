package tools

import (
	"fmt"
	"path/filepath"
	"strings"

	"codedock/pkg/agent/tool"
)

var (
	pathAbs = filepath.Abs
	pathRel = filepath.Rel
)

func jailPath(root, target string, fileSystem FileSystem) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("workspace root is required")
	}
	rootAbs, err := pathAbs(root)
	if err != nil {
		return "", err
	}
	rootAbs = filepath.Clean(rootAbs)
	targetAbs, err := pathAbs(target)
	if err != nil {
		return "", err
	}
	targetAbs = filepath.Clean(targetAbs)
	if !insideRoot(rootAbs, targetAbs) {
		return "", fmt.Errorf("%w: %s", tool.ErrOutsideWorkspace, target)
	}
	targetReal := evalExisting(targetAbs, fileSystem)
	if targetReal != targetAbs {
		rootReal := evalExisting(rootAbs, fileSystem)
		if !insideRoot(rootReal, targetReal) {
			return "", fmt.Errorf("%w: %s", tool.ErrOutsideWorkspace, target)
		}
		return targetReal, nil
	}
	return targetAbs, nil
}

func evalExisting(path string, fileSystem FileSystem) string {
	if fileSystem != nil {
		if linked, err := fileSystem.EvalSymlinks(path); err == nil && linked != "" {
			return linked
		}
		return path
	}
	if linked, err := filepath.EvalSymlinks(path); err == nil && linked != "" {
		return linked
	}
	return path
}

func insideRoot(root, target string) bool {
	rel, err := pathRel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func workspaceOf(input tool.Input, ports Ports) string {
	if strings.TrimSpace(input.WorkspaceRoot) != "" {
		return input.WorkspaceRoot
	}
	return ports.WorkspaceRoot
}
