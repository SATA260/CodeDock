package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/tool"
)

type planListInput struct{}

type planReadInput struct {
	Name string `json:"name"`
}

// planWriteItemInput 是 plan_write 的一条验收项。
type planWriteItemInput struct {
	ID          string `json:"id" jsonschema:"验收项 id，如 V1"`
	Description string `json:"description" jsonschema:"可观察的验收标准，字段名必须是 description"`
	VerifyCmd   string `json:"verify_cmd" jsonschema:"验证命令；文档可用 manual，业务逻辑必须是具体测试命令"`
}

type planWriteInput struct {
	Name    string               `json:"name" jsonschema:"计划文件名，如 book-mgmt.md"`
	Content string               `json:"content" jsonschema:"给人看的 markdown 正文。可含标题、验收勾选列表和步骤；不要写 JSON"`
	Title   string               `json:"title,omitempty" jsonschema:"计划标题，写成一级标题"`
	Items   []planWriteItemInput `json:"items" jsonschema:"验收清单。每条必须有 id、description、verify_cmd；写入 Markdown 勾选列表"`
}

type planPassInput struct {
	Name     string `json:"name"`
	ItemID   string `json:"itemId"`
	Evidence string `json:"evidence"`
}

type planListOutput struct {
	Names []string `json:"names"`
	Note  string   `json:"note,omitempty"` // 未绑定时说明为什么不列出其他计划
}

type planItemOutput struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type planTool struct {
	name   string
	prompt string
	schema json.RawMessage
	ports  Ports
}

// Definition 描述计划工具。
func (t planTool) Definition() tool.Definition {
	return tool.Definition{
		Name:             t.name,
		Prompt:           t.prompt,
		ParametersSchema: t.schema,
		Permission:       tool.Permission{Effect: tool.EffectAllow},
		Version:          "1",
	}
}

// Inspect 校验计划名落在工作区 .cursor/ 下。
func (t planTool) Inspect(_ context.Context, input tool.Input) error {
	if t.name == "plan_list" {
		return nil
	}
	_, err := planNameFromArgs(t.name, input.Call.Arguments)
	return err
}

// Execute 按工具名列出、读取、写入或打勾计划。
func (t planTool) Execute(ctx context.Context, input tool.Input) (tool.Result, error) {
	if err := ctx.Err(); err != nil {
		return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, err
	}
	dir, err := planDir(workspaceOf(input, t.ports))
	if err != nil {
		return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
	}
	switch t.name {
	case "plan_list":
		entries, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
				continue
			}
			names = append(names, entry.Name())
		}
		return okToolResult(input.Call.ID, t.name, filterPlanList(input, names))
	case "plan_read":
		name, err := planNameFromArgs(t.name, input.Call.Arguments)
		if err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		if err := denyForeignPlan(input, name); err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		return okToolResult(input.Call.ID, t.name, planItemOutput{Name: name, Content: string(body)})
	case "plan_write":
		var args planWriteInput
		if err := json.Unmarshal(nonzeroJSON(input.Call.Arguments), &args); err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		name, err := sanitizePlanName(args.Name)
		if err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		path := filepath.Join(dir, name)
		if _, statErr := os.Stat(path); statErr == nil {
			if err := denyForeignPlan(input, name); err != nil {
				return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
			}
		}
		var existing *pkgagent.PlanContract
		if prev, err := os.ReadFile(path); err == nil {
			if parsed, _, parseErr := pkgagent.ParsePlanContract(string(prev)); parseErr == nil {
				existing = &parsed
			}
		}
		composed := pkgagent.ComposePlanWrite(args.Content, args.Title, planItemsFromWrite(args.Items))
		contract, err := pkgagent.ValidatePlan(composed, existing)
		if err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		if err := enforcePlanTestBinding(contract); err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		contract.PlanName = name
		_, body, _ := pkgagent.ParsePlanContract(composed)
		content := pkgagent.RenderPlan(contract, body)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		return okToolResult(input.Call.ID, t.name, planItemOutput{Name: name, Content: content})
	case "plan_pass":
		var args planPassInput
		if err := json.Unmarshal(nonzeroJSON(input.Call.Arguments), &args); err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		name, err := sanitizePlanName(args.Name)
		if err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		if err := denyForeignPlan(input, name); err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		path := filepath.Join(dir, name)
		prev, err := os.ReadFile(path)
		if err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		contract, body, err := pkgagent.ParsePlanContract(string(prev))
		if err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		contract, err = pkgagent.MarkPassed(contract, args.ItemID, args.Evidence)
		if err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		content := pkgagent.RenderPlan(contract, body)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		return okToolResult(input.Call.ID, t.name, planItemOutput{Name: name, Content: content})
	default:
		return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: "unknown plan tool"}, nil
	}
}

// planNameFromArgs 从入参取出并规范化计划文件名。
func planNameFromArgs(name string, raw json.RawMessage) (string, error) {
	var args planReadInput
	if err := json.Unmarshal(nonzeroJSON(raw), &args); err != nil {
		return "", err
	}
	if name == "plan_write" {
		var write planWriteInput
		if err := json.Unmarshal(nonzeroJSON(raw), &write); err != nil {
			return "", err
		}
		args.Name = write.Name
	}
	if name == "plan_pass" {
		var pass planPassInput
		if err := json.Unmarshal(nonzeroJSON(raw), &pass); err != nil {
			return "", err
		}
		args.Name = pass.Name
	}
	return sanitizePlanName(args.Name)
}

// sanitizePlanName 只允许 .cursor 下的短文件名。
func sanitizePlanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	base := filepath.Base(name)
	if base != filepath.Clean(name) || strings.Contains(name, "/") || strings.Contains(name, `\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid plan name")
	}
	if !strings.HasSuffix(strings.ToLower(base), ".md") {
		base += ".md"
	}
	return base, nil
}

// isWorkspacePlanFile 判断编码工具目标是否是工作区计划文件。
func isWorkspacePlanFile(ports Ports, name string, raw json.RawMessage, root string) bool {
	path, err := codingPath(name, raw)
	if err != nil || strings.TrimSpace(path) == "" || strings.TrimSpace(root) == "" {
		return false
	}
	exec := ports.executor()
	resolved, err := exec.resolveToCWD(path, root)
	if err != nil {
		return false
	}
	jailed, err := jailPath(root, resolved, exec.FS)
	if err != nil {
		return false
	}
	rel, err := pathRel(filepath.Clean(root), jailed)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	base, ok := strings.CutPrefix(rel, ".cursor/")
	if !ok || base == "" || strings.Contains(base, "/") {
		return false
	}
	return strings.HasSuffix(strings.ToLower(base), ".md")
}

// planDir 返回工作区 .cursor 目录。
func planDir(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("workspace root is required")
	}
	dir := filepath.Join(root, ".cursor")
	if _, err := jailPath(root, dir, nil); err != nil {
		return "", err
	}
	return dir, nil
}

// okToolResult 把结构体编成成功的工具结果。
func okToolResult(callID, name string, body any) (tool.Result, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return tool.Result{CallID: callID, Name: name, Success: false, Error: err.Error()}, nil
	}
	return tool.Result{CallID: callID, Name: name, Output: raw, Success: true}, nil
}

// planTools 构造计划四工具。
func planTools(ports Ports) []tool.Tool {
	return []tool.Tool{
		planTool{name: "plan_list", prompt: "列出本会话已绑定或用户点名的计划。用户未指定时不要用来扫目录里的其他计划。", schema: schemaOf[planListInput](), ports: ports},
		planTool{name: "plan_read", prompt: "读取本会话已绑定或用户点名的一篇计划。不要读其他计划。", schema: schemaOf[planReadInput](), ports: ports},
		planTool{name: "plan_write", prompt: "新建或覆盖本会话的一篇给人看的 markdown 计划。优先传 items（id、description、verify_cmd），写成标题和验收勾选列表；也可以在 content 里直接写「- [ ] V1 说明 — 命令」。不要写 JSON。只能增不能删。不要改未点名的其他计划。", schema: schemaOf[planWriteInput](), ports: ports},
		planTool{name: "plan_pass", prompt: "把本会话计划里的一条验收项标为通过，必须提交证据。", schema: schemaOf[planPassInput](), ports: ports},
	}
}

// planScopeOf 从工具入参还原本会话计划范围。
func planScopeOf(input tool.Input) pkgagent.PlanScope {
	return pkgagent.PlanScope{
		ActivePlan:   input.ActivePlan,
		Mentioned:    input.MentionedPlans,
		AllowListAll: input.AllowListAll,
	}
}

// denyForeignPlan 拒绝读写未绑定且用户未点名的计划。
func denyForeignPlan(input tool.Input, name string) error {
	if planScopeOf(input).Allows(name) {
		return nil
	}
	return fmt.Errorf("本会话未绑定计划 %s，用户也未点名；不要读取或改其他计划", name)
}

// filterPlanList 默认只返回本会话范围内的计划名。
func filterPlanList(input tool.Input, names []string) planListOutput {
	scope := planScopeOf(input)
	if scope.AllowListAll {
		return planListOutput{Names: names}
	}
	allowed := make([]string, 0, len(names))
	for _, name := range names {
		if scope.Allows(name) {
			allowed = append(allowed, name)
		}
	}
	out := planListOutput{Names: allowed}
	if len(allowed) == 0 {
		out.Note = "本会话尚未绑定计划，且用户未点名已有计划。请为当前任务新建一篇，不要读取目录里的其他计划。"
	}
	return out
}

// planItemsFromWrite 把工具入参里的验收项转成契约条目。
func planItemsFromWrite(items []planWriteItemInput) []pkgagent.PlanItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]pkgagent.PlanItem, 0, len(items))
	for _, item := range items {
		out = append(out, pkgagent.PlanItem{
			ID:          item.ID,
			Description: item.Description,
			VerifyCmd:   item.VerifyCmd,
		})
	}
	return out
}

// enforcePlanTestBinding 要求业务逻辑验收项绑定测试命令。
func enforcePlanTestBinding(contract pkgagent.PlanContract) error {
	for _, item := range contract.Items {
		if (item.IsTestBound || pkgagent.RequiresTestCommand(item.Description, item.VerifyCmd)) && !pkgagent.IsTestVerifyCmd(item.VerifyCmd) {
			return fmt.Errorf("验收项 %s 涉及业务逻辑，verify_cmd 必须是测试命令", item.ID)
		}
	}
	return nil
}
