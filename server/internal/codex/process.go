package codex

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Proc 是一条已启动的 app-server 进程。
type Proc interface {
	Stdin() io.WriteCloser
	Stdout() io.ReadCloser
	Stderr() io.ReadCloser
	Wait() error
	Kill() error
}

// Starter 启动一条 app-server 进程。测试可换成假进程。
type Starter func(ctx context.Context, bin string) (Proc, error)

// Versioner 读取 `codex --version`。
type Versioner func(ctx context.Context, bin string) (string, error)

// LookPath 解析可执行文件路径。
type LookPath func(file string) (string, error)

type execProc struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	once   sync.Once
}

func (p *execProc) Stdin() io.WriteCloser { return p.stdin }
func (p *execProc) Stdout() io.ReadCloser { return p.stdout }
func (p *execProc) Stderr() io.ReadCloser { return p.stderr }
func (p *execProc) Wait() error           { return p.cmd.Wait() }
func (p *execProc) Kill() error {
	var err error
	p.once.Do(func() {
		if p.cmd.Process != nil {
			err = p.cmd.Process.Kill()
		}
	})
	return err
}

// DefaultStarter 启动 `bin app-server --stdio`。进程寿命不绑在调用方 ctx 上，避免 HTTP 请求结束时把长驻 app-server 杀掉。
func DefaultStarter(ctx context.Context, bin string) (Proc, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, "app-server", "--stdio")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execProc{cmd: cmd, stdin: stdin, stdout: stdout, stderr: stderr}, nil
}

// DefaultVersioner 跑 `bin --version`。
func DefaultVersioner(ctx context.Context, bin string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, "--version")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// DefaultLookPath 用 exec.LookPath。
func DefaultLookPath(file string) (string, error) {
	if file == "" {
		file = "codex"
	}
	return exec.LookPath(file)
}

// Drain 丢掉 stderr，避免管道堵住。
func Drain(r io.Reader) {
	if r == nil {
		return
	}
	go func() {
		_, _ = io.Copy(io.Discard, r)
	}()
}

// ParseVersion 从 `codex --version` 输出里取出版本号。
func ParseVersion(out string) string {
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(strings.TrimSuffix(lines[i], "\r"))
		if line == "" {
			continue
		}
		if v, ok := strings.CutPrefix(line, "codex-cli "); ok {
			return strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(line, "codex "); ok {
			return strings.TrimSpace(v)
		}
		return line
	}
	return ""
}
