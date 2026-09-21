package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestImportDirection 倒逼底座包不能引用上层内部包。
func TestImportDirection(t *testing.T) {
	root := repoRoot(t)
	rules := []struct {
		dir  string
		deny []string
		hint string
		doc  string
	}{
		{"server/pkg/agent", []string{"codedock/internal"}, "底座核心包不可依赖上层内部包，请将事件通过接口回调注入。", "docs/architecture.md 第 3 节"},
		{"server/pkg/git", []string{"codedock/internal", "codedock/pkg/agent"}, "Git CLI 不进 pkg/agent，也不写产品流程。", "docs/architecture.md"},
		{"server/pkg/plugin", []string{"codedock/internal"}, "插件 SDK 不依赖 handler 或 internal。", "docs/architecture.md"},
		{"server/internal/agent/memory", []string{"codedock/internal/agent"}, "memory 不 import 父包 internal/agent。", "docs/architecture.md"},
		{"server/internal/agent/tools", []string{"codedock/internal/agent"}, "tools 不 import 父包 internal/agent。", "docs/architecture.md"},
		{"server/internal/handler", []string{"codedock/internal/agent/tools"}, "Handler 不 import internal/agent/tools。", "docs/architecture.md"},
	}
	for _, rule := range rules {
		imports := packageImports(t, filepath.Join(root, rule.dir))
		for _, deny := range rule.deny {
			for _, item := range imports {
				if item.path == deny || (deny != "codedock/internal/agent" && strings.HasPrefix(item.path, deny)) {
					t.Errorf("❌ 出了什么问题：`%s` 引用了 `%s`。\n💡 应该怎么改：%s\n📖 参考哪篇规范：请查阅 %s。", item.pkg, item.path, rule.hint, rule.doc)
				}
			}
		}
	}
}

// TestArchitectureMentionsCoreNames 倒逼核心工具、状态和事件写入架构文档。
func TestArchitectureMentionsCoreNames(t *testing.T) {
	root := repoRoot(t)
	doc, err := os.ReadFile(filepath.Join(root, "docs", "architecture.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(doc)
	required := []string{
		"plan_pass",
		"explore",
		"verifying",
		"evaluating",
		"verify.started",
		"evaluate.result",
		"HadSideEffects",
		"EvaluatorModel",
		"SubagentModel",
		"plan_write",
		"edit",
		"write",
	}
	for _, name := range required {
		if !strings.Contains(body, name) {
			t.Errorf("你新增了 %s，但 docs/architecture.md 中尚未提及，请去补上一句职责说明。", name)
		}
	}
}

type importRef struct {
	pkg  string
	path string
}

// packageImports 收集目录下 Go 文件的 import 路径。
func packageImports(t *testing.T, dir string) []importRef {
	t.Helper()
	var out []importRef
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == "vendor" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		pkg := strings.TrimPrefix(path, dir+string(os.PathSeparator))
		for _, spec := range file.Imports {
			out = append(out, importRef{pkg: filepath.ToSlash(pkg), path: strings.Trim(spec.Path.Value, `"`)})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// repoRoot 从测试目录向上找到仓根。
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "docs", "architecture.md")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("repo root not found")
	return ""
}
