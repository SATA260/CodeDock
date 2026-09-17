package codex

import (
	"context"
	"io"
	"sync"

	pkg "codedock/pkg/codex"
)

type pipeProc struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	wait   chan struct{}
	once   sync.Once
}

func (p *pipeProc) Stdin() io.WriteCloser { return p.stdin }
func (p *pipeProc) Stdout() io.ReadCloser { return p.stdout }
func (p *pipeProc) Stderr() io.ReadCloser { return p.stderr }
func (p *pipeProc) Wait() error {
	<-p.wait
	return io.EOF
}
func (p *pipeProc) Kill() error {
	p.once.Do(func() {
		_ = p.stdin.Close()
		_ = p.stdout.Close()
		_ = p.stderr.Close()
		close(p.wait)
	})
	return nil
}

// LoopbackStarter 用内存管道假 app-server，供测试。
func LoopbackStarter(handler func(pkg.Envelope) []pkg.Envelope) Starter {
	return func(ctx context.Context, bin string) (Proc, error) {
		sr, sw := io.Pipe()
		cr, cw := io.Pipe()
		er, ew := io.Pipe()
		_ = ew.Close()
		server := pkg.NewJSONL(sr, cw, nil, 0)
		go pkg.Serve(server, handler)
		return &pipeProc{stdin: sw, stdout: cr, stderr: er, wait: make(chan struct{})}, nil
	}
}
