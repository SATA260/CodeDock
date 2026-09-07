package codex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	pkg "codedock/pkg/codex"
)

func TestDefaultLookPathAndCancelledStarter(t *testing.T) {
	if _, err := DefaultLookPath("true"); err != nil {
		t.Fatal(err)
	}
	_, _ = DefaultLookPath("")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DefaultStarter(ctx, "true"); err == nil {
		t.Fatal("expected canceled starter")
	}
}

func TestDefaultVersionerAndStarter(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	bin := filepath.Join(dir, "codex")
	code := `package main
import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("codex-cli 0.0.0-test")
		return
	}
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var msg map[string]any
		if json.Unmarshal(sc.Bytes(), &msg) != nil {
			continue
		}
		method, _ := msg["method"].(string)
		if method == "initialized" {
			continue
		}
		resp := map[string]any{"id": msg["id"], "result": map[string]any{}}
		if method == "initialize" {
			resp["result"] = map[string]any{"codexHome": "/tmp", "platformFamily": "unix", "platformOs": "linux", "userAgent": "t"}
		}
		if method == "account/read" {
			resp["result"] = map[string]any{"requiresOpenaiAuth": false, "account": map[string]any{"type": "apiKey"}}
		}
		body, _ := json.Marshal(resp)
		os.Stdout.Write(append(body, '\n'))
	}
}
`
	if err := os.WriteFile(src, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fakecodex\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake codex: %s %v", out, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	ver, err := DefaultVersioner(ctx, bin)
	if err != nil {
		t.Fatal(err)
	}
	if ParseVersion(ver) != "0.0.0-test" {
		t.Fatal(ver)
	}
	proc, err := DefaultStarter(ctx, bin)
	if err != nil {
		t.Fatal(err)
	}
	Drain(proc.Stderr())
	transport := pkg.NewJSONL(proc.Stdout(), proc.Stdin(), proc.Stdin(), 0)
	client := pkg.NewClient(transport)
	if _, err := client.Handshake(ctx, pkg.DefaultClientInfo()); err != nil {
		_ = proc.Kill()
		_ = client.Close()
		t.Fatal(err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	_ = proc.Wait()
}
