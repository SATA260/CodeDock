package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"codedock/pkg/agent/tool"
)

var (
	planFileNamePattern = regexp.MustCompile(`(?i)(?:\.cursor/)?([a-z0-9][a-z0-9._-]{0,80})\.md`)
	planCatalogPattern  = regexp.MustCompile(`(?i)((列出|有哪些|全部|所有).{0,12}(计划|plan)|(list|show|which)\s+(all\s+)?plans)`)
)

var ignoredPlanMentions = map[string]struct{}{
	"readme.md":       {},
	"agents.md":       {},
	"changelog.md":    {},
	"license.md":      {},
	"contributing.md": {},
}

// PlanScope 是一次会话对计划文件的可见范围。
type PlanScope struct {
	ActivePlan   string   // 本会话已绑定的计划文件名
	Mentioned    []string // 用户点名的计划文件名
	AllowListAll bool     // 用户明确要求列出已有计划
}

// Allows 判断该计划是否属于本会话范围。
func (s PlanScope) Allows(name string) bool {
	name = NormalizePlanFileName(name)
	if name == "" {
		return false
	}
	if NormalizePlanFileName(s.ActivePlan) == name {
		return true
	}
	for _, item := range s.Mentioned {
		if NormalizePlanFileName(item) == name {
			return true
		}
	}
	return false
}

// ResolvePlanScope 从已绑定名和会话消息推出本轮计划范围。
func ResolvePlanScope(active string, messages []Message) PlanScope {
	mentioned := make([]string, 0, 2)
	lastUser := ""
	for _, msg := range messages {
		if msg.Role != RoleUser {
			continue
		}
		lastUser = DecodeText(msg.Content)
		mentioned = append(mentioned, MentionedPlanNames(lastUser)...)
	}
	active = NormalizePlanFileName(active)
	if active == "" {
		active = InferActivePlan(messages)
	}
	mentioned = uniquePlanNames(mentioned)
	if active == "" && len(mentioned) == 1 {
		active = mentioned[0]
	}
	return PlanScope{
		ActivePlan:   active,
		Mentioned:    mentioned,
		AllowListAll: WantsPlanCatalog(lastUser),
	}
}

// MentionedPlanNames 从用户正文抽出点名的计划文件。
func MentionedPlanNames(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	matches := planFileNamePattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := NormalizePlanFileName(match[1] + ".md")
		if name == "" {
			continue
		}
		if _, skip := ignoredPlanMentions[strings.ToLower(name)]; skip {
			continue
		}
		names = append(names, name)
	}
	return uniquePlanNames(names)
}

// WantsPlanCatalog 判断用户是否在要求列出已有计划。
func WantsPlanCatalog(text string) bool {
	return planCatalogPattern.MatchString(strings.TrimSpace(text))
}

// InferActivePlan 从会话里最近一次成功的计划工具结果还原绑定名。
func InferActivePlan(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != RoleTool {
			continue
		}
		if name := planNameFromToolContent(messages[i].Content); name != "" {
			return name
		}
	}
	return ""
}

// PlanNameFromToolResult 从计划工具成功结果取出文件名。
func PlanNameFromToolResult(result tool.Result) string {
	if !result.Success {
		return ""
	}
	switch result.Name {
	case "plan_write", "plan_read", "plan_pass":
	default:
		return ""
	}
	return planNameFromOutput(result.Output)
}

// BindActivePlan 用本批计划工具结果更新会话绑定；后写的覆盖先写的。
func BindActivePlan(active string, results []tool.Result) string {
	active = NormalizePlanFileName(active)
	for _, result := range results {
		if name := PlanNameFromToolResult(result); name != "" {
			active = name
		}
	}
	return active
}

// NormalizePlanFileName 把路径或无后缀名收成 .cursor 下的短文件名。
func NormalizePlanFileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if name == "" || strings.Contains(name, "..") {
		return ""
	}
	if !strings.HasSuffix(strings.ToLower(name), ".md") {
		name += ".md"
	}
	if strings.EqualFold(name, ".md") {
		return ""
	}
	return name
}

// LoadPlanItems 只读一篇绑定计划的验收项；名为空或解析失败则返回空。
func LoadPlanItems(workspaceRoot, planName string) []PlanItem {
	planName = NormalizePlanFileName(planName)
	if strings.TrimSpace(workspaceRoot) == "" || planName == "" {
		return nil
	}
	body, err := os.ReadFile(filepath.Join(workspaceRoot, ".cursor", planName))
	if err != nil {
		return nil
	}
	contract, _, err := ParsePlanContract(string(body))
	if err != nil {
		return nil
	}
	return contract.Items
}

// PlanScopeNote 写给模型的本会话计划范围说明。
func PlanScopeNote(active string) string {
	active = NormalizePlanFileName(active)
	if active == "" {
		return "本会话尚未绑定计划。用户没有点名已有计划时，不要列出或读取 .cursor 下的其他计划；为当前任务新建一篇。"
	}
	return "本会话当前计划：" + active + "。未点名时不要读、列或改 .cursor 下的其他计划。"
}

// planNameFromToolContent 从工具消息载荷抽出计划名。
func planNameFromToolContent(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var payload ToolResultContent
	if json.Unmarshal(content, &payload) != nil {
		return planNameFromOutput(content)
	}
	return planNameFromOutput(payload.Output)
}

// planNameFromOutput 从计划工具 JSON 取出 name。
func planNameFromOutput(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var body struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return ""
	}
	name := strings.TrimSpace(body.Name)
	if !strings.HasSuffix(strings.ToLower(name), ".md") {
		return ""
	}
	return NormalizePlanFileName(name)
}

// uniquePlanNames 去重并保持出现顺序。
func uniquePlanNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = NormalizePlanFileName(name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, name)
	}
	return out
}
