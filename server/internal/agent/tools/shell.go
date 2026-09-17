package tools

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"
)

const maxTimeoutSeconds = float64(2_147_483_647) / 1000

func (executor *Executor) Bash(ctx context.Context, cwd string, input ShellInput) (ToolResult, error) {
	executor = executor.withDefaults()
	shell, args, err := executor.resolveBash()
	if err != nil {
		return ToolResult{}, err
	}
	args = append(args, input.Command)
	return executor.runShell(ctx, cwd, input, shell, args, "bash", "pi-bash")
}

func (executor *Executor) PowerShell(ctx context.Context, cwd string, input ShellInput) (ToolResult, error) {
	executor = executor.withDefaults()
	if executor.GOOS != "windows" {
		return ToolResult{}, fmt.Errorf("The powershell tool is only available on Windows.")
	}
	shell, err := executor.LookPath("pwsh.exe")
	if err != nil {
		shell, err = executor.LookPath("powershell.exe")
	}
	if err != nil {
		return ToolResult{}, fmt.Errorf(
			"No PowerShell executable found. Install PowerShell or add powershell.exe/pwsh.exe to PATH.",
		)
	}
	command := "try { [Console]::OutputEncoding=[System.Text.Encoding]::UTF8 } catch {}\n" + input.Command
	args := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command}
	return executor.runShell(ctx, cwd, input, shell, args, "PowerShell", "pi-powershell")
}

func (executor *Executor) runShell(
	ctx context.Context,
	cwd string,
	input ShellInput,
	executable string,
	args []string,
	shellName string,
	tempPrefix string,
) (ToolResult, error) {
	if _, err := executor.FS.Stat(cwd); err != nil {
		return ToolResult{}, fmt.Errorf(
			"Working directory does not exist: %s\nCannot execute %s commands.",
			cwd,
			shellName,
		)
	}
	runContext := ctx
	cancel := func() {}
	if input.Timeout != nil {
		if math.IsNaN(*input.Timeout) || math.IsInf(*input.Timeout, 0) || *input.Timeout <= 0 {
			return ToolResult{}, fmt.Errorf("Invalid timeout: must be a finite number of seconds")
		}
		if *input.Timeout > maxTimeoutSeconds {
			return ToolResult{}, fmt.Errorf("Invalid timeout: maximum is %v seconds", maxTimeoutSeconds)
		}
		runContext, cancel = context.WithTimeout(ctx, time.Duration(*input.Timeout*float64(time.Second)))
	}
	defer cancel()

	output := newOutputAccumulator(executor, tempPrefix)
	commandResult, runErr := executor.RunCommand(
		runContext,
		executable,
		args,
		cwd,
		executor.Env,
		output.Append,
		output.Append,
	)
	emptyText := "(no output)"
	if runErr != nil || ctx.Err() != nil || errors.Is(runContext.Err(), context.DeadlineExceeded) {
		emptyText = ""
	}
	outputText, details, outputErr := output.Finish(emptyText)
	if outputErr != nil {
		return ToolResult{}, outputErr
	}
	appendStatus := func(status string) error {
		if outputText == "" {
			return errors.New(status)
		}
		return fmt.Errorf("%s\n\n%s", outputText, status)
	}

	switch {
	case ctx.Err() != nil:
		return ToolResult{}, appendStatus("Command aborted")
	case errors.Is(runContext.Err(), context.DeadlineExceeded):
		return ToolResult{}, appendStatus(
			fmt.Sprintf("Command timed out after %v seconds", *input.Timeout),
		)
	case runErr != nil:
		return ToolResult{}, runErr
	case commandResult.ExitCode != 0:
		return ToolResult{}, appendStatus(fmt.Sprintf("Command exited with code %d", commandResult.ExitCode))
	default:
		return textResult(outputText, details), nil
	}
}

func (executor *Executor) resolveBash() (string, []string, error) {
	if executor.GOOS != "windows" {
		if info, err := executor.FS.Stat("/bin/bash"); err == nil && !info.IsDir() {
			return "/bin/bash", []string{"-c"}, nil
		}
		if path, err := executor.LookPath("bash"); err == nil {
			return path, []string{"-c"}, nil
		}
		return "sh", []string{"-c"}, nil
	}

	for _, candidate := range []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Git", "bin", "bash.exe"),
	} {
		if candidate == "" {
			continue
		}
		if info, err := executor.FS.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, []string{"-c"}, nil
		}
	}
	if path, err := executor.LookPath("bash.exe"); err == nil {
		return path, []string{"-c"}, nil
	}
	return "", nil, fmt.Errorf("No bash shell found.")
}
