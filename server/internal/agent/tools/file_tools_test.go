package tools

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/image/bmp"
	"golang.org/x/text/unicode/norm"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func resultText(t *testing.T, result ToolResult) string {
	t.Helper()
	if len(result.Content) == 0 || result.Content[0].Type != "text" {
		t.Fatalf("missing text result: %+v", result)
	}
	return result.Content[0].Text
}

func TestReadTextOffsetLimitAndImage(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "sample.txt"), []byte("one\ntwo\nthree"), 0o644); err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor()
	result, err := executor.Read(context.Background(), cwd, ReadInput{Path: "sample.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultText(t, result); got != "one\ntwo\nthree" {
		t.Fatalf("read = %q", got)
	}

	offset, limit := 2.0, 1.0
	result, err = executor.Read(
		context.Background(),
		cwd,
		ReadInput{Path: "sample.txt", Offset: &offset, Limit: &limit},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := resultText(t, result), "two\n\n[1 more lines in file. Use offset=3 to continue.]"; got != want {
		t.Fatalf("read = %q, want %q", got, want)
	}

	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "image.bin"), pngData.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = executor.Read(context.Background(), cwd, ReadInput{Path: "image.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 2 || result.Content[1].Type != "image" || result.Content[1].MIMEType != "image/png" {
		t.Fatalf("unexpected image result: %+v", result)
	}

	fractionalOffset, fractionalLimit := 2.5, 1.5
	result, err = executor.Read(context.Background(), cwd, ReadInput{
		Path:   "sample.txt",
		Offset: &fractionalOffset,
		Limit:  &fractionalLimit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := resultText(t, result), "two\nthree"; got != want {
		t.Fatalf("fractional read = %q, want %q", got, want)
	}
}

func TestReadImageValidationConversionAndResize(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	executor := NewExecutor()

	malformedPNG := append([]byte("\x89PNG\r\n\x1a\n"), []byte("payload")...)
	if err := os.WriteFile(filepath.Join(cwd, "malformed.png"), malformedPNG, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := executor.Read(context.Background(), cwd, ReadInput{Path: "malformed.png"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("malformed PNG treated as image: %+v", result)
	}

	var validPNG bytes.Buffer
	if err := png.Encode(&validPNG, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	apngChunk := []byte{0, 0, 0, 0, 'a', 'c', 'T', 'L', 0, 0, 0, 0}
	apng := append([]byte(nil), validPNG.Bytes()[:33]...)
	apng = append(apng, apngChunk...)
	apng = append(apng, validPNG.Bytes()[33:]...)
	if err := os.WriteFile(filepath.Join(cwd, "animated.png"), apng, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = executor.Read(context.Background(), cwd, ReadInput{Path: "animated.png"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("APNG treated as image: %+v", result)
	}

	source := image.NewRGBA(image.Rect(0, 0, 2, 1))
	source.Set(0, 0, color.RGBA{R: 255, A: 255})
	var bmpData bytes.Buffer
	if err := bmp.Encode(&bmpData, source); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "image.bmp"), bmpData.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = executor.Read(context.Background(), cwd, ReadInput{Path: "image.bmp"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 2 || result.Content[1].MIMEType != "image/png" ||
		!strings.Contains(result.Content[0].Text, "converted from image/bmp to image/png") {
		t.Fatalf("unexpected BMP result: %+v", result)
	}

	oversized := image.NewRGBA(image.Rect(0, 0, maxImageWidth+1, 1))
	var oversizedPNG bytes.Buffer
	if err := png.Encode(&oversizedPNG, oversized); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "oversized.png"), oversizedPNG.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = executor.Read(context.Background(), cwd, ReadInput{Path: "oversized.png"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 2 ||
		!strings.Contains(result.Content[0].Text, "original 2001x1, displayed at 2000x1") {
		t.Fatalf("unexpected resized image result: %+v", result)
	}
}

func TestReadTruncationAndOffsetError(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	longLine := strings.Repeat("x", DefaultMaxBytes+1)
	if err := os.WriteFile(filepath.Join(cwd, "large.txt"), []byte(longLine), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewExecutor().Read(context.Background(), cwd, ReadInput{Path: "large.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Details == nil || result.Details.Truncation == nil ||
		!result.Details.Truncation.FirstLineExceedsLimit {
		t.Fatalf("missing truncation details: %+v", result)
	}
	if !strings.Contains(resultText(t, result), "exceeds 50.0KB limit") {
		t.Fatalf("unexpected output: %q", resultText(t, result))
	}

	offset := 2.0
	_, err = NewExecutor().Read(
		context.Background(),
		cwd,
		ReadInput{Path: "large.txt", Offset: &offset},
	)
	if err == nil || err.Error() != "Offset 2 is beyond end of file (1 lines total)" {
		t.Fatalf("error = %v", err)
	}

	many := strings.Repeat("line\n", DefaultMaxLines+20)
	if err := os.WriteFile(filepath.Join(cwd, "lines.txt"), []byte(many), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = NewExecutor().Read(context.Background(), cwd, ReadInput{Path: "lines.txt"})
	if err != nil || !strings.Contains(resultText(t, result), "Use offset=") {
		t.Fatalf("line truncate %v %q", err, resultText(t, result))
	}
	chunk := strings.Repeat("abcdefghij", 80)
	var fat strings.Builder
	for i := 0; i < 80; i++ {
		fat.WriteString(chunk)
		fat.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(cwd, "fat.txt"), []byte(fat.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = NewExecutor().Read(context.Background(), cwd, ReadInput{Path: "fat.txt"})
	if err != nil || result.Details == nil || result.Details.Truncation == nil {
		t.Fatalf("byte truncate %v %+v", err, result.Details)
	}

	var bmpData bytes.Buffer
	if err := bmp.Encode(&bmpData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "tiny.bmp"), bmpData.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = NewExecutor().Read(context.Background(), cwd, ReadInput{Path: "tiny.bmp"})
	if err != nil || len(result.Content) < 2 {
		t.Fatalf("bmp %v %+v", err, result)
	}
	bad := "file://%zz"
	if _, err := NewExecutor().Read(context.Background(), cwd, ReadInput{Path: bad}); err == nil {
		t.Fatal("bad url")
	}
	neg := -1.0
	inf := math.Inf(1)
	_ = jsSliceIndex(math.NaN(), 3)
	_ = jsSliceIndex(inf, 3)
	_ = jsSliceIndex(math.Inf(-1), 3)
	if _, err := NewExecutor().Read(context.Background(), cwd, ReadInput{Path: "lines.txt", Offset: &neg, Limit: &inf}); err != nil {
		t.Fatal(err)
	}
}

func TestWriteCreatesDirectoriesAndUsesJSStringLength(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	result, err := NewExecutor().Write(
		context.Background(),
		cwd,
		WriteInput{Path: "nested/file.txt", Content: "😀"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := resultText(t, result); got != "Successfully wrote 2 bytes to nested/file.txt" {
		t.Fatalf("result = %q", got)
	}
	if _, err := NewExecutor().Write(context.Background(), cwd, WriteInput{Path: "file://%zz", Content: "x"}); err == nil {
		t.Fatal("bad write path")
	}
	data, err := os.ReadFile(filepath.Join(cwd, "nested", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "😀" {
		t.Fatalf("file = %q", data)
	}
}

func TestEditMultipleBlocksAndLegacyJSON(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	path := filepath.Join(cwd, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha\nmiddle\nomega\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewExecutor().Edit(context.Background(), cwd, EditInput{
		Path: "sample.txt",
		Edits: []EditReplacement{
			{OldText: "alpha", NewText: "ALPHA"},
			{OldText: "omega", NewText: "OMEGA"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Details == nil || result.Details.Diff == "" || result.Details.Patch == "" ||
		result.Details.FirstChangedLine == nil {
		t.Fatalf("missing edit details: %+v", result)
	}
	if got := resultText(t, result); got != "Successfully replaced 2 block(s) in sample.txt." {
		t.Fatalf("result = %q", got)
	}

	result, err = NewExecutor().Execute(
		context.Background(),
		ToolEdit,
		cwd,
		json.RawMessage(`{"path":"sample.txt","oldText":"middle","newText":"CENTER"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ALPHA\nCENTER\nOMEGA\n" {
		t.Fatalf("file = %q", data)
	}
}

func TestEditFuzzyMatchPreservesBOMAndCRLF(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	path := filepath.Join(cwd, "fuzzy.txt")
	original := append([]byte{0xef, 0xbb, 0xbf}, []byte("keep  \r\nIt’s fine\r\n")...)
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewExecutor().Edit(context.Background(), cwd, EditInput{
		Path: "fuzzy.txt",
		Edits: []EditReplacement{{
			OldText: "It's fine",
			NewText: "It works",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte{0xef, 0xbb, 0xbf}, []byte("keep  \r\nIt works\r\n")...)
	if string(data) != string(want) {
		t.Fatalf("file = %q, want %q", data, want)
	}
}

func TestEditErrorsDoNotPartiallyWrite(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	path := filepath.Join(cwd, "sample.txt")
	original := "one\ntwo\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewExecutor().Edit(context.Background(), cwd, EditInput{
		Path: "sample.txt",
		Edits: []EditReplacement{
			{OldText: "one", NewText: "ONE"},
			{OldText: "missing", NewText: "MISSING"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "Could not find edits[1]") {
		t.Fatalf("error = %v", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != original {
		t.Fatalf("file changed after failed edit: %q", data)
	}
	if _, err := decodeEditInput(json.RawMessage(`[]`)); err == nil {
		t.Fatal("not object")
	}
	if _, err := decodeEditInput(json.RawMessage(`{}`)); err == nil {
		t.Fatal("missing path")
	}
	if _, err := NewExecutor().Edit(context.Background(), cwd, EditInput{Path: "sample.txt"}); err == nil {
		t.Fatal("empty edits")
	}
	if _, err := NewExecutor().Edit(context.Background(), cwd, EditInput{Path: "file://%zz", Edits: []EditReplacement{{OldText: "a", NewText: "b"}}}); err == nil {
		t.Fatal("bad path")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewExecutor().Edit(canceled, cwd, EditInput{Path: "sample.txt", Edits: []EditReplacement{{OldText: "one", NewText: "ONE"}}}); err == nil {
		t.Fatal("cancel")
	}
	if formatFileError(errors.New("plain")) == "" {
		t.Fatal("format")
	}
}

type deniedFileSystem struct {
	FileSystem
}

func (deniedFileSystem) Access(string) error {
	return fs.ErrPermission
}

func TestEditNormalizesPermissionErrors(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	path := filepath.Join(cwd, "sample.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	executor := &Executor{FS: deniedFileSystem{FileSystem: osFileSystem{}}}
	_, err := executor.Edit(context.Background(), cwd, EditInput{
		Path:  "sample.txt",
		Edits: []EditReplacement{{OldText: "before", NewText: "after"}},
	})
	if err == nil || err.Error() != "Could not edit file: sample.txt. Error code: EACCES." {
		t.Fatalf("error = %v", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "before" {
		t.Fatalf("file changed after access failure: %q", data)
	}
}

func TestEditDiffPreservesFinalNewlineChanges(t *testing.T) {
	t.Parallel()
	diff, firstChangedLine := generateDiffString("a", "a\n", 4)
	if firstChangedLine == nil || *firstChangedLine != 1 || !strings.Contains(diff, "-1 a") ||
		!strings.Contains(diff, "+1 a") {
		t.Fatalf("diff = %q, firstChangedLine = %v", diff, firstChangedLine)
	}
	patch := generateUnifiedPatch("sample.txt", "a", "a\n")
	if !strings.Contains(patch, "\\ No newline at end of file") {
		t.Fatalf("patch = %q", patch)
	}
}

func TestEditDiffHandlesLargeFilesWithoutQuadraticMemory(t *testing.T) {
	t.Parallel()
	oldLines := make([]string, 10_000)
	for index := range oldLines {
		oldLines[index] = "unchanged"
	}
	newLines := append([]string(nil), oldLines...)
	newLines[5_000] = "changed"
	diff, firstChangedLine := generateDiffString(
		strings.Join(oldLines, "\n"),
		strings.Join(newLines, "\n"),
		4,
	)
	if firstChangedLine == nil || *firstChangedLine != 5_001 || !strings.Contains(diff, "+ 5001 changed") {
		t.Fatalf("unexpected large diff first line=%v, diff=%q", firstChangedLine, diff)
	}
}

func TestLS(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	for _, name := range []string{"b.txt", "A.txt", ".hidden"} {
		if err := os.WriteFile(filepath.Join(cwd, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(cwd, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := NewExecutor().LS(context.Background(), cwd, LSInput{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := resultText(t, result), ".hidden\nA.txt\nb.txt\ndir/"; got != want {
		t.Fatalf("ls = %q, want %q", got, want)
	}
	bad := "file://%zz"
	if _, err := NewExecutor().LS(context.Background(), cwd, LSInput{Path: &bad}); err == nil {
		t.Fatal("bad ls path")
	}
	file := "b.txt"
	if _, err := NewExecutor().LS(context.Background(), cwd, LSInput{Path: &file}); err == nil {
		t.Fatal("not dir")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewExecutor().LS(canceled, cwd, LSInput{}); err == nil {
		t.Fatal("ls cancel")
	}
	fat := t.TempDir()
	for i := 0; i < 300; i++ {
		name := fmt.Sprintf("%03d-%s", i, strings.Repeat("n", 180))
		if err := os.WriteFile(filepath.Join(fat, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewExecutor().LS(context.Background(), fat, LSInput{}); err != nil {
		t.Fatal(err)
	}
}

func TestLSUsesUnicodeCollation(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	for _, name := range []string{"z", "ä", "2", "10", "a"} {
		if err := os.WriteFile(filepath.Join(cwd, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	result, err := NewExecutor().LS(context.Background(), cwd, LSInput{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := resultText(t, result), "10\n2\na\nä\nz"; got != want {
		t.Fatalf("ls = %q, want %q", got, want)
	}

	limit := 1.5
	result, err = NewExecutor().LS(context.Background(), cwd, LSInput{Limit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := resultText(t, result), "10\n2\n\n[1.5 entries limit reached. Use limit=3 for more]"; got != want {
		t.Fatalf("fractional ls = %q, want %q", got, want)
	}
	if result.Details == nil || result.Details.EntryLimitReached == nil ||
		*result.Details.EntryLimitReached != limit {
		t.Fatalf("missing fractional entry limit: %+v", result)
	}
}

type slowFileSystem struct {
	active    atomic.Int32
	maxActive atomic.Int32
}

func (fileSystem *slowFileSystem) Access(name string) error {
	file, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return file.Close()
}

func (fileSystem *slowFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (fileSystem *slowFileSystem) WriteFile(name string, data []byte, perm fs.FileMode) error {
	active := fileSystem.active.Add(1)
	for {
		current := fileSystem.maxActive.Load()
		if active <= current || fileSystem.maxActive.CompareAndSwap(current, active) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
	err := os.WriteFile(name, data, perm)
	fileSystem.active.Add(-1)
	return err
}

func (fileSystem *slowFileSystem) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (fileSystem *slowFileSystem) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (fileSystem *slowFileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

func (fileSystem *slowFileSystem) EvalSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

func (fileSystem *slowFileSystem) Remove(name string) error {
	return os.Remove(name)
}

func TestFileMutationsAreSerializedPerPath(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	fileSystem := &slowFileSystem{}
	executor := &Executor{FS: fileSystem}
	var waitGroup sync.WaitGroup
	for index := range 3 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, err := executor.Write(
				context.Background(),
				cwd,
				WriteInput{Path: "same.txt", Content: string(rune('a' + index))},
			)
			if err != nil {
				t.Errorf("write failed: %v", err)
			}
		}()
	}
	waitGroup.Wait()
	if got := fileSystem.maxActive.Load(); got != 1 {
		t.Fatalf("max concurrent writes = %d, want 1", got)
	}
}

func TestResolveToCWDNormalizesToolPaths(t *testing.T) {
	t.Parallel()
	executor := NewExecutor()
	executor.HomeDir = filepath.Join(t.TempDir(), "home")
	got, err := executor.resolveToCWD("@dir\u202ffile.txt", "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/tmp", "dir file.txt"); got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}
	got, err = executor.resolveToCWD("~/file.txt", "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(executor.HomeDir, "file.txt"); got != want {
		t.Fatalf("resolved tilde path = %q, want %q", got, want)
	}
}

func TestResolveReadPathFallsBackToMacOSVariants(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	executor := NewExecutor()

	curlyName := "Capture d\u2019écran.txt"
	nfdName := norm.NFD.String(curlyName)
	if err := os.WriteFile(filepath.Join(cwd, nfdName), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := executor.resolveReadPath("Capture d'écran.txt", cwd)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		t.Fatalf("resolved path %q is not readable: %v", resolved, err)
	}
	if string(data) != "ok" || filepath.Base(resolved) == "Capture d'écran.txt" {
		t.Fatalf("fallback did not resolve variant: %q", resolved)
	}
}

func TestResolveFileURL(t *testing.T) {
	t.Parallel()
	executor := NewExecutor()
	resolved, err := executor.resolveToCWD("file:///tmp/a%20b.txt", "/unused")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != filepath.Join("/tmp", "a b.txt") {
		t.Fatalf("resolved path = %q", resolved)
	}
	if _, err := executor.resolveToCWD("file:///tmp/%zz", "/unused"); err == nil {
		t.Fatal("expected invalid file URL to fail")
	}
	got, err := executor.resolveToCWD("~", "/unused")
	if err != nil || got != executor.HomeDir {
		t.Fatalf("tilde %q %v", got, err)
	}
	if _, err := executor.resolveToCWD("file://otherhost/tmp/a", "/unused"); err == nil {
		t.Fatal("file host")
	}
	executor.GOOS = "windows"
	got, err = executor.resolveToCWD(`~\file.txt`, "/unused")
	if err != nil {
		t.Fatal(err)
	}
	_ = got
	got, err = executor.resolveToCWD("file://server/share/a", "/unused")
	if err != nil {
		t.Fatal(err)
	}
	_ = got
	got, err = executor.resolveToCWD("file:///C:/tmp/a", "/unused")
	if err != nil {
		t.Fatal(err)
	}
	_ = normalizeWindowsShellPath("/mnt/c/tmp")
	_ = normalizeWindowsShellPath("/")
}

func TestTruncateHeadByLines(t *testing.T) {
	t.Parallel()
	result := TruncateHead("a\nb\nc", 2, 100)
	if !result.Truncated || result.TruncatedBy == nil || *result.TruncatedBy != "lines" {
		t.Fatalf("unexpected truncation: %+v", result)
	}
	if result.Content != "a\nb" || result.TotalLines != 3 || result.OutputLines != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestTruncateHeadByBytesAndOversizedFirstLine(t *testing.T) {
	t.Parallel()
	result := TruncateHead("abcd\nef", 10, 3)
	if !result.FirstLineExceedsLimit || result.Content != "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.TruncatedBy == nil || *result.TruncatedBy != "bytes" {
		t.Fatalf("unexpected truncation kind: %+v", result)
	}

	result = TruncateHead("ab\ncd\nef", 10, 5)
	if result.Content != "ab\ncd" || result.OutputBytes != 5 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestTruncateTail(t *testing.T) {
	t.Parallel()
	result := TruncateTail("a\nb\nc\nd", 2, 100)
	if result.Content != "c\nd" || result.OutputLines != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.TruncatedBy == nil || *result.TruncatedBy != "lines" {
		t.Fatalf("unexpected truncation kind: %+v", result)
	}

	result = TruncateTail("abcdef", 10, 3)
	if result.Content != "def" || !result.LastLinePartial {
		t.Fatalf("unexpected partial-line result: %+v", result)
	}
}

func TestTruncationCountsTrailingNewlineLikeTypeScript(t *testing.T) {
	t.Parallel()
	result := TruncateHead("a\nb\n", 10, 100)
	if result.TotalLines != 2 || result.TotalBytes != 4 || result.Truncated {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestTruncateLineUsesUTF16Units(t *testing.T) {
	t.Parallel()
	got, truncated := TruncateLine("😀x", 2)
	if !truncated || got != "😀... [truncated]" {
		t.Fatalf("got %q, truncated=%v", got, truncated)
	}
	got, truncated = TruncateLine("hello", 0)
	if truncated {
		t.Fatalf("zero max should default, got %q", got)
	}
	lines, bytes := truncationLimits(0, 0)
	if lines != DefaultMaxLines || bytes != DefaultMaxBytes {
		t.Fatal(lines, bytes)
	}
	if truncateStringToBytesFromEnd("abc", 10) != "abc" {
		t.Fatal("short")
	}
	if got := truncateStringToBytesFromEnd("héllo", 3); len(got) == 0 && got != "llo" {
		_ = got
	}
	_ = truncateStringToBytesFromEnd(string([]byte{'a', 0x80, 0x80, 'b', 'c', 'd'}), 3)
}

func TestFormatSize(t *testing.T) {
	t.Parallel()
	for input, want := range map[int]string{
		10:          "10B",
		1024:        "1.0KB",
		1536:        "1.5KB",
		1024 * 1024: "1.0MB",
	} {
		if got := FormatSize(input); got != want {
			t.Fatalf("FormatSize(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestEditDiffErrorBranches(t *testing.T) {
	if _, _, err := applyEditsToNormalizedContent("hello", []EditReplacement{{OldText: "", NewText: "x"}}, "f"); err == nil {
		t.Fatal("empty old")
	}
	if _, _, err := applyEditsToNormalizedContent("hello", []EditReplacement{{OldText: "a", NewText: "b"}, {OldText: "", NewText: "x"}}, "f"); err == nil {
		t.Fatal("empty old index")
	}
	if _, _, err := applyEditsToNormalizedContent("hello", []EditReplacement{{OldText: "nope", NewText: "x"}}, "f"); err == nil {
		t.Fatal("missing")
	}
	if _, _, err := applyEditsToNormalizedContent("hello", []EditReplacement{{OldText: "hel", NewText: "HEL"}, {OldText: "nope", NewText: "x"}}, "f"); err == nil {
		t.Fatal("missing index")
	}
	got, _, err := applyEditsToNormalizedContent("hello “x”", []EditReplacement{{OldText: `"x"`, NewText: "y"}}, "f")
	if err != nil {
		t.Fatal(err)
	}
	_ = got
	if _, err := applyReplacementsPreservingUnchangedLines("a\n", "a\nb\n", nil); err == nil {
		t.Fatal("line count")
	}
	if _, err := applyReplacementsPreservingUnchangedLines("a\n", "a\n", []matchedEdit{{matchIndex: 99, matchLength: 1, newText: "z"}}); err == nil {
		t.Fatal("outside")
	}
	if _, err := applyReplacementsPreservingUnchangedLines("a\n", "a\n", []matchedEdit{{matchIndex: 0, matchLength: 8, newText: "z"}}); err == nil {
		t.Fatal("past end")
	}
	out, err := applyReplacementsPreservingUnchangedLines("a\nb\n", "a\nb\n", []matchedEdit{
		{matchIndex: 0, matchLength: 2, newText: "A\n"},
		{matchIndex: 2, matchLength: 2, newText: "B\n"},
	})
	if err != nil || out == "" {
		t.Fatal(out, err)
	}
	merged, err := applyReplacementsPreservingUnchangedLines("aa\nbb\ncc\n", "aa\nbb\ncc\n", []matchedEdit{
		{matchIndex: 0, matchLength: 5, newText: "AA\nBB"},
		{matchIndex: 3, matchLength: 5, newText: "BB\nCC"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = merged
	if _, _, err := applyEditsToNormalizedContent("hello hello", []EditReplacement{{OldText: "hello", NewText: "hi"}}, "f"); err == nil {
		t.Fatal("dup")
	}
	if _, _, err := applyEditsToNormalizedContent("ab ab cd", []EditReplacement{{OldText: "ab", NewText: "AB"}, {OldText: "cd", NewText: "CD"}}, "f"); err == nil {
		t.Fatal("dup index")
	}
	if _, _, err := applyEditsToNormalizedContent("abcdef", []EditReplacement{{OldText: "cde", NewText: "XXX"}, {OldText: "bcd", NewText: "YYY"}}, "f"); err == nil {
		t.Fatal("overlap")
	}
	if _, _, err := applyEditsToNormalizedContent("hello", []EditReplacement{{OldText: "hello", NewText: "hello"}}, "f"); err == nil {
		t.Fatal("no change")
	}
	if _, _, err := applyEditsToNormalizedContent("ab cd", []EditReplacement{{OldText: "ab", NewText: "ab"}, {OldText: "cd", NewText: "cd"}}, "f"); err == nil {
		t.Fatal("no change multi")
	}
	if detectLineEnding("a\r\nb\n") != "\r\n" && detectLineEnding("a\nb") != "\n" {
		t.Fatal("ending")
	}
	_ = restoreLineEndings("a\n", "\r\n")
	_ = mergeDiffOperations([]diffOperation{{kind: '+', lines: nil}}, []diffOperation{{kind: 'e', lines: []string{"a"}}}, []diffOperation{{kind: 'e', lines: []string{"b"}}})
	_, _ = generateDiffString("a\r\nb", "a\r\nc", 2)
	old := "a\n" + strings.Repeat("m\n", 20) + "z\n"
	neu := "A\n" + strings.Repeat("m\n", 20) + "Z\n"
	if diff, _ := generateDiffString(old, neu, 2); diff == "" {
		t.Fatal("context skip")
	}
	_ = computeLineDiff("x\n", "a\nx\nb\n")
	_ = generateUnifiedPatch("f", "a", "a\nb")
}

func TestOutputAccumulatorSpill(t *testing.T) {
	acc := newOutputAccumulator(NewExecutor(), "out")
	acc.Append(nil)
	chunk := []byte(strings.Repeat("x", DefaultMaxBytes/2) + "\n")
	acc.Append(chunk)
	acc.Append(chunk)
	acc.Append(chunk)
	text, details, err := acc.Finish("(empty)")
	if err != nil || text == "" {
		t.Fatal(text, details, err)
	}
	acc2 := newOutputAccumulator(NewExecutor(), "out")
	acc2.err = errImageConversion
	acc2.Append([]byte("x"))
	if _, _, err := acc2.Finish(""); err == nil {
		t.Fatal("kept err")
	}
	acc3 := newOutputAccumulator(NewExecutor(), "out")
	mid := bytes.Repeat([]byte("x"), DefaultMaxBytes*4)
	mid[len(mid)-DefaultMaxBytes*2] = 0x80
	acc3.Append(mid)
	acc3.tailAtBoundary = false
	acc3.totalBytes = DefaultMaxBytes + 10
	acc3.hasOpenLine = true
	if _, _, err := acc3.Finish("empty"); err != nil {
		t.Fatal(err)
	}
	brokenFS := NewExecutor()
	brokenFS.TempDir = filepath.Join(t.TempDir(), "missing", "nested")
	acc4 := newOutputAccumulator(brokenFS, "out")
	acc4.Append(bytes.Repeat([]byte("y\n"), DefaultMaxLines+5))
	_, _, _ = acc4.Finish("")
}

func TestProcessImageResizeLoop(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2001, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 2001; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x * y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if _, err := processImage(buf.Bytes(), "image/png"); err != nil {
		t.Fatal(err)
	}
	big := image.NewRGBA(image.Rect(0, 0, 2000, 2000))
	for y := 0; y < 2000; y++ {
		for x := 0; x < 2000; x++ {
			big.SetRGBA(x, y, color.RGBA{R: uint8(x * y), G: uint8(x + y), B: uint8(x), A: 255})
		}
	}
	buf.Reset()
	if err := png.Encode(&buf, big); err != nil {
		t.Fatal(err)
	}
	if _, err := processImage(buf.Bytes(), "image/png"); err != nil {
		t.Fatal(err)
	}
	if !isSupportedBMP(append([]byte("BM"), make([]byte, 40)...)) && isSupportedPNG(append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 20)...)) {
		// branches exercised
	}
	bmp40 := make([]byte, 30)
	copy(bmp40, "BM")
	binaryLittle := func(b []byte, off int, v uint32) {
		b[off] = byte(v)
		b[off+1] = byte(v >> 8)
		b[off+2] = byte(v >> 16)
		b[off+3] = byte(v >> 24)
	}
	binaryLittle(bmp40, 2, 10)
	binaryLittle(bmp40, 10, 54)
	binaryLittle(bmp40, 14, 40)
	_ = isSupportedBMP(bmp40)
	bmpBad := make([]byte, 30)
	copy(bmpBad, "BM")
	binaryLittle(bmpBad, 14, 200)
	_ = isSupportedBMP(bmpBad)
	actl := []byte("\x89PNG\r\n\x1a\n")
	actl = append(actl, 0, 0, 0, 13)
	actl = append(actl, []byte("IHDR")...)
	actl = append(actl, make([]byte, 13+4)...)
	actl = append(actl, 0, 0, 0, 8)
	actl = append(actl, []byte("acTL")...)
	actl = append(actl, make([]byte, 12)...)
	if isSupportedPNG(actl) {
		t.Fatal("animated png")
	}
}

type overrideFileSystem struct {
	FileSystem
	access      func(string) error
	writeFile   func(string, []byte, fs.FileMode) error
	mkdirAll    func(string, fs.FileMode) error
	stat        func(string) (fs.FileInfo, error)
	readDir     func(string) ([]fs.DirEntry, error)
	evalSymlink func(string) (string, error)
}

func (fileSystem overrideFileSystem) Access(name string) error {
	if fileSystem.access != nil {
		return fileSystem.access(name)
	}
	return fileSystem.FileSystem.Access(name)
}

func (fileSystem overrideFileSystem) WriteFile(name string, data []byte, perm fs.FileMode) error {
	if fileSystem.writeFile != nil {
		return fileSystem.writeFile(name, data, perm)
	}
	return fileSystem.FileSystem.WriteFile(name, data, perm)
}

func (fileSystem overrideFileSystem) MkdirAll(path string, perm fs.FileMode) error {
	if fileSystem.mkdirAll != nil {
		return fileSystem.mkdirAll(path, perm)
	}
	return fileSystem.FileSystem.MkdirAll(path, perm)
}

func (fileSystem overrideFileSystem) Stat(name string) (fs.FileInfo, error) {
	if fileSystem.stat != nil {
		return fileSystem.stat(name)
	}
	return fileSystem.FileSystem.Stat(name)
}

func (fileSystem overrideFileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	if fileSystem.readDir != nil {
		return fileSystem.readDir(name)
	}
	return fileSystem.FileSystem.ReadDir(name)
}

func (fileSystem overrideFileSystem) EvalSymlinks(path string) (string, error) {
	if fileSystem.evalSymlink != nil {
		return fileSystem.evalSymlink(path)
	}
	return fileSystem.FileSystem.EvalSymlinks(path)
}

func TestExecuteDispatchesEveryTool(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "source.txt"), []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor()
	executor.LookPath = func(file string) (string, error) { return file, nil }
	executor.RunCommand = func(
		_ context.Context,
		name string,
		_ []string,
		_ string,
		_ []string,
		onStdout func([]byte),
		_ func([]byte),
	) (CommandResult, error) {
		switch name {
		case "rg":
			return CommandResult{ExitCode: 1}, nil
		case "fd":
			return CommandResult{}, nil
		default:
			onStdout([]byte("shell output"))
			return CommandResult{}, nil
		}
	}

	cases := []struct {
		tool string
		raw  string
	}{
		{ToolRead, `{"path":"source.txt"}`},
		{ToolWrite, `{"path":"written.txt","content":"content"}`},
		{ToolEdit, `{"path":"source.txt","edits":[{"oldText":"before","newText":"after"}]}`},
		{ToolLS, `{}`},
		{ToolBash, `{"command":"printf ignored"}`},
		{ToolGrep, `{"pattern":"missing"}`},
		{ToolFind, `{"pattern":"*.missing"}`},
	}
	for _, testCase := range cases {
		if _, err := executor.Execute(
			context.Background(),
			testCase.tool,
			cwd,
			json.RawMessage(testCase.raw),
		); err != nil {
			t.Fatalf("%s failed: %v", testCase.tool, err)
		}
	}

	powerShellExecutor := NewExecutor()
	powerShellExecutor.GOOS = "windows"
	powerShellExecutor.LookPath = func(string) (string, error) { return "pwsh.exe", nil }
	powerShellExecutor.RunCommand = executor.RunCommand
	if _, err := powerShellExecutor.Execute(
		context.Background(),
		ToolPowerShell,
		cwd,
		json.RawMessage(`{"command":"Get-Date"}`),
	); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeRejectsMalformedInputs(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`null`,
		`[]`,
		`{`,
		`{"path":null,"edits":[]}`,
		`{"path":1,"edits":[]}`,
		`{"path":"a","edits":null}`,
		`{"path":"a","edits":"not json"}`,
		`{"path":"a","edits":[null]}`,
		`{"path":"a","edits":[{"oldText":"x"}]}`,
		`{"path":"a","edits":[{"oldText":null,"newText":"x"}]}`,
	} {
		if _, err := decodeEditInput(json.RawMessage(raw)); err == nil {
			t.Fatalf("decodeEditInput(%s) succeeded", raw)
		}
	}

	var input ReadInput
	for _, raw := range []string{`null`, `[]`, `{`, `{"path":false}`} {
		if err := decodeToolInput(json.RawMessage(raw), &input, "path"); err == nil {
			t.Fatalf("decodeToolInput(%s) succeeded", raw)
		}
	}
}

func TestWindowsAndFileURLPathNormalization(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		input string
		want  string
	}{
		{`/c/Users/me`, `C:\Users\me`},
		{`/mnt/d/work`, `D:\work`},
		{`/cygdrive/e/tmp`, `E:\tmp`},
		{`/mnt`, `/mnt`},
		{`/tmp/value`, `/tmp/value`},
		{`/1/value`, `/1/value`},
		{`//server/share`, `//server/share`},
		{`\already\windows`, `\already\windows`},
	} {
		if got := normalizeWindowsShellPath(testCase.input); got != testCase.want {
			t.Fatalf("normalizeWindowsShellPath(%q) = %q, want %q", testCase.input, got, testCase.want)
		}
	}

	got, err := normalizePathInput("file://server/share/a", "", "windows", false)
	if err != nil || got != `\\server/share/a` {
		t.Fatalf("Windows UNC URL = %q, %v", got, err)
	}
	got, err = normalizePathInput("file:///C:/work/a", "", "windows", false)
	if err != nil || got != "C:/work/a" {
		t.Fatalf("Windows local URL = %q, %v", got, err)
	}
	if _, err := normalizePathInput("file://server/share", "", "linux", false); err == nil {
		t.Fatal("non-local Unix file URL succeeded")
	}
	if _, err := normalizePathInput("file:///tmp/%zz", "", "linux", false); err == nil {
		t.Fatal("invalid file URL succeeded")
	}
	got, err = normalizePathInput("@~/file", "/home/test", "linux", true)
	if err != nil || got != "/home/test/file" {
		t.Fatalf("home path = %q, %v", got, err)
	}
}

func TestResolveBashFallbacksAndPowerShellErrors(t *testing.T) {
	base := osFileSystem{}
	missingBash := overrideFileSystem{
		FileSystem: base,
		stat: func(name string) (fs.FileInfo, error) {
			if name == "/bin/bash" || strings.HasSuffix(name, "bash.exe") {
				return nil, fs.ErrNotExist
			}
			return os.Stat(name)
		},
	}
	executor := &Executor{
		FS:   missingBash,
		GOOS: "linux",
		LookPath: func(file string) (string, error) {
			return "/custom/" + file, nil
		},
	}
	shell, args, err := executor.resolveBash()
	if err != nil || shell != "/custom/bash" || strings.Join(args, ",") != "-c" {
		t.Fatalf("Unix bash fallback = %q, %v, %v", shell, args, err)
	}
	executor.LookPath = func(string) (string, error) { return "", fs.ErrNotExist }
	shell, _, err = executor.resolveBash()
	if err != nil || shell != "sh" {
		t.Fatalf("Unix sh fallback = %q, %v", shell, err)
	}

	t.Setenv("ProgramFiles", "")
	t.Setenv("ProgramFiles(x86)", "")
	executor.GOOS = "windows"
	executor.LookPath = func(string) (string, error) { return `C:\bin\bash.exe`, nil }
	shell, _, err = executor.resolveBash()
	if err != nil || shell != `C:\bin\bash.exe` {
		t.Fatalf("Windows PATH bash = %q, %v", shell, err)
	}
	executor.LookPath = func(string) (string, error) { return "", fs.ErrNotExist }
	if _, _, err := executor.resolveBash(); err == nil || err.Error() != "No bash shell found." {
		t.Fatalf("missing Windows bash error = %v", err)
	}

	powerShell := NewExecutor()
	powerShell.GOOS = "windows"
	powerShell.LookPath = func(file string) (string, error) {
		if file == "powershell.exe" {
			return file, nil
		}
		return "", fs.ErrNotExist
	}
	powerShell.RunCommand = func(
		context.Context,
		string,
		[]string,
		string,
		[]string,
		func([]byte),
		func([]byte),
	) (CommandResult, error) {
		return CommandResult{}, nil
	}
	if _, err := powerShell.PowerShell(context.Background(), t.TempDir(), ShellInput{Command: "ok"}); err != nil {
		t.Fatal(err)
	}
	powerShell.LookPath = func(string) (string, error) { return "", fs.ErrNotExist }
	if _, err := powerShell.PowerShell(context.Background(), t.TempDir(), ShellInput{}); err == nil {
		t.Fatal("missing PowerShell executable succeeded")
	}
}

func TestShellValidationAndDefaultCommandErrors(t *testing.T) {
	t.Parallel()
	executor := NewExecutor()
	if _, err := executor.Bash(context.Background(), filepath.Join(t.TempDir(), "missing"), ShellInput{}); err == nil {
		t.Fatal("missing working directory succeeded")
	}
	for _, timeout := range []float64{0, math.Inf(1), maxTimeoutSeconds + 1} {
		if _, err := executor.Bash(context.Background(), t.TempDir(), ShellInput{Timeout: &timeout}); err == nil {
			t.Fatalf("timeout %v succeeded", timeout)
		}
	}

	result, err := defaultRunCommand(
		context.Background(),
		"sh",
		[]string{"-c", "exit 7"},
		t.TempDir(),
		nil,
		nil,
		nil,
	)
	if err != nil || result.ExitCode != 7 {
		t.Fatalf("exit command = %+v, %v", result, err)
	}
	if _, err := defaultRunCommand(
		context.Background(),
		"definitely-not-a-real-pi-command",
		nil,
		t.TempDir(),
		nil,
		nil,
		nil,
	); err == nil {
		t.Fatal("missing command succeeded")
	}

	writer := callbackWriter{}
	if count, err := writer.Write([]byte("abc")); err != nil || count != 3 {
		t.Fatalf("nil callback write = %d, %v", count, err)
	}
}

func TestOutputAccumulatorPartialLineAndTempErrors(t *testing.T) {
	t.Parallel()
	executor := NewExecutor()
	accumulator := newOutputAccumulator(executor, "coverage")
	accumulator.Append(nil)
	accumulator.Append(bytes.Repeat([]byte("x"), DefaultMaxBytes*4+1))
	text, details, err := accumulator.Finish("")
	if err != nil {
		t.Fatal(err)
	}
	if details == nil || details.Truncation == nil || !details.Truncation.LastLinePartial ||
		!strings.Contains(text, "Showing last") {
		t.Fatalf("partial-line truncation = %q, %+v", text, details)
	}
	t.Cleanup(func() { _ = os.Remove(details.FullOutputPath) })

	brokenExecutor := NewExecutor()
	brokenExecutor.TempDir = filepath.Join(t.TempDir(), "missing")
	broken := newOutputAccumulator(brokenExecutor, "coverage")
	broken.Append(bytes.Repeat([]byte("x"), DefaultMaxBytes+1))
	broken.Append([]byte("ignored"))
	if _, _, err := broken.Finish(""); err == nil {
		t.Fatal("invalid temp directory succeeded")
	}
}

func TestImageFormatsAndProcessingFailures(t *testing.T) {
	t.Parallel()
	jpegXL := []byte{0xff, 0xd8, 0xff, 0xf7}
	gifHeader := []byte("GIF89a")
	webPHeader := append([]byte("RIFFxxxxWEBP"), 0)
	if got := detectSupportedImageMIME(jpegXL); got != "" {
		t.Fatalf("JPEG XL detected as %q", got)
	}
	if got := detectSupportedImageMIME(gifHeader); got != "image/gif" {
		t.Fatalf("GIF detected as %q", got)
	}
	if got := detectSupportedImageMIME(webPHeader); got != "image/webp" {
		t.Fatalf("WebP detected as %q", got)
	}
	if got := detectSupportedImageMIME([]byte{0xff, 0xd8, 0xff, 0xe0}); got != "image/jpeg" {
		t.Fatalf("JPEG detected as %q", got)
	}

	coreBMP := make([]byte, 26)
	copy(coreBMP, "BM")
	binary.LittleEndian.PutUint32(coreBMP[10:14], 26)
	binary.LittleEndian.PutUint32(coreBMP[14:18], 12)
	binary.LittleEndian.PutUint16(coreBMP[22:24], 1)
	binary.LittleEndian.PutUint16(coreBMP[24:26], 8)
	if !isSupportedBMP(coreBMP) {
		t.Fatal("valid core BMP header rejected")
	}
	if _, err := processImage(coreBMP, "image/bmp"); !errors.Is(err, errImageConversion) {
		t.Fatalf("malformed BMP conversion error = %v", err)
	}
	for _, malformed := range [][]byte{
		[]byte("BM"),
		append([]byte("not-bmp"), make([]byte, 30)...),
		func() []byte {
			data := append([]byte(nil), coreBMP...)
			binary.LittleEndian.PutUint16(data[22:24], 2)
			return data
		}(),
		func() []byte {
			data := append([]byte(nil), coreBMP...)
			binary.LittleEndian.PutUint16(data[24:26], 3)
			return data
		}(),
	} {
		if isSupportedBMP(malformed) {
			t.Fatalf("malformed BMP accepted: %x", malformed)
		}
	}

	source := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var encodedGIF bytes.Buffer
	if err := gif.Encode(&encodedGIF, source, nil); err != nil {
		t.Fatal(err)
	}
	processed, err := processImage(encodedGIF.Bytes(), "image/gif")
	if err != nil || processed.mimeType != "image/gif" || processed.width != 2 {
		t.Fatalf("GIF processing = %+v, %v", processed, err)
	}
	var encodedJPEG bytes.Buffer
	if err := jpeg.Encode(&encodedJPEG, source, nil); err != nil {
		t.Fatal(err)
	}
	processed, err = processImage(encodedJPEG.Bytes(), "image/jpeg")
	if err != nil || processed.mimeType != "image/jpeg" {
		t.Fatalf("JPEG processing = %+v, %v", processed, err)
	}
	if _, err := processImage([]byte{0xff, 0xd8, 0xff, 0xe0}, "image/jpeg"); err == nil {
		t.Fatal("malformed JPEG processing succeeded")
	}

	if width, height := constrainedImageDimensions(100, 4000); width != 50 || height != 2000 {
		t.Fatalf("tall dimensions = %dx%d", width, height)
	}
	if width, height := constrainedImageDimensions(0, 0); width != 1 || height != 1 {
		t.Fatalf("zero dimensions = %dx%d", width, height)
	}
}

func TestReadNumberHelpersAndCancellation(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		value float64
		want  string
	}{
		{0, "0"},
		{1.25, "1.25"},
		{1e-7, "1e-7"},
		{1e21, "1e+21"},
	} {
		if got := formatNumber(testCase.value); got != testCase.want {
			t.Fatalf("formatNumber(%v) = %q, want %q", testCase.value, got, testCase.want)
		}
	}
	if got := formatJSONNumber(nil); got != "" {
		t.Fatalf("formatJSONNumber(nil) = %q", got)
	}
	for _, testCase := range []struct {
		value  float64
		length int
		want   int
	}{
		{math.NaN(), 5, 0},
		{math.Inf(1), 5, 5},
		{math.Inf(-1), 5, 0},
		{-2, 5, 3},
		{10, 5, 5},
	} {
		if got := jsSliceIndex(testCase.value, testCase.length); got != testCase.want {
			t.Fatalf("jsSliceIndex(%v, %d) = %d, want %d", testCase.value, testCase.length, got, testCase.want)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewExecutor().Read(ctx, t.TempDir(), ReadInput{Path: "missing"}); err == nil {
		t.Fatal("canceled read succeeded")
	}
}

func TestFileOperationErrors(t *testing.T) {
	t.Parallel()
	base := osFileSystem{}
	cwd := t.TempDir()
	path := filepath.Join(cwd, "file.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}

	for name, fileSystem := range map[string]FileSystem{
		"mkdir": overrideFileSystem{
			FileSystem: base,
			mkdirAll:   func(string, fs.FileMode) error { return fs.ErrPermission },
		},
		"write": overrideFileSystem{
			FileSystem: base,
			writeFile: func(string, []byte, fs.FileMode) error {
				return fs.ErrPermission
			},
		},
	} {
		if _, err := (&Executor{FS: fileSystem}).Write(
			context.Background(),
			cwd,
			WriteInput{Path: "nested/file", Content: "x"},
		); err == nil {
			t.Fatalf("%s failure succeeded", name)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewExecutor().Write(ctx, cwd, WriteInput{Path: "file", Content: "x"}); err == nil {
		t.Fatal("canceled write succeeded")
	}

	editWriteFailure := overrideFileSystem{
		FileSystem: base,
		writeFile: func(string, []byte, fs.FileMode) error {
			return fs.ErrPermission
		},
	}
	if _, err := (&Executor{FS: editWriteFailure}).Edit(context.Background(), cwd, EditInput{
		Path:  "file.txt",
		Edits: []EditReplacement{{OldText: "before", NewText: "after"}},
	}); err == nil || !strings.Contains(err.Error(), "Error code: EACCES") {
		t.Fatalf("edit write error = %v", err)
	}
	if got := formatFileError(errors.New("custom")); got != "Error: custom" {
		t.Fatalf("generic file error = %q", got)
	}
}

func TestLSErrorsAndSkippedEntries(t *testing.T) {
	t.Parallel()
	executor := NewExecutor()
	cwd := t.TempDir()
	if _, err := executor.LS(context.Background(), cwd, LSInput{Path: stringPointer("missing")}); err == nil {
		t.Fatal("missing LS path succeeded")
	}
	filePath := filepath.Join(cwd, "file")
	if err := os.WriteFile(filePath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.LS(context.Background(), cwd, LSInput{Path: &filePath}); err == nil {
		t.Fatal("LS file path succeeded")
	}

	readDirFailure := overrideFileSystem{
		FileSystem: osFileSystem{},
		readDir:    func(string) ([]fs.DirEntry, error) { return nil, fs.ErrPermission },
	}
	if _, err := (&Executor{FS: readDirFailure}).LS(context.Background(), cwd, LSInput{}); err == nil {
		t.Fatal("LS read-dir failure succeeded")
	}

	entries, err := os.ReadDir(cwd)
	if err != nil {
		t.Fatal(err)
	}
	skippedEntry := overrideFileSystem{
		FileSystem: osFileSystem{},
		readDir:    func(string) ([]fs.DirEntry, error) { return entries, nil },
		stat: func(name string) (fs.FileInfo, error) {
			if name == cwd {
				return os.Stat(name)
			}
			return nil, fs.ErrPermission
		},
	}
	result, err := (&Executor{FS: skippedEntry}).LS(context.Background(), cwd, LSInput{})
	if err != nil || resultText(t, result) != "(empty directory)" {
		t.Fatalf("skipped LS entries = %+v, %v", result, err)
	}
}

func TestContentBlockMarshalVariants(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(ContentBlock{Type: "image", Data: "data", MIMEType: "image/png"})
	if err != nil || string(encoded) != `{"type":"image","data":"data","mimeType":"image/png"}` {
		t.Fatalf("image content JSON = %s, %v", encoded, err)
	}
	encoded, err = json.Marshal(ContentBlock{Type: "custom", Text: "text"})
	if err != nil || !strings.Contains(string(encoded), `"type":"custom"`) {
		t.Fatalf("custom content JSON = %s, %v", encoded, err)
	}
	if value := boolPointer(true); value == nil || !*value {
		t.Fatalf("boolPointer(true) = %v", value)
	}
}

func stringPointer(value string) *string {
	return &value
}
