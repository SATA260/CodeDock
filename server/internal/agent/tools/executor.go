package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type osFileSystem struct{}

func (osFileSystem) Access(name string) error {
	file, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return file.Close()
}

func (osFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (osFileSystem) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (osFileSystem) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osFileSystem) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (osFileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

func (osFileSystem) EvalSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

func (osFileSystem) Remove(name string) error {
	return os.Remove(name)
}

func defaultRunCommand(
	ctx context.Context,
	name string,
	args []string,
	dir string,
	env []string,
	onStdout func(data []byte),
	onStderr func(data []byte),
) (CommandResult, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	if env == nil {
		command.Env = os.Environ()
	} else {
		command.Env = env
	}
	configureProcess(command)
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		return killProcessTree(command.Process)
	}
	command.WaitDelay = 2 * time.Second
	command.Stdout = callbackWriter{callback: onStdout}
	command.Stderr = callbackWriter{callback: onStderr}
	err := command.Run()
	result := CommandResult{}
	if err == nil {
		return result, nil
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}

	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
		return result, nil
	}
	return result, err
}

type callbackWriter struct {
	callback func(data []byte)
}

func (writer callbackWriter) Write(data []byte) (int, error) {
	if writer.callback != nil {
		writer.callback(append([]byte(nil), data...))
	}
	return len(data), nil
}

func NewExecutor() *Executor {
	homeDir, _ := os.UserHomeDir()
	return &Executor{
		FS:         osFileSystem{},
		RunCommand: defaultRunCommand,
		LookPath:   exec.LookPath,
		TempDir:    os.TempDir(),
		GOOS:       runtime.GOOS,
		HomeDir:    homeDir,
		Env:        os.Environ(),
	}
}

func (executor *Executor) withDefaults() *Executor {
	defaults := NewExecutor()
	if executor == nil {
		return defaults
	}
	copy := *executor
	if copy.FS == nil {
		copy.FS = defaults.FS
	}
	if copy.RunCommand == nil {
		copy.RunCommand = defaults.RunCommand
	}
	if copy.LookPath == nil {
		copy.LookPath = defaults.LookPath
	}
	if copy.TempDir == "" {
		copy.TempDir = defaults.TempDir
	}
	if copy.GOOS == "" {
		copy.GOOS = defaults.GOOS
	}
	if copy.HomeDir == "" {
		copy.HomeDir = defaults.HomeDir
	}
	if copy.Env == nil {
		copy.Env = defaults.Env
	}
	return &copy
}

func (executor *Executor) Execute(
	ctx context.Context,
	toolName string,
	cwd string,
	rawInput json.RawMessage,
) (ToolResult, error) {
	executor = executor.withDefaults()

	switch toolName {
	case ToolRead:
		var input ReadInput
		if err := decodeToolInput(rawInput, &input, "path"); err != nil {
			return ToolResult{}, err
		}
		return executor.Read(ctx, cwd, input)
	case ToolBash:
		var input ShellInput
		if err := decodeToolInput(rawInput, &input, "command"); err != nil {
			return ToolResult{}, err
		}
		return executor.Bash(ctx, cwd, input)
	case ToolPowerShell:
		var input ShellInput
		if err := decodeToolInput(rawInput, &input, "command"); err != nil {
			return ToolResult{}, err
		}
		return executor.PowerShell(ctx, cwd, input)
	case ToolEdit:
		input, err := decodeEditInput(rawInput)
		if err != nil {
			return ToolResult{}, err
		}
		return executor.Edit(ctx, cwd, input)
	case ToolWrite:
		var input WriteInput
		if err := decodeToolInput(rawInput, &input, "path", "content"); err != nil {
			return ToolResult{}, err
		}
		return executor.Write(ctx, cwd, input)
	case ToolGrep:
		var input GrepInput
		if err := decodeToolInput(rawInput, &input, "pattern"); err != nil {
			return ToolResult{}, err
		}
		return executor.Grep(ctx, cwd, input)
	case ToolFind:
		var input FindInput
		if err := decodeToolInput(rawInput, &input, "pattern"); err != nil {
			return ToolResult{}, err
		}
		return executor.Find(ctx, cwd, input)
	case ToolLS:
		var input LSInput
		if err := decodeToolInput(rawInput, &input); err != nil {
			return ToolResult{}, err
		}
		return executor.LS(ctx, cwd, input)
	default:
		return ToolResult{}, fmt.Errorf("unknown tool name: %s", toolName)
	}
}

func decodeToolInput(rawInput json.RawMessage, target interface{}, requiredFields ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawInput, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("tool input must be a JSON object")
	}
	for _, field := range requiredFields {
		value, found := fields[field]
		if !found {
			return fmt.Errorf("missing required property: %s", field)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("property %s must not be null", field)
		}
	}
	return json.Unmarshal(rawInput, target)
}
