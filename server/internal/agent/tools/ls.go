package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

func (executor *Executor) LS(ctx context.Context, cwd string, input LSInput) (ToolResult, error) {
	executor = executor.withDefaults()
	path := "."
	if input.Path != nil && *input.Path != "" {
		path = *input.Path
	}
	dirPath, err := executor.resolveToCWD(path, cwd)
	if err != nil {
		return ToolResult{}, err
	}
	info, err := executor.FS.Stat(dirPath)
	if err != nil {
		return ToolResult{}, fmt.Errorf("Path not found: %s", dirPath)
	}
	if !info.IsDir() {
		return ToolResult{}, fmt.Errorf("Not a directory: %s", dirPath)
	}
	entries, err := executor.FS.ReadDir(dirPath)
	if err != nil {
		return ToolResult{}, fmt.Errorf("Cannot read directory: %s", err)
	}
	if err := contextOperationError(ctx); err != nil {
		return ToolResult{}, err
	}
	collator := collate.New(language.AmericanEnglish)
	sort.SliceStable(entries, func(left int, right int) bool {
		leftName := strings.ToLower(entries[left].Name())
		rightName := strings.ToLower(entries[right].Name())
		return collator.CompareString(leftName, rightName) < 0
	})

	effectiveLimit := 500.0
	if input.Limit != nil {
		effectiveLimit = *input.Limit
	}
	results := make([]string, 0, len(entries))
	entryLimitReached := false
	for _, entry := range entries {
		if float64(len(results)) >= effectiveLimit {
			entryLimitReached = true
			break
		}
		entryInfo, statErr := executor.FS.Stat(filepath.Join(dirPath, entry.Name()))
		if statErr != nil {
			continue
		}
		name := entry.Name()
		if entryInfo.IsDir() {
			name += "/"
		}
		results = append(results, name)
	}
	if len(results) == 0 {
		return textResult("(empty directory)", nil), nil
	}

	truncation := TruncateHead(strings.Join(results, "\n"), int(^uint(0)>>1), DefaultMaxBytes)
	output := truncation.Content
	details := &ResultDetails{}
	notices := make([]string, 0, 2)
	if entryLimitReached {
		details.EntryLimitReached = floatPointer(effectiveLimit)
		notices = append(
			notices,
			fmt.Sprintf(
				"%s entries limit reached. Use limit=%s for more",
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
	if details.EntryLimitReached == nil && details.Truncation == nil {
		details = nil
	}
	return textResult(output, details), nil
}
