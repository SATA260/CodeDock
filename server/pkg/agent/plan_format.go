package agent

import "strings"

// PlanMarkdownExample 是给人看的最小计划正文。
const PlanMarkdownExample = "# 任务名\n\n## 验收\n\n- [ ] V1 可观察的验收标准 — `manual`\n"

// PlanFrontmatterExample 兼容旧名字，内容已是 Markdown 例子。
const PlanFrontmatterExample = PlanMarkdownExample

// PlanItemFieldHint 说明验收项怎么写进 Markdown，以及工具入参字段名。
const PlanItemFieldHint = "每条验收项写成「- [ ] V1 说明 — 验证命令」；工具入参 items 的字段名仍是 id、description、verify_cmd"

// PlanWriteFormatHint 给模式提示和工具描述复用的格式说明。
const PlanWriteFormatHint = "计划是给人看的 Markdown。plan_write 优先填 items（id、description、verify_cmd），工具会写成标题和验收勾选列表；也可以直接在正文写同样的列表。不要写 JSON。例子：\n" + PlanMarkdownExample

// ComposePlanWrite 把独立验收项与 markdown 正文合成完整计划；没有额外字段时原样返回 content。
func ComposePlanWrite(content, title string, items []PlanItem) string {
	title = strings.TrimSpace(title)
	contract, body, err := ParsePlanContract(content)
	if err != nil {
		body = content
		contract = PlanContract{}
	}
	if title != "" {
		contract.Title = title
	}
	if len(items) > 0 {
		contract.Items = append([]PlanItem(nil), items...)
	}
	if err != nil && title == "" && len(items) == 0 {
		return content
	}
	if err == nil && title == "" && len(items) == 0 {
		return content
	}
	return RenderPlan(contract, body)
}
