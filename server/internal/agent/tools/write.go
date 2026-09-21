package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"unicode/utf16"
)

// Write 把正文写到工作区路径，并带上相对旧内容的 unified patch。
func (executor *Executor) Write(ctx context.Context, cwd string, input WriteInput) (ToolResult, error) {
	executor = executor.withDefaults()
	absolutePath, err := executor.resolveToCWD(input.Path, cwd)
	if err != nil {
		return ToolResult{}, err
	}
	return executor.withFileMutation(absolutePath, func() (ToolResult, error) {
		if err := contextOperationError(ctx); err != nil {
			return ToolResult{}, err
		}
		if err := executor.FS.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
			return ToolResult{}, err
		}
		if err := contextOperationError(ctx); err != nil {
			return ToolResult{}, err
		}
		existed := true
		oldContent := ""
		if raw, err := executor.FS.ReadFile(absolutePath); err == nil {
			oldContent = string(raw)
		} else {
			existed = false
		}
		if err := executor.FS.WriteFile(absolutePath, []byte(input.Content), 0o644); err != nil {
			return ToolResult{}, err
		}
		if err := applyEditGuard(executor.FS, executor.Lint, absolutePath, oldContent, input.Content, existed); err != nil {
			return ToolResult{}, err
		}
		if err := contextOperationError(ctx); err != nil {
			return ToolResult{}, err
		}
		length := len(utf16.Encode([]rune(input.Content)))
		return textResult(
			fmt.Sprintf("Successfully wrote %d bytes to %s", length, input.Path),
			&ResultDetails{Patch: generateUnifiedPatch(input.Path, oldContent, input.Content)},
		), nil
	})
}
