package tools

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

func (executor *Executor) Read(ctx context.Context, cwd string, input ReadInput) (ToolResult, error) {
	executor = executor.withDefaults()
	if err := contextOperationError(ctx); err != nil {
		return ToolResult{}, err
	}

	absolutePath, err := executor.resolveReadPath(input.Path, cwd)
	if err != nil {
		return ToolResult{}, err
	}
	data, err := executor.FS.ReadFile(absolutePath)
	if err != nil {
		return ToolResult{}, err
	}
	if err := contextOperationError(ctx); err != nil {
		return ToolResult{}, err
	}

	if mimeType := detectSupportedImageMIME(data); mimeType != "" {
		image, imageErr := processImage(data, mimeType)
		if imageErr != nil {
			message := "[Image omitted: could not be resized below the inline image size limit.]"
			if errors.Is(imageErr, errImageConversion) {
				message = "[Image omitted: could not be converted to a supported inline image format.]"
			}
			return textResult(
				fmt.Sprintf("Read image file [%s]\n%s", mimeType, message),
				nil,
			), nil
		}
		text := fmt.Sprintf("Read image file [%s]", image.mimeType)
		if image.convertedFrom != "" {
			text += fmt.Sprintf("\n[Image converted from %s to %s.]", image.convertedFrom, image.mimeType)
		}
		if image.width != image.originalWidth || image.height != image.originalHeight {
			scale := float64(image.originalWidth) / float64(image.width)
			text += fmt.Sprintf(
				"\n[Image: original %dx%d, displayed at %dx%d. Multiply coordinates by %.2f to map to original image.]",
				image.originalWidth,
				image.originalHeight,
				image.width,
				image.height,
				scale,
			)
		}
		return ToolResult{
			Content: []ContentBlock{
				{Type: "text", Text: text},
				{Type: "image", Data: image.data, MIMEType: image.mimeType},
			},
		}, nil
	}

	textContent := strings.ToValidUTF8(string(data), "\uFFFD")
	allLines := strings.Split(textContent, "\n")
	startLine := 0.0
	if input.Offset != nil && *input.Offset != 0 {
		startLine = max(0, *input.Offset-1)
	}
	if startLine >= float64(len(allLines)) {
		return ToolResult{}, fmt.Errorf(
			"Offset %s is beyond end of file (%d lines total)",
			formatJSONNumber(input.Offset),
			len(allLines),
		)
	}

	startIndex := jsSliceIndex(startLine, len(allLines))
	selectedLines := allLines[startIndex:]
	var userLimitedLines *float64
	if input.Limit != nil {
		endLine := min(startLine+*input.Limit, float64(len(allLines)))
		endIndex := jsSliceIndex(endLine, len(allLines))
		if endIndex < startIndex {
			endIndex = startIndex
		}
		selectedLines = allLines[startIndex:endIndex]
		limitedLines := endLine - startLine
		userLimitedLines = &limitedLines
	}
	selectedContent := strings.Join(selectedLines, "\n")
	truncation := TruncateHead(selectedContent, DefaultMaxLines, DefaultMaxBytes)
	startLineDisplay := startLine + 1
	var output string
	var details *ResultDetails

	switch {
	case truncation.FirstLineExceedsLimit:
		firstLineSize := FormatSize(len([]byte(allLines[startIndex])))
		output = fmt.Sprintf(
			"[Line %s is %s, exceeds %s limit. Use bash: sed -n '%sp' %s | head -c %d]",
			formatNumber(startLineDisplay),
			firstLineSize,
			FormatSize(DefaultMaxBytes),
			formatNumber(startLineDisplay),
			input.Path,
			DefaultMaxBytes,
		)
		details = &ResultDetails{Truncation: &truncation}
	case truncation.Truncated:
		endLineDisplay := startLineDisplay + float64(truncation.OutputLines) - 1
		nextOffset := endLineDisplay + 1
		output = truncation.Content
		if truncation.TruncatedBy != nil && *truncation.TruncatedBy == "lines" {
			output += fmt.Sprintf(
				"\n\n[Showing lines %s-%s of %d. Use offset=%s to continue.]",
				formatNumber(startLineDisplay),
				formatNumber(endLineDisplay),
				len(allLines),
				formatNumber(nextOffset),
			)
		} else {
			output += fmt.Sprintf(
				"\n\n[Showing lines %s-%s of %d (%s limit). Use offset=%s to continue.]",
				formatNumber(startLineDisplay),
				formatNumber(endLineDisplay),
				len(allLines),
				FormatSize(DefaultMaxBytes),
				formatNumber(nextOffset),
			)
		}
		details = &ResultDetails{Truncation: &truncation}
	case userLimitedLines != nil && startLine+*userLimitedLines < float64(len(allLines)):
		remaining := float64(len(allLines)) - (startLine + *userLimitedLines)
		nextOffset := startLine + *userLimitedLines + 1
		output = fmt.Sprintf(
			"%s\n\n[%s more lines in file. Use offset=%s to continue.]",
			truncation.Content,
			formatNumber(remaining),
			formatNumber(nextOffset),
		)
	default:
		output = truncation.Content
	}
	return textResult(output, details), nil
}

func jsSliceIndex(value float64, length int) int {
	switch {
	case math.IsNaN(value):
		return 0
	case math.IsInf(value, 1):
		return length
	case math.IsInf(value, -1):
		return 0
	}
	integer := math.Trunc(value)
	if integer < 0 {
		return max(0, length+int(integer))
	}
	return min(length, int(integer))
}

func contextOperationError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("Operation aborted")
	default:
		return nil
	}
}

func formatJSONNumber(value *float64) string {
	if value == nil {
		return ""
	}
	return formatNumber(*value)
}

func formatNumber(value float64) string {
	if value == 0 {
		return "0"
	}
	absolute := math.Abs(value)
	if absolute >= 1e-6 && absolute < 1e21 {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	formatted := strconv.FormatFloat(value, 'e', -1, 64)
	for _, marker := range []string{"e-0", "e+0"} {
		if strings.Contains(formatted, marker) {
			formatted = strings.Replace(formatted, marker, marker[:2], 1)
		}
	}
	return formatted
}
