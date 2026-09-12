package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type outputAccumulator struct {
	mutex            sync.Mutex
	executor         *Executor
	prefix           string
	pending          []byte
	tail             []byte
	tailAtBoundary   bool
	totalBytes       int
	completedLines   int
	hasOpenLine      bool
	currentLineBytes int
	tempFile         *os.File
	tempFilePath     string
	err              error
}

func newOutputAccumulator(executor *Executor, prefix string) *outputAccumulator {
	return &outputAccumulator{
		executor:       executor,
		prefix:         prefix,
		tailAtBoundary: true,
	}
}

func (accumulator *outputAccumulator) Append(data []byte) {
	if len(data) == 0 {
		return
	}
	accumulator.mutex.Lock()
	defer accumulator.mutex.Unlock()
	if accumulator.err != nil {
		return
	}

	accumulator.totalBytes += len(data)
	for _, value := range data {
		if value == '\n' {
			accumulator.completedLines++
			accumulator.currentLineBytes = 0
			accumulator.hasOpenLine = false
		} else {
			accumulator.currentLineBytes++
			accumulator.hasOpenLine = true
		}
	}
	if accumulator.tempFile != nil {
		_, accumulator.err = accumulator.tempFile.Write(data)
	} else {
		accumulator.pending = append(accumulator.pending, data...)
		if accumulator.shouldSpill() {
			accumulator.openTempFile()
		}
	}

	accumulator.tail = append(accumulator.tail, data...)
	maxRollingBytes := DefaultMaxBytes * 2
	if len(accumulator.tail) > maxRollingBytes*2 {
		start := len(accumulator.tail) - maxRollingBytes
		for start < len(accumulator.tail) && accumulator.tail[start]&0xc0 == 0x80 {
			start++
		}
		accumulator.tailAtBoundary = start == 0 || accumulator.tail[start-1] == '\n'
		accumulator.tail = append([]byte(nil), accumulator.tail[start:]...)
	}
}

func (accumulator *outputAccumulator) Finish(emptyText string) (string, *ResultDetails, error) {
	accumulator.mutex.Lock()
	defer accumulator.mutex.Unlock()
	if accumulator.tempFile != nil {
		if err := accumulator.tempFile.Close(); accumulator.err == nil {
			accumulator.err = err
		}
		accumulator.tempFile = nil
	}
	if accumulator.err != nil {
		return "", nil, accumulator.err
	}

	tail := accumulator.tail
	if !accumulator.tailAtBoundary {
		if newline := strings.IndexByte(string(tail), '\n'); newline >= 0 {
			tail = tail[newline+1:]
		}
	}
	decoded := strings.ToValidUTF8(string(tail), "\uFFFD")
	truncation := TruncateTail(decoded, DefaultMaxLines, DefaultMaxBytes)
	totalLines := accumulator.completedLines
	if accumulator.hasOpenLine {
		totalLines++
	}
	truncated := totalLines > DefaultMaxLines || accumulator.totalBytes > DefaultMaxBytes
	truncation.Truncated = truncated
	truncation.TotalLines = totalLines
	truncation.TotalBytes = accumulator.totalBytes
	if truncated && truncation.TruncatedBy == nil {
		by := "lines"
		if accumulator.totalBytes > DefaultMaxBytes {
			by = "bytes"
		}
		truncation.TruncatedBy = &by
	}

	text := truncation.Content
	if text == "" {
		text = emptyText
	}
	if !truncation.Truncated {
		return text, nil, nil
	}

	startLine := truncation.TotalLines - truncation.OutputLines + 1
	endLine := truncation.TotalLines
	switch {
	case truncation.LastLinePartial:
		text += fmt.Sprintf(
			"\n\n[Showing last %s of line %d (line is %s). Full output: %s]",
			FormatSize(truncation.OutputBytes),
			endLine,
			FormatSize(accumulator.currentLineBytes),
			accumulator.tempFilePath,
		)
	case truncation.TruncatedBy != nil && *truncation.TruncatedBy == "lines":
		text += fmt.Sprintf(
			"\n\n[Showing lines %d-%d of %d. Full output: %s]",
			startLine,
			endLine,
			truncation.TotalLines,
			accumulator.tempFilePath,
		)
	default:
		text += fmt.Sprintf(
			"\n\n[Showing lines %d-%d of %d (%s limit). Full output: %s]",
			startLine,
			endLine,
			truncation.TotalLines,
			FormatSize(DefaultMaxBytes),
			accumulator.tempFilePath,
		)
	}
	return text, &ResultDetails{
		Truncation:     &truncation,
		FullOutputPath: accumulator.tempFilePath,
	}, nil
}

func (accumulator *outputAccumulator) shouldSpill() bool {
	totalLines := accumulator.completedLines
	if accumulator.hasOpenLine {
		totalLines++
	}
	return accumulator.totalBytes > DefaultMaxBytes || totalLines > DefaultMaxLines
}

func (accumulator *outputAccumulator) openTempFile() {
	if accumulator.tempFile != nil || accumulator.err != nil {
		return
	}
	file, err := os.CreateTemp(accumulator.executor.TempDir, accumulator.prefix+"-*.log")
	if err != nil {
		accumulator.err = err
		return
	}
	accumulator.tempFile = file
	accumulator.tempFilePath = filepath.Clean(file.Name())
	if _, err := file.Write(accumulator.pending); err != nil {
		accumulator.err = err
		return
	}
	accumulator.pending = nil
}
