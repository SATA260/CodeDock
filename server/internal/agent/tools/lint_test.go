package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInspectEditRejectsBrokenGo 新引入的 Go 语法错误必须被检查出来。
func TestInspectEditRejectsBrokenGo(t *testing.T) {
	old := "package p\n\nfunc Ok() {}\n"
	bad := "package p\n\nfunc Ok( {\n"
	got, err := InspectEdit("demo.go", old, bad)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.NewErrors) == 0 {
		t.Fatal("expected new syntax errors")
	}
}

// TestApplyEditGuardRollsBackBrokenWrite 写坏后文件必须恢复原样。
func TestApplyEditGuardRollsBackBrokenWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.go")
	old := "package p\n\nfunc Ok() int { return 1 }\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := osFileSystem{}
	if err := fs.WriteFile(path, []byte("package p\n\nfunc Ok( {\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := applyEditGuard(fs, nil, path, old, "package p\n\nfunc Ok( {\n", true)
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("err=%v", err)
	}
	body, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(body) != old {
		t.Fatalf("file=%q", body)
	}
}

// TestIsExistingTestFile 只有已存在的测试文件才锁定审批。
func TestIsExistingTestFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo_test.go")
	if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsExistingTestFile(dir, "foo_test.go") {
		t.Fatal("existing test should lock")
	}
	if IsExistingTestFile(dir, "new_test.go") {
		t.Fatal("missing test should not lock")
	}
	if IsExistingTestFile(dir, "main.go") {
		t.Fatal("non-test should not lock")
	}
}
