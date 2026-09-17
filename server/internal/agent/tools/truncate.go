package tools

import (
	"fmt"
	"strings"
	"unicode/utf16"
)

const (
	DefaultMaxLines   = 2000
	DefaultMaxBytes   = 50 * 1024
	GrepMaxLineLength = 500
)

type TruncationResult struct {
	Content               string  `json:"content"`
	Truncated             bool    `json:"truncated"`
	TruncatedBy           *string `json:"truncatedBy"`
	TotalLines            int     `json:"totalLines"`
	TotalBytes            int     `json:"totalBytes"`
	OutputLines           int     `json:"outputLines"`
	OutputBytes           int     `json:"outputBytes"`
	LastLinePartial       bool    `json:"lastLinePartial"`
	FirstLineExceedsLimit bool    `json:"firstLineExceedsLimit"`
	MaxLines              int     `json:"maxLines"`
	MaxBytes              int     `json:"maxBytes"`
}

func splitLinesForCounting(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func FormatSize(bytes int) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%dB", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
}

func TruncateHead(content string, maxLines int, maxBytes int) TruncationResult {
	maxLines, maxBytes = truncationLimits(maxLines, maxBytes)
	lines := splitLinesForCounting(content)
	totalBytes := len([]byte(content))
	if len(lines) <= maxLines && totalBytes <= maxBytes {
		return noTruncation(content, len(lines), totalBytes, maxLines, maxBytes)
	}

	if len(lines) > 0 && len([]byte(lines[0])) > maxBytes {
		by := "bytes"
		return TruncationResult{
			Content:               "",
			Truncated:             true,
			TruncatedBy:           &by,
			TotalLines:            len(lines),
			TotalBytes:            totalBytes,
			FirstLineExceedsLimit: true,
			MaxLines:              maxLines,
			MaxBytes:              maxBytes,
		}
	}

	output := make([]string, 0, min(len(lines), maxLines))
	outputBytes := 0
	truncatedBy := "lines"
	for index, line := range lines {
		if index >= maxLines {
			break
		}
		lineBytes := len([]byte(line))
		if index > 0 {
			lineBytes++
		}
		if outputBytes+lineBytes > maxBytes {
			truncatedBy = "bytes"
			break
		}
		output = append(output, line)
		outputBytes += lineBytes
	}
	if len(output) >= maxLines && outputBytes <= maxBytes {
		truncatedBy = "lines"
	}

	content = strings.Join(output, "\n")
	return TruncationResult{
		Content:     content,
		Truncated:   true,
		TruncatedBy: &truncatedBy,
		TotalLines:  len(lines),
		TotalBytes:  totalBytes,
		OutputLines: len(output),
		OutputBytes: len([]byte(content)),
		MaxLines:    maxLines,
		MaxBytes:    maxBytes,
	}
}

func TruncateTail(content string, maxLines int, maxBytes int) TruncationResult {
	maxLines, maxBytes = truncationLimits(maxLines, maxBytes)
	lines := splitLinesForCounting(content)
	totalBytes := len([]byte(content))
	if len(lines) <= maxLines && totalBytes <= maxBytes {
		return noTruncation(content, len(lines), totalBytes, maxLines, maxBytes)
	}

	output := make([]string, 0, min(len(lines), maxLines))
	outputBytes := 0
	truncatedBy := "lines"
	lastLinePartial := false
	for index := len(lines) - 1; index >= 0 && len(output) < maxLines; index-- {
		line := lines[index]
		lineBytes := len([]byte(line))
		if len(output) > 0 {
			lineBytes++
		}
		if outputBytes+lineBytes > maxBytes {
			truncatedBy = "bytes"
			if len(output) == 0 {
				line = truncateStringToBytesFromEnd(line, maxBytes)
				output = append(output, line)
				outputBytes = len([]byte(line))
				lastLinePartial = true
			}
			break
		}
		output = append(output, "")
		copy(output[1:], output[:len(output)-1])
		output[0] = line
		outputBytes += lineBytes
	}
	if len(output) >= maxLines && outputBytes <= maxBytes {
		truncatedBy = "lines"
	}

	content = strings.Join(output, "\n")
	return TruncationResult{
		Content:         content,
		Truncated:       true,
		TruncatedBy:     &truncatedBy,
		TotalLines:      len(lines),
		TotalBytes:      totalBytes,
		OutputLines:     len(output),
		OutputBytes:     len([]byte(content)),
		LastLinePartial: lastLinePartial,
		MaxLines:        maxLines,
		MaxBytes:        maxBytes,
	}
}

func TruncateLine(line string, maxUTF16Units int) (string, bool) {
	if maxUTF16Units <= 0 {
		maxUTF16Units = GrepMaxLineLength
	}
	units := utf16.Encode([]rune(line))
	if len(units) <= maxUTF16Units {
		return line, false
	}
	return string(utf16.Decode(units[:maxUTF16Units])) + "... [truncated]", true
}

func truncationLimits(maxLines int, maxBytes int) (int, int) {
	if maxLines <= 0 {
		maxLines = DefaultMaxLines
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	return maxLines, maxBytes
}

func noTruncation(content string, lines int, bytes int, maxLines int, maxBytes int) TruncationResult {
	return TruncationResult{
		Content:     content,
		TotalLines:  lines,
		TotalBytes:  bytes,
		OutputLines: lines,
		OutputBytes: bytes,
		MaxLines:    maxLines,
		MaxBytes:    maxBytes,
	}
}

func truncateStringToBytesFromEnd(value string, maxBytes int) string {
	data := []byte(value)
	if len(data) <= maxBytes {
		return value
	}
	start := len(data) - maxBytes
	for start < len(data) && data[start]&0xc0 == 0x80 {
		start++
	}
	return string(data[start:])
}
