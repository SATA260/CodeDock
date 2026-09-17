package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

func (executor *Executor) Find(ctx context.Context, cwd string, input FindInput) (ToolResult, error) {
	executor = executor.withDefaults()
	searchDir := "."
	if input.Path != nil && *input.Path != "" {
		searchDir = *input.Path
	}
	searchPath, err := executor.resolveToCWD(searchDir, cwd)
	if err != nil {
		return ToolResult{}, err
	}
	if _, err := executor.FS.Stat(searchPath); err != nil {
		return ToolResult{}, fmt.Errorf("Path not found: %s", searchPath)
	}
	fdPath, err := executor.LookPath("fd")
	if err != nil {
		return ToolResult{}, fmt.Errorf("fd is not available and could not be downloaded")
	}
	effectiveLimit := 1000.0
	if input.Limit != nil {
		effectiveLimit = *input.Limit
	}

	args := []string{"--glob", "--color=never", "--hidden"}
	if !executor.isInsideGitRepository(searchPath) {
		args = append(args, "--no-require-git")
	}
	args = append(args, "--max-results", formatNumber(effectiveLimit))
	effectivePattern := input.Pattern
	if strings.Contains(input.Pattern, "/") {
		args = append(args, "--full-path")
		if !strings.HasPrefix(input.Pattern, "/") &&
			!strings.HasPrefix(input.Pattern, "**/") &&
			input.Pattern != "**" {
			effectivePattern = "**/" + input.Pattern
		}
		if executor.GOOS == "windows" {
			effectivePattern = strings.ReplaceAll(effectivePattern, "/", `[/\\]`)
		}
	}
	args = append(args, "--", effectivePattern, searchPath)

	results := make([]string, 0)
	stdout := lineStream{
		onLine: func(rawLine []byte) bool {
			line := strings.TrimSpace(strings.TrimSuffix(stringsToValidUTF8(rawLine), "\r"))
			if line != "" {
				results = append(results, relativizeFindResultPath(line, searchPath))
			}
			return true
		},
	}
	stderr := limitedBuffer{limit: DefaultMaxBytes}
	commandResult, err := executor.RunCommand(
		ctx,
		fdPath,
		args,
		cwd,
		executor.Env,
		stdout.Append,
		stderr.Append,
	)
	stdout.Finish()
	if ctx.Err() != nil {
		return ToolResult{}, fmt.Errorf("Operation aborted")
	}
	if err != nil {
		return ToolResult{}, fmt.Errorf("Failed to run fd: %s", err)
	}
	if commandResult.ExitCode != 0 && len(results) == 0 {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = fmt.Sprintf("fd exited with code %d", commandResult.ExitCode)
		}
		return ToolResult{}, fmt.Errorf("%s", message)
	}
	if len(results) == 0 {
		return textResult("No files found matching pattern", nil), nil
	}

	resultLimitReached := float64(len(results)) >= effectiveLimit
	truncation := TruncateHead(strings.Join(results, "\n"), int(^uint(0)>>1), DefaultMaxBytes)
	output := truncation.Content
	details := &ResultDetails{}
	notices := make([]string, 0, 2)
	if resultLimitReached {
		details.ResultLimitReached = floatPointer(effectiveLimit)
		notices = append(
			notices,
			fmt.Sprintf(
				"%s results limit reached. Use limit=%s for more, or refine pattern",
				formatNumber(effectiveLimit),
				formatNumber(effectiveLimit*2),
			),
		)
	}
	if truncation.Truncated {
		details.Truncation = &truncation
		notices = append(notices, fmt.Sprintf("%s limit reached", FormatSize(DefaultMaxBytes)))
	}
	if len(notices) > 0 {
		output += "\n\n[" + strings.Join(notices, ". ") + "]"
	}
	if details.ResultLimitReached == nil && details.Truncation == nil {
		details = nil
	}
	return textResult(output, details), nil
}

func (executor *Executor) isInsideGitRepository(path string) bool {
	current := path
	for {
		if _, err := executor.FS.Stat(filepath.Join(current, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

func relativizeFindResultPath(resultPath string, searchPath string) string {
	hadTrailingSeparator := strings.HasSuffix(resultPath, string(filepath.Separator)) ||
		strings.HasSuffix(resultPath, "/")
	relativePath := resultPath
	if filepath.IsAbs(resultPath) {
		if relative, err := filepath.Rel(searchPath, resultPath); err == nil {
			relativePath = relative
		}
	}
	relativePath = filepath.ToSlash(relativePath)
	if hadTrailingSeparator && !strings.HasSuffix(relativePath, "/") {
		relativePath += "/"
	}
	return relativePath
}
