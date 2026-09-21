package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codedock/pkg/agent/tool"
)

func TestMentionedPlanNames(t *testing.T) {
	t.Parallel()
	got := MentionedPlanNames("继续 .cursor/task.md，也看一下 notes.md 和 README.md")
	if len(got) != 2 || got[0] != "task.md" || got[1] != "notes.md" {
		t.Fatalf("got=%v", got)
	}
	if MentionedPlanNames("写一个计时器") != nil {
		t.Fatal("plain request should not mention a plan")
	}
}

func TestWantsPlanCatalog(t *testing.T) {
	t.Parallel()
	if !WantsPlanCatalog("列出所有计划") || !WantsPlanCatalog("list plans") {
		t.Fatal("catalog ask")
	}
	if WantsPlanCatalog("写一个计划") || WantsPlanCatalog("更新当前计划") {
		t.Fatal("write request is not a catalog ask")
	}
}

func TestResolvePlanScopeBindsMentionAndHistory(t *testing.T) {
	t.Parallel()
	scope := ResolvePlanScope("", []Message{{
		Role:    RoleUser,
		Content: EncodeText("按 timer.md 做"),
	}})
	if scope.ActivePlan != "timer.md" || !scope.Allows("timer.md") || scope.AllowListAll {
		t.Fatalf("mention bind=%+v", scope)
	}
	wrote, _ := json.Marshal(map[string]string{"name": "desk.md"})
	scope = ResolvePlanScope("", []Message{
		{Role: RoleUser, Content: EncodeText("先写一份")},
		{Role: RoleTool, Content: EncodeToolResult("c1", wrote)},
	})
	if scope.ActivePlan != "desk.md" {
		t.Fatalf("history bind=%+v", scope)
	}
}

func TestLoadPlanItemsIgnoresSiblings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	other := "---\n{\"title\":\"other\",\"items\":[{\"id\":\"x\",\"description\":\"别的任务\",\"verify_cmd\":\"manual\"}]}\n---\n# other\n"
	mine := "---\n{\"title\":\"mine\",\"items\":[{\"id\":\"a\",\"description\":\"当前任务\",\"verify_cmd\":\"manual\"}]}\n---\n# mine\n"
	if err := os.WriteFile(filepath.Join(dir, ".cursor", "other.md"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".cursor", "mine.md"), []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadPlanItems(dir, ""); len(got) != 0 {
		t.Fatalf("unbound=%+v", got)
	}
	got := LoadPlanItems(dir, "mine.md")
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("bound=%+v", got)
	}
	all := LoadWorkspacePlanItems(dir)
	if len(all) != 2 {
		t.Fatalf("workspace all=%+v", all)
	}
}

func TestBindActivePlanFromResults(t *testing.T) {
	t.Parallel()
	body, _ := json.Marshal(map[string]string{"name": "next.md"})
	got := BindActivePlan("old.md", []tool.Result{{
		Name:    "plan_write",
		Success: true,
		Output:  body,
	}})
	if got != "next.md" {
		t.Fatalf("got=%q", got)
	}
	if BindActivePlan("keep.md", []tool.Result{{Name: "plan_list", Success: true}}) != "keep.md" {
		t.Fatal("list should not rebind")
	}
}

func TestInferActivePlanIgnoresMemoryName(t *testing.T) {
	t.Parallel()
	body, _ := json.Marshal(map[string]string{"name": "index"})
	if InferActivePlan([]Message{{Role: RoleTool, Content: EncodeToolResult("c", body)}}) != "" {
		t.Fatal("memory name must not bind a plan")
	}
}

func TestPlanScopeNote(t *testing.T) {
	t.Parallel()
	if !strings.Contains(PlanScopeNote(""), "尚未绑定") {
		t.Fatal("unbound note")
	}
	if !strings.Contains(PlanScopeNote("task.md"), "task.md") {
		t.Fatal("bound note")
	}
}
