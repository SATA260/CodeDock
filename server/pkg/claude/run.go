package claude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func claudeBin() string {
	if bin := strings.TrimSpace(os.Getenv("CLAUDE_BIN")); bin != "" {
		return bin
	}
	return "claude"
}

func claudeTimeout() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("CLAUDE_TIMEOUT")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return 60 * time.Second
}

type claudeOutput struct {
	stdout string
	stderr string
	code   int
	err    error
}

func runClaude(dir string, args ...string) claudeOutput {
	return runClaudeCtx(context.Background(), dir, args...)
}

func runClaudeCtx(ctx context.Context, dir string, args ...string) claudeOutput {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, claudeTimeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, claudeBin(), args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := claudeOutput{stdout: stdout.String(), stderr: stderr.String(), err: err}
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			out.err = fmt.Errorf("claude %s timed out", strings.Join(args, " "))
			out.code = -1
			return out
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			out.code = ee.ExitCode()
		} else {
			out.code = -1
		}
	}
	return out
}

func claudeErr(out claudeOutput) error {
	if out.err == nil {
		return nil
	}
	msg := strings.TrimSpace(out.stderr)
	if msg == "" {
		msg = strings.TrimSpace(out.stdout)
	}
	if msg == "" {
		msg = out.err.Error()
	}
	return errors.New(msg)
}

func lookClaude() error {
	bin := claudeBin()
	if filepath.IsAbs(bin) || strings.Contains(bin, string(os.PathSeparator)) {
		_, err := os.Stat(bin)
		return err
	}
	_, err := exec.LookPath(bin)
	return err
}

func startClaude(dir string, args []string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, error) {
	cmd := exec.Command(claudeBin(), args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	cmd.Env = os.Environ()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, nil, nil, err
	}
	return cmd, stdin, stdout, nil
}

func signalInterrupt(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
