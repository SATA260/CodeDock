package tools

import (
	"bytes"
	"sync"
)

type lineStream struct {
	mutex   sync.Mutex
	pending []byte
	onLine  func(line []byte) bool
	stopped bool
}

func (stream *lineStream) Append(data []byte) {
	stream.mutex.Lock()
	defer stream.mutex.Unlock()
	if stream.stopped {
		return
	}
	stream.pending = append(stream.pending, data...)
	for {
		newline := bytes.IndexByte(stream.pending, '\n')
		if newline == -1 {
			return
		}
		line := append([]byte(nil), stream.pending[:newline]...)
		stream.pending = stream.pending[newline+1:]
		if !stream.onLine(line) {
			stream.stopped = true
			stream.pending = nil
			return
		}
	}
}

func (stream *lineStream) Finish() {
	stream.mutex.Lock()
	defer stream.mutex.Unlock()
	if stream.stopped || len(stream.pending) == 0 {
		return
	}
	stream.onLine(append([]byte(nil), stream.pending...))
	stream.pending = nil
}

type limitedBuffer struct {
	mutex sync.Mutex
	data  []byte
	limit int
}

func (buffer *limitedBuffer) Append(data []byte) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	remaining := buffer.limit - len(buffer.data)
	if remaining <= 0 {
		return
	}
	buffer.data = append(buffer.data, data[:min(len(data), remaining)]...)
}

func (buffer *limitedBuffer) String() string {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return stringsToValidUTF8(buffer.data)
}
