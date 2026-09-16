package handler

import (
	"os"
	"path/filepath"
	"testing"

	cderr "codedock/internal/errors"
)

func TestFreezeSessionWorkspace(t *testing.T) {
	dir := t.TempDir()
	got, err := freezeSessionWorkspace(dir, "/fallback")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(dir)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	if _, err := freezeSessionWorkspace(filepath.Join(dir, "missing"), dir); !cderr.IsInvalid(err) {
		t.Fatalf("missing dir: %v", err)
	}

	file := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := freezeSessionWorkspace(file, dir); !cderr.IsInvalid(err) {
		t.Fatalf("file: %v", err)
	}

	fallback, err := freezeSessionWorkspace("", dir)
	if err != nil || fallback != dir {
		t.Fatalf("empty %q %v", fallback, err)
	}
	fallback, err = freezeSessionWorkspace("default", dir)
	if err != nil || fallback != dir {
		t.Fatalf("default %q %v", fallback, err)
	}
	fallback, err = freezeSessionWorkspace("  ", "")
	if err != nil || fallback != "." {
		t.Fatalf("blank %q %v", fallback, err)
	}
}
