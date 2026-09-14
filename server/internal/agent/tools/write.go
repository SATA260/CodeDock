package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"unicode/utf16"
)

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
		if err := executor.FS.WriteFile(absolutePath, []byte(input.Content), 0o644); err != nil {
			return ToolResult{}, err
		}
		if err := contextOperationError(ctx); err != nil {
			return ToolResult{}, err
		}
		length := len(utf16.Encode([]rune(input.Content)))
		return textResult(
			fmt.Sprintf("Successfully wrote %d bytes to %s", length, input.Path),
			nil,
		), nil
	})
}
