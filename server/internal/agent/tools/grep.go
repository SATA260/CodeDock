package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
)

type ripgrepEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
	} `json:"data"`
}

func (executor *Executor) Grep(ctx context.Context, cwd string, input GrepInput) (ToolResult, error) {
	executor = executor.withDefaults()
	rgPath, err := executor.LookPath("rg")
	if err != nil {
		return ToolResult{}, fmt.Errorf("ripgrep (rg) is not available and could not be downloaded")
	}
	searchDir := "."
	if input.Path != nil && *input.Path != "" {
		searchDir = *input.Path
	}
	searchPath, err := executor.resolveToCWD(searchDir, cwd)
	if err != nil {
		return ToolResult{}, err
	}
	info, err := executor.FS.Stat(searchPath)
	if err != nil {
		return ToolResult{}, fmt.Errorf("Path not found: %s", searchPath)
	}

	contextLines := 0.0
	if input.Context != nil && *input.Context > 0 {
		contextLines = *input.Context
	}
	effectiveLimit := 100.0
	if input.Limit != nil {
		effectiveLimit = max(1, *input.Limit)
	}
	args := []string{"--json", "--line-number", "--color=never", "--hidden"}
	if input.IgnoreCase != nil && *input.IgnoreCase {
		args = append(args, "--ignore-case")
	}
	if input.Literal != nil && *input.Literal {
		args = append(args, "--fixed-strings")
	}
	if input.Glob != nil {
		args = append(args, "--glob", *input.Glob)
	}
	args = append(args, "--", input.Pattern, searchPath)

	commandContext, cancel := context.WithCancel(ctx)
	defer cancel()
	events := make([]ripgrepEvent, 0)
	var killedDueToLimit atomic.Bool
	stdout := lineStream{
		onLine: func(line []byte) bool {
			var event ripgrepEvent
			if json.Unmarshal(line, &event) != nil || event.Type != "match" {
				return true
			}
			events = append(events, event)
			if float64(len(events)) >= effectiveLimit {
				killedDueToLimit.Store(true)
				cancel()
				return false
			}
			return true
		},
	}
	stderr := limitedBuffer{limit: DefaultMaxBytes}
	commandResult, err := executor.RunCommand(
		commandContext,
		rgPath,
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
	if err != nil && !killedDueToLimit.Load() {
		return ToolResult{}, fmt.Errorf("Failed to run ripgrep: %s", err)
	}
	if !killedDueToLimit.Load() && commandResult.ExitCode != 0 && commandResult.ExitCode != 1 {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = fmt.Sprintf("ripgrep exited with code %d", commandResult.ExitCode)
		}
		return ToolResult{}, fmt.Errorf("%s", message)
	}

	if len(events) == 0 {
		return textResult("No matches found", nil), nil
	}

	matchLimitReached := killedDueToLimit.Load()
	linesTruncated := false
	outputLines := make([]string, 0, len(events))
	for _, event := range events {
		displayPath := filepath.Base(event.Data.Path.Text)
		if info.IsDir() {
			if relative, relativeErr := filepath.Rel(searchPath, event.Data.Path.Text); relativeErr == nil &&
				relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				displayPath = filepath.ToSlash(relative)
			}
		}
		if contextLines == 0 {
			line := strings.TrimSuffix(
				strings.ReplaceAll(strings.ReplaceAll(event.Data.Lines.Text, "\r\n", "\n"), "\r", ""),
				"\n",
			)
			line, truncated := TruncateLine(line, GrepMaxLineLength)
			linesTruncated = linesTruncated || truncated
			outputLines = append(outputLines, fmt.Sprintf("%s:%d: %s", displayPath, event.Data.LineNumber, line))
			continue
		}

		fileData, readErr := executor.FS.ReadFile(event.Data.Path.Text)
		if readErr != nil {
			outputLines = append(
				outputLines,
				fmt.Sprintf("%s:%d: (unable to read file)", displayPath, event.Data.LineNumber),
			)
			continue
		}
		fileLines := strings.Split(normalizeToLF(stringsToValidUTF8(fileData)), "\n")
		start := max(1, float64(event.Data.LineNumber)-contextLines)
		end := min(float64(len(fileLines)), float64(event.Data.LineNumber)+contextLines)
		for lineNumber := start; lineNumber <= end; lineNumber++ {
			line := ""
			if float64(int(lineNumber)) == lineNumber {
				line = fileLines[int(lineNumber)-1]
			}
			line, truncated := TruncateLine(strings.ReplaceAll(line, "\r", ""), GrepMaxLineLength)
			linesTruncated = linesTruncated || truncated
			formattedLineNumber := formatNumber(lineNumber)
			if lineNumber == float64(event.Data.LineNumber) {
				outputLines = append(outputLines, fmt.Sprintf("%s:%s: %s", displayPath, formattedLineNumber, line))
			} else {
				outputLines = append(outputLines, fmt.Sprintf("%s-%s- %s", displayPath, formattedLineNumber, line))
			}
		}
	}

	truncation := TruncateHead(strings.Join(outputLines, "\n"), int(^uint(0)>>1), DefaultMaxBytes)
	output := truncation.Content
	details := &ResultDetails{}
	notices := make([]string, 0, 3)
	if matchLimitReached {
		details.MatchLimitReached = floatPointer(effectiveLimit)
		notices = append(
			notices,
			fmt.Sprintf(
				"%s matches limit reached. Use limit=%s for more, or refine pattern",
				formatNumber(effectiveLimit),
				formatNumber(effectiveLimit*2),
			),
		)
	}
	if truncation.Truncated {
		details.Truncation = &truncation
		notices = append(notices, fmt.Sprintf("%s limit reached", FormatSize(DefaultMaxBytes)))
	}
	if linesTruncated {
		details.LinesTruncated = boolPointer(true)
		notices = append(
			notices,
			fmt.Sprintf("Some lines truncated to %d chars. Use read tool to see full lines", GrepMaxLineLength),
		)
	}
	if len(notices) > 0 {
		output += "\n\n[" + strings.Join(notices, ". ") + "]"
	}
	if details.MatchLimitReached == nil && details.Truncation == nil && details.LinesTruncated == nil {
		details = nil
	}
	return textResult(output, details), nil
}
