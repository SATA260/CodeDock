package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBashSuccessFailureTimeoutAndTruncation(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	executor := NewExecutor()
	executor.RunCommand = func(
		_ context.Context,
		_ string,
		args []string,
		_ string,
		_ []string,
		onStdout func([]byte),
		_ func([]byte),
	) (CommandResult, error) {
		if !slices.Equal(args, []string{"-c", "printf ok"}) {
			t.Fatalf("args = %v", args)
		}
		onStdout([]byte("ok"))
		return CommandResult{}, nil
	}
	result, err := executor.Bash(context.Background(), cwd, ShellInput{Command: "printf ok"})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultText(t, result); got != "ok" {
		t.Fatalf("bash = %q", got)
	}

	executor.RunCommand = func(
		context.Context,
		string,
		[]string,
		string,
		[]string,
		func([]byte),
		func([]byte),
	) (CommandResult, error) {
		return CommandResult{ExitCode: 2}, nil
	}
	_, err = executor.Bash(context.Background(), cwd, ShellInput{Command: "false"})
	if err == nil || err.Error() != "(no output)\n\nCommand exited with code 2" {
		t.Fatalf("error = %v", err)
	}

	timeout := 0.01
	executor.RunCommand = func(
		ctx context.Context,
		_ string,
		_ []string,
		_ string,
		_ []string,
		_ func([]byte),
		_ func([]byte),
	) (CommandResult, error) {
		<-ctx.Done()
		return CommandResult{}, ctx.Err()
	}
	_, err = executor.Bash(context.Background(), cwd, ShellInput{Command: "sleep", Timeout: &timeout})
	if err == nil || err.Error() != "Command timed out after 0.01 seconds" {
		t.Fatalf("error = %v", err)
	}

	largeOutput := []byte(strings.Repeat("line\n", DefaultMaxLines+1))
	executor.RunCommand = func(
		_ context.Context,
		_ string,
		_ []string,
		_ string,
		_ []string,
		onStdout func([]byte),
		_ func([]byte),
	) (CommandResult, error) {
		onStdout(largeOutput)
		return CommandResult{}, nil
	}
	result, err = executor.Bash(context.Background(), cwd, ShellInput{Command: "large"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Details == nil || result.Details.Truncation == nil || result.Details.FullOutputPath == "" {
		t.Fatalf("missing truncation details: %+v", result)
	}
	fullOutput, err := os.ReadFile(result.Details.FullOutputPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(result.Details.FullOutputPath) })
	if string(fullOutput) != string(largeOutput) {
		t.Fatalf("full output was not preserved")
	}
}

func TestPowerShellPlatformAndArguments(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	executor := NewExecutor()
	executor.GOOS = "darwin"
	if _, err := executor.PowerShell(context.Background(), cwd, ShellInput{Command: "Get-Date"}); err == nil ||
		err.Error() != "The powershell tool is only available on Windows." {
		t.Fatalf("error = %v", err)
	}

	executor.GOOS = "windows"
	executor.LookPath = func(file string) (string, error) {
		if file == "pwsh.exe" {
			return `C:\pwsh.exe`, nil
		}
		return "", errors.New("not found")
	}
	executor.RunCommand = func(
		_ context.Context,
		name string,
		args []string,
		_ string,
		_ []string,
		onStdout func([]byte),
		_ func([]byte),
	) (CommandResult, error) {
		if name != `C:\pwsh.exe` {
			t.Fatalf("name = %q", name)
		}
		if len(args) != 6 || args[4] != "-Command" ||
			!strings.Contains(args[5], "[Console]::OutputEncoding") ||
			!strings.HasSuffix(args[5], "Get-Date") {
			t.Fatalf("args = %v", args)
		}
		onStdout([]byte("date"))
		return CommandResult{}, nil
	}
	result, err := executor.PowerShell(context.Background(), cwd, ShellInput{Command: "Get-Date"})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultText(t, result); got != "date" {
		t.Fatalf("PowerShell = %q", got)
	}
}

func TestGrepFormattingContextAndLimit(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	path := filepath.Join(cwd, "sample.txt")
	if err := os.WriteFile(path, []byte("before\nmatch\nafter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	event := ripgrepEvent{Type: "match"}
	event.Data.Path.Text = path
	event.Data.Lines.Text = "match\n"
	event.Data.LineNumber = 2
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}

	executor := NewExecutor()
	executor.LookPath = func(file string) (string, error) { return "/usr/bin/" + file, nil }
	executor.RunCommand = func(
		_ context.Context,
		_ string,
		args []string,
		_ string,
		_ []string,
		onStdout func([]byte),
		_ func([]byte),
	) (CommandResult, error) {
		if !slices.Contains(args, "--json") || !slices.Contains(args, "--hidden") {
			t.Fatalf("args = %v", args)
		}
		payload := append(append(append([]byte(nil), encoded...), '\n'), encoded...)
		onStdout(append(payload, '\n'))
		return CommandResult{}, nil
	}
	contextLines, limit := 1.0, 1.0
	result, err := executor.Grep(context.Background(), cwd, GrepInput{
		Pattern: "match",
		Context: &contextLines,
		Limit:   &limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := "sample.txt-1- before\nsample.txt:2: match\nsample.txt-3- after"
	if !strings.HasPrefix(resultText(t, result), wantPrefix) {
		t.Fatalf("grep = %q", resultText(t, result))
	}
	if result.Details == nil || result.Details.MatchLimitReached == nil ||
		*result.Details.MatchLimitReached != 1 {
		t.Fatalf("missing match limit: %+v", result)
	}

	fractionalLimit := 1.5
	result, err = executor.Grep(context.Background(), cwd, GrepInput{
		Pattern: "match",
		Limit:   &fractionalLimit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Details == nil || result.Details.MatchLimitReached == nil ||
		*result.Details.MatchLimitReached != fractionalLimit {
		t.Fatalf("missing fractional match limit: %+v", result)
	}
	if got := strings.Count(resultText(t, result), "sample.txt:2: match"); got != 2 {
		t.Fatalf("fractional grep returned %d matches, want 2: %q", got, resultText(t, result))
	}
}

func TestGrepNoMatches(t *testing.T) {
	t.Parallel()
	executor := NewExecutor()
	executor.LookPath = func(file string) (string, error) { return file, nil }
	executor.RunCommand = func(
		context.Context,
		string,
		[]string,
		string,
		[]string,
		func([]byte),
		func([]byte),
	) (CommandResult, error) {
		return CommandResult{ExitCode: 1}, nil
	}
	result, err := executor.Grep(context.Background(), t.TempDir(), GrepInput{Pattern: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultText(t, result); got != "No matches found" {
		t.Fatalf("grep = %q", got)
	}
}

func TestFindPathGlobRelativizationAndLimit(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor()
	executor.LookPath = func(file string) (string, error) { return "/usr/bin/" + file, nil }
	wantMaxResults := "2"
	executor.RunCommand = func(
		_ context.Context,
		_ string,
		args []string,
		_ string,
		_ []string,
		onStdout func([]byte),
		_ func([]byte),
	) (CommandResult, error) {
		if !slices.Contains(args, "--full-path") || slices.Contains(args, "--no-require-git") {
			t.Fatalf("args = %v", args)
		}
		maxResultsIndex := slices.Index(args, "--max-results")
		if maxResultsIndex < 0 || args[maxResultsIndex+1] != wantMaxResults {
			t.Fatalf("max-results args = %v, want %s", args, wantMaxResults)
		}
		patternIndex := len(args) - 2
		if args[patternIndex] != "**/src/**/*.test.ts" {
			t.Fatalf("pattern = %q", args[patternIndex])
		}
		output := strings.Join([]string{
			filepath.Join(cwd, "src", "a.test.ts"),
			filepath.Join(cwd, "src", "nested", "b.test.ts"),
		}, "\n")
		onStdout([]byte(output))
		return CommandResult{}, nil
	}
	limit := 2.0
	result, err := executor.Find(context.Background(), cwd, FindInput{
		Pattern: "src/**/*.test.ts",
		Limit:   &limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resultText(t, result), "src/a.test.ts\nsrc/nested/b.test.ts") {
		t.Fatalf("find = %q", resultText(t, result))
	}
	if result.Details == nil || result.Details.ResultLimitReached == nil ||
		*result.Details.ResultLimitReached != 2 {
		t.Fatalf("missing result limit: %+v", result)
	}

	fractionalLimit := 1.5
	wantMaxResults = "1.5"
	result, err = executor.Find(context.Background(), cwd, FindInput{
		Pattern: "src/**/*.test.ts",
		Limit:   &fractionalLimit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Details == nil || result.Details.ResultLimitReached == nil ||
		*result.Details.ResultLimitReached != fractionalLimit {
		t.Fatalf("missing fractional result limit: %+v", result)
	}
}

func TestFindSurfacesStderrOnFailure(t *testing.T) {
	t.Parallel()
	executor := NewExecutor()
	executor.LookPath = func(file string) (string, error) { return file, nil }
	executor.RunCommand = func(
		_ context.Context,
		_ string,
		_ []string,
		_ string,
		_ []string,
		_ func([]byte),
		onStderr func([]byte),
	) (CommandResult, error) {
		onStderr([]byte("invalid glob"))
		return CommandResult{ExitCode: 2}, nil
	}
	_, err := executor.Find(context.Background(), t.TempDir(), FindInput{Pattern: "["})
	if err == nil || err.Error() != "invalid glob" {
		t.Fatalf("error = %v", err)
	}
}

func TestShellAbort(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	executor := NewExecutor()
	executor.RunCommand = func(
		ctx context.Context,
		_ string,
		_ []string,
		_ string,
		_ []string,
		onStdout func([]byte),
		_ func([]byte),
	) (CommandResult, error) {
		<-ctx.Done()
		onStdout([]byte("partial"))
		return CommandResult{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(time.Millisecond)
		cancel()
	}()
	_, err := executor.Bash(ctx, cwd, ShellInput{Command: "wait"})
	if err == nil || err.Error() != "partial\n\nCommand aborted" {
		t.Fatalf("error = %v", err)
	}
}

func TestDefaultBashAndGrepBackends(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell integration test")
	}
	executor := NewExecutor()
	cwd := t.TempDir()
	result, err := executor.Bash(context.Background(), cwd, ShellInput{Command: "printf integration"})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultText(t, result); got != "integration" {
		t.Fatalf("bash = %q", got)
	}

	if _, err := executor.LookPath("rg"); err != nil {
		t.Skip("ripgrep is not installed")
	}
	if err := os.WriteFile(filepath.Join(cwd, "sample.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = executor.Grep(context.Background(), cwd, GrepInput{Pattern: "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultText(t, result); got != "sample.txt:1: needle" {
		t.Fatalf("grep = %q", got)
	}
}

func TestDefaultBashTimeoutKillsProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix process-group integration test")
	}
	timeout := 0.05
	startedAt := time.Now()
	_, err := NewExecutor().Bash(
		context.Background(),
		t.TempDir(),
		ShellInput{Command: "sleep 10 & wait", Timeout: &timeout},
	)
	if err == nil || !strings.Contains(err.Error(), "Command timed out after 0.05 seconds") {
		t.Fatalf("error = %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("process tree took %s to terminate", elapsed)
	}
}

func TestGrepFindAndReadMore(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "a.txt"), []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exec := NewExecutor()
	exec.LookPath = func(file string) (string, error) {
		if file == "rg" || file == "fd" {
			return file, nil
		}
		return "", os.ErrNotExist
	}
	path := cwd
	limit := 1.0
	ctxLines := 1.0
	ignore := true
	literal := true
	glob := "*.txt"
	exec.RunCommand = func(_ context.Context, name string, args []string, dir string, env []string, onStdout, onStderr func([]byte)) (CommandResult, error) {
		if name == "rg" {
			onStdout([]byte(`{"type":"begin"}` + "\n"))
			onStdout([]byte(`{"type":"match","data":{"path":{"text":"a.txt"},"lines":{"text":"hello"},"line_number":1}}` + "\n"))
			onStdout([]byte(`{"type":"match","data":{"path":{"text":"a.txt"},"lines":{"text":"world"},"line_number":2}}` + "\n"))
			return CommandResult{}, nil
		}
		onStdout([]byte(filepath.Join(cwd, "a.txt") + "\n"))
		onStdout([]byte("sub/b.txt\n"))
		return CommandResult{}, nil
	}
	if _, err := exec.Grep(context.Background(), cwd, GrepInput{
		Pattern: "hello", Path: &path, Glob: &glob, IgnoreCase: &ignore, Literal: &literal, Context: &ctxLines, Limit: &limit,
	}); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(cwd, "nope")
	if _, err := exec.Grep(context.Background(), cwd, GrepInput{Pattern: "x", Path: &missing}); err == nil {
		t.Fatal("missing path")
	}
	slash := "dir/a.txt"
	if _, err := exec.Find(context.Background(), cwd, FindInput{Pattern: slash, Path: &path, Limit: &limit}); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.Find(context.Background(), cwd, FindInput{Pattern: "*", Path: &missing}); err == nil {
		t.Fatal("find missing")
	}

	noRG := NewExecutor()
	noRG.LookPath = func(string) (string, error) { return "", os.ErrNotExist }
	if _, err := noRG.Grep(context.Background(), cwd, GrepInput{Pattern: "x"}); err == nil {
		t.Fatal("rg missing")
	}
	if _, err := noRG.Find(context.Background(), cwd, FindInput{Pattern: "*"}); err == nil {
		t.Fatal("fd missing")
	}

	offset := 99.0
	if _, err := exec.Read(context.Background(), cwd, ReadInput{Path: "a.txt", Offset: &offset}); err == nil {
		t.Fatal("offset")
	}
	if _, err := exec.Read(context.Background(), cwd, ReadInput{Path: "missing.txt"}); err == nil {
		t.Fatal("missing file")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := exec.Read(canceled, cwd, ReadInput{Path: "a.txt"}); err == nil {
		t.Fatal("cancel")
	}

	big := image.NewRGBA(image.Rect(0, 0, 2100, 80))
	var buf bytes.Buffer
	if err := png.Encode(&buf, big); err != nil {
		t.Fatal(err)
	}
	if _, err := processImage(buf.Bytes(), "image/png"); err != nil {
		t.Fatal(err)
	}
	if detectSupportedImageMIME([]byte{0xff, 0xd8, 0xff, 0xf7}) != "" {
		t.Fatal("jpeg f7")
	}
	if detectSupportedImageMIME([]byte("GIF89a")) != "image/gif" {
		t.Fatal("gif")
	}

	failRG := NewExecutor()
	failRG.LookPath = func(string) (string, error) { return "rg", nil }
	failRG.RunCommand = func(context.Context, string, []string, string, []string, func([]byte), func([]byte)) (CommandResult, error) {
		return CommandResult{}, os.ErrPermission
	}
	if _, err := failRG.Grep(context.Background(), cwd, GrepInput{Pattern: "x"}); err == nil {
		t.Fatal("rg run")
	}
	codeRG := NewExecutor()
	codeRG.LookPath = func(string) (string, error) { return "rg", nil }
	codeRG.RunCommand = func(_ context.Context, name string, args []string, dir string, env []string, onStdout, onStderr func([]byte)) (CommandResult, error) {
		onStderr([]byte("bad pattern"))
		return CommandResult{ExitCode: 2}, nil
	}
	if _, err := codeRG.Grep(context.Background(), cwd, GrepInput{Pattern: "x"}); err == nil {
		t.Fatal("rg exit")
	}
	silentRG := NewExecutor()
	silentRG.LookPath = func(string) (string, error) { return "rg", nil }
	silentRG.RunCommand = func(context.Context, string, []string, string, []string, func([]byte), func([]byte)) (CommandResult, error) {
		return CommandResult{ExitCode: 3}, nil
	}
	if _, err := silentRG.Grep(context.Background(), cwd, GrepInput{Pattern: "x"}); err == nil {
		t.Fatal("rg silent exit")
	}
	abort, abortCancel := context.WithCancel(context.Background())
	abortRG := NewExecutor()
	abortRG.LookPath = func(string) (string, error) { return "rg", nil }
	abortRG.RunCommand = func(context.Context, string, []string, string, []string, func([]byte), func([]byte)) (CommandResult, error) {
		abortCancel()
		return CommandResult{}, nil
	}
	if _, err := abortRG.Grep(abort, cwd, GrepInput{Pattern: "x"}); err == nil {
		t.Fatal("rg abort")
	}
	longLine := string(bytes.Repeat([]byte("x"), GrepMaxLineLength+8))
	truncRG := NewExecutor()
	truncRG.LookPath = func(string) (string, error) { return "rg", nil }
	truncRG.RunCommand = func(_ context.Context, name string, args []string, dir string, env []string, onStdout, onStderr func([]byte)) (CommandResult, error) {
		onStdout([]byte(`{"type":"match","data":{"path":{"text":"` + filepath.Join(cwd, "a.txt") + `"},"lines":{"text":"` + longLine + `"},"line_number":1}}` + "\n"))
		return CommandResult{}, nil
	}
	if _, err := truncRG.Grep(context.Background(), cwd, GrepInput{Pattern: "x"}); err != nil {
		t.Fatal(err)
	}
	badURL := "file://%zz"
	if _, err := exec.Grep(context.Background(), cwd, GrepInput{Pattern: "x", Path: &badURL}); err == nil {
		t.Fatal("bad path")
	}
	if _, err := exec.Find(context.Background(), cwd, FindInput{Pattern: "*", Path: &badURL}); err == nil {
		t.Fatal("find bad path")
	}
	failFD := NewExecutor()
	failFD.LookPath = func(string) (string, error) { return "fd", nil }
	failFD.RunCommand = func(context.Context, string, []string, string, []string, func([]byte), func([]byte)) (CommandResult, error) {
		return CommandResult{}, os.ErrPermission
	}
	if _, err := failFD.Find(context.Background(), cwd, FindInput{Pattern: "*"}); err == nil {
		t.Fatal("fd run")
	}
	codeFD := NewExecutor()
	codeFD.LookPath = func(string) (string, error) { return "fd", nil }
	codeFD.RunCommand = func(context.Context, string, []string, string, []string, func([]byte), func([]byte)) (CommandResult, error) {
		return CommandResult{ExitCode: 2}, nil
	}
	if _, err := codeFD.Find(context.Background(), cwd, FindInput{Pattern: "*"}); err == nil {
		t.Fatal("fd exit")
	}
	abortFD, abortFDCancel := context.WithCancel(context.Background())
	runFD := NewExecutor()
	runFD.LookPath = func(string) (string, error) { return "fd", nil }
	runFD.RunCommand = func(context.Context, string, []string, string, []string, func([]byte), func([]byte)) (CommandResult, error) {
		abortFDCancel()
		return CommandResult{}, nil
	}
	if _, err := runFD.Find(abortFD, cwd, FindInput{Pattern: "*"}); err == nil {
		t.Fatal("fd abort")
	}
	lim := 1.0
	limitFD := NewExecutor()
	limitFD.LookPath = func(string) (string, error) { return "fd", nil }
	limitFD.GOOS = "windows"
	limitFD.RunCommand = func(_ context.Context, name string, args []string, dir string, env []string, onStdout, onStderr func([]byte)) (CommandResult, error) {
		onStdout([]byte(cwd + string(filepath.Separator) + "\n"))
		onStdout([]byte(filepath.Join(cwd, "a.txt") + "\n"))
		return CommandResult{}, nil
	}
	if _, err := limitFD.Find(context.Background(), cwd, FindInput{Pattern: "dir/a", Path: &path, Limit: &lim}); err != nil {
		t.Fatal(err)
	}
	hugeFD := NewExecutor()
	hugeFD.LookPath = func(string) (string, error) { return "fd", nil }
	hugeFD.RunCommand = func(_ context.Context, name string, args []string, dir string, env []string, onStdout, onStderr func([]byte)) (CommandResult, error) {
		onStdout(bytes.Repeat([]byte("xxxxxxxxxxxxxxxx\n"), 4000))
		return CommandResult{}, nil
	}
	if _, err := hugeFD.Find(context.Background(), cwd, FindInput{Pattern: "*"}); err != nil {
		t.Fatal(err)
	}
}
