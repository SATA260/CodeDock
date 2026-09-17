package codex

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
)

// Transport 是一条 JSONL 字节流。pkg 不启动进程。
type Transport interface {
	Read(ctx context.Context) ([]byte, error)
	Write(ctx context.Context, frame []byte) error
	Close() error
}

// JSONL 把读写流编成带长度上限的 JSONL Transport。
type JSONL struct {
	reader *bufio.Reader
	writer io.Writer
	closer io.Closer
	mu     sync.Mutex
	max    int
}

// NewJSONL 包装 r/w。closer 在 Close 时关掉；maxBytes<=0 时用 MaxFrameBytes。
func NewJSONL(r io.Reader, w io.Writer, closer io.Closer, maxBytes int) *JSONL {
	if maxBytes <= 0 {
		maxBytes = MaxFrameBytes
	}
	return &JSONL{
		reader: bufio.NewReaderSize(r, 64*1024),
		writer: w,
		closer: closer,
		max:    maxBytes,
	}
}

// Read 读下一帧；空行跳过。超过上限或半帧结束则报错。
func (t *JSONL) Read(ctx context.Context) ([]byte, error) {
	type result struct {
		line []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		for {
			line, err := t.readLine()
			if err != nil {
				ch <- result{err: err}
				return
			}
			if len(line) == 0 {
				continue
			}
			ch <- result{line: line}
			return
		}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case out := <-ch:
		return out.line, out.err
	}
}

func (t *JSONL) readLine() ([]byte, error) {
	var buf []byte
	for {
		chunk, err := t.reader.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			if t.max > 0 && len(buf)+len(chunk) > t.max {
				return nil, fmt.Errorf("jsonl frame exceeds %d bytes", t.max)
			}
			buf = append(buf, chunk...)
			continue
		}
		if err != nil {
			n := len(buf) + len(chunk)
			if err == io.EOF && n > 0 {
				return nil, fmt.Errorf("truncated jsonl frame")
			}
			return nil, err
		}
		chunk = bytes.TrimSuffix(chunk, []byte{'\n'})
		chunk = bytes.TrimSuffix(chunk, []byte{'\r'})
		if t.max > 0 && len(buf)+len(chunk) > t.max {
			return nil, fmt.Errorf("jsonl frame exceeds %d bytes", t.max)
		}
		return append(buf, chunk...), nil
	}
}

// Write 写出一帧并补换行。并发写出串行化。
func (t *JSONL) Write(ctx context.Context, frame []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.writer.Write(frame); err != nil {
		return err
	}
	_, err := t.writer.Write([]byte{'\n'})
	return err
}

// Close 关闭底层 closer。
func (t *JSONL) Close() error {
	if t.closer != nil {
		return t.closer.Close()
	}
	return nil
}

type closerPair struct{ a, b io.Closer }

func (c closerPair) Close() error {
	err1 := c.a.Close()
	err2 := c.b.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

// PipePair 返回一对相连的 JSONL Transport，供测试当假 app-server。
func PipePair() (Transport, Transport) {
	ar, aw := io.Pipe()
	br, bw := io.Pipe()
	left := NewJSONL(ar, bw, closerPair{aw, br}, 0)
	right := NewJSONL(br, aw, closerPair{bw, ar}, 0)
	return left, right
}
