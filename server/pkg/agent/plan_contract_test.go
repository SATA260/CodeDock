package agent

import (
	"strings"
	"testing"
)

func TestParsePlanAcceptsItemAliases(t *testing.T) {
	t.Parallel()
	raw := "---\n{\"title\":\"t\",\"items\":[{\"id\":\"V1\",\"desc\":\"读者可借书\",\"verify\":\"manual\"}]}\n---\n# body\n"
	got, err := ValidatePlan(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Items[0].ID != "V1" || got.Items[0].Description != "读者可借书" || got.Items[0].VerifyCmd != "manual" {
		t.Fatalf("alias item=%+v", got.Items[0])
	}

	yaml := "---\ntitle: t\nacceptance:\n  - V1: 读者可还书\n    verify: manual\n---\n# body\n"
	got, err = ValidatePlan(yaml, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Items[0].ID != "V1" || got.Items[0].Description != "读者可还书" {
		t.Fatalf("yaml shorthand=%+v", got.Items[0])
	}
}

func TestValidatePlanErrorsNameRequiredFields(t *testing.T) {
	t.Parallel()
	_, err := ValidatePlan("# only body\n", nil)
	if err == nil || !strings.Contains(err.Error(), "验收清单") {
		t.Fatalf("body-only err=%v", err)
	}
	_, err = ValidatePlan("---\n{\"title\":\"t\"}\n---\n# x\n", nil)
	if err == nil || !strings.Contains(err.Error(), "缺少验收清单") {
		t.Fatalf("empty items err=%v", err)
	}
	_, err = ValidatePlan("---\n{\"title\":\"t\",\"items\":[{\"id\":\"V1\",\"verify_cmd\":\"manual\"}]}\n---\n# x\n", nil)
	if err == nil || !strings.Contains(err.Error(), "V1 缺少说明") {
		t.Fatalf("missing desc err=%v", err)
	}
}

func TestComposePlanWriteFromItems(t *testing.T) {
	t.Parallel()
	got := ComposePlanWrite("# 图书\n", "图书管理", []PlanItem{{
		ID: "V1", Description: "可以登记图书", VerifyCmd: "manual",
	}})
	contract, err := ValidatePlan(got, nil)
	if err != nil {
		t.Fatal(err)
	}
	if contract.Title != "图书管理" || contract.Items[0].Description != "可以登记图书" {
		t.Fatalf("composed=%+v body=%q", contract, got)
	}
	if strings.Contains(got, "---") || strings.Contains(got, `"plan_name"`) {
		t.Fatalf("plan should be markdown, got %q", got)
	}
	if !strings.Contains(got, "# 图书管理") || !strings.Contains(got, "- [ ] V1 可以登记图书") {
		t.Fatalf("readable markdown=%q", got)
	}
	unchanged := "---\n{\"title\":\"t\",\"items\":[{\"id\":\"a\",\"description\":\"文档\",\"verify_cmd\":\"manual\"}]}\n---\n# keep\n"
	if ComposePlanWrite(unchanged, "", nil) != unchanged {
		t.Fatal("content without extras should stay unchanged")
	}
}

// TestParseMarkdownPlanReadsChecklist 正常 Markdown 勾选列表能抽出验收项。
func TestParseMarkdownPlanReadsChecklist(t *testing.T) {
	t.Parallel()
	raw := "# 图书管理\n\n## 验收\n\n- [ ] V1 可以登记图书 — `manual`\n- [x] V2 可以检索 — `npx vitest run tests/search.test.ts`\n  依据：测试通过\n\n## 步骤\n\n先写存储。\n"
	got, body, err := ParsePlanContract(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "图书管理" || len(got.Items) != 2 {
		t.Fatalf("got=%+v", got)
	}
	if got.Items[0].VerifyCmd != "manual" || got.Items[1].VerifyCmd != "npx vitest run tests/search.test.ts" {
		t.Fatalf("cmds=%+v", got.Items)
	}
	if !got.Items[1].Passes || got.Items[1].Evidence != "测试通过" {
		t.Fatalf("passed item=%+v", got.Items[1])
	}
	if !strings.Contains(body, "先写存储") || strings.Contains(body, "## 验收") {
		t.Fatalf("body=%q", body)
	}
}
