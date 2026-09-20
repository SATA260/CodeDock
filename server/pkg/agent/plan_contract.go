package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PlanItem 计划验收清单中的一条契约。
type PlanItem struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	VerifyCmd   string `json:"verify_cmd"`
	Passes      bool   `json:"passes"`
	Evidence    string `json:"evidence,omitempty"`
	IsTestBound bool   `json:"is_test_bound"`
}

// PlanContract 计划文件的结构化契约。
type PlanContract struct {
	PlanName  string     `json:"plan_name,omitempty"`
	Title     string     `json:"title,omitempty"`
	Items     []PlanItem `json:"items"`
	UpdatedAt int64      `json:"updated_at,omitempty"`
}

// ValidatePlan 校验计划验收清单，禁止删项或弱化验证命令。
func ValidatePlan(content string, existing *PlanContract) (PlanContract, error) {
	contract, _, err := ParsePlanContract(content)
	if err != nil {
		return PlanContract{}, err
	}
	if len(contract.Items) == 0 {
		return PlanContract{}, fmt.Errorf("计划缺少验收清单。%s。例：\n%s", PlanItemFieldHint, strings.TrimSpace(PlanMarkdownExample))
	}
	seen := map[string]struct{}{}
	for i, item := range contract.Items {
		if strings.TrimSpace(item.ID) == "" {
			return PlanContract{}, fmt.Errorf("验收项缺少 id。%s", PlanItemFieldHint)
		}
		if _, dup := seen[item.ID]; dup {
			return PlanContract{}, fmt.Errorf("验收项 id 重复：%s", item.ID)
		}
		seen[item.ID] = struct{}{}
		if strings.TrimSpace(item.Description) == "" {
			return PlanContract{}, fmt.Errorf("验收项 %s 缺少说明。%s", item.ID, PlanItemFieldHint)
		}
		if strings.TrimSpace(item.VerifyCmd) == "" {
			return PlanContract{}, fmt.Errorf("验收项 %s 缺少验证命令。%s", item.ID, PlanItemFieldHint)
		}
		if (item.IsTestBound || RequiresTestCommand(item.Description, item.VerifyCmd)) && !IsTestVerifyCmd(item.VerifyCmd) {
			return PlanContract{}, fmt.Errorf("验收项 %s 涉及后端或 core 逻辑，verify_cmd 必须是具体测试命令", item.ID)
		}
		if item.Passes && strings.TrimSpace(item.Evidence) == "" {
			return PlanContract{}, fmt.Errorf("验收项 %s 标记通过时必须有 evidence", item.ID)
		}
		if existing == nil && item.Passes {
			return PlanContract{}, fmt.Errorf("新建计划时验收项 %s 不能直接标为通过", item.ID)
		}
		contract.Items[i] = item
	}
	if existing != nil {
		if err := ensurePlanNotWeakened(*existing, contract); err != nil {
			return PlanContract{}, err
		}
	}
	if contract.UpdatedAt == 0 {
		contract.UpdatedAt = time.Now().UnixMilli()
	}
	return contract, nil
}

// MarkPassed 将指定验收项单向标为通过。
func MarkPassed(contract PlanContract, itemID, evidence string) (PlanContract, error) {
	if strings.TrimSpace(evidence) == "" {
		return PlanContract{}, fmt.Errorf("evidence is required")
	}
	for i, item := range contract.Items {
		if item.ID != itemID {
			continue
		}
		item.Passes = true
		item.Evidence = evidence
		contract.Items[i] = item
		contract.UpdatedAt = time.Now().UnixMilli()
		return contract, nil
	}
	return PlanContract{}, fmt.Errorf("验收项不存在：%s", itemID)
}

// ParsePlanContract 从 Markdown 验收列表或旧 frontmatter 抽出契约。
func ParsePlanContract(content string) (PlanContract, string, error) {
	content = strings.TrimPrefix(content, "\ufeff")
	if meta, body, ok := splitFrontmatter(content); ok {
		contract, err := parsePlanMeta(meta)
		if err != nil {
			return PlanContract{}, body, err
		}
		return contract, body, nil
	}
	contract, body, err := parseMarkdownPlan(content)
	if err != nil {
		return PlanContract{}, content, err
	}
	if len(contract.Items) == 0 {
		return PlanContract{}, content, fmt.Errorf("计划缺少验收清单。%s。例：\n%s", PlanItemFieldHint, strings.TrimSpace(PlanMarkdownExample))
	}
	return normalizePlanItems(contract), body, nil
}

// LoadWorkspacePlanItems 收集工作区 .cursor 下各计划的验收项，解析失败的文件跳过。
// 复审应走 LoadPlanItems，只对照本会话绑定的那一篇。
func LoadWorkspacePlanItems(workspaceRoot string) []PlanItem {
	if strings.TrimSpace(workspaceRoot) == "" {
		return nil
	}
	dir := filepath.Join(workspaceRoot, ".cursor")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var items []PlanItem
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		contract, _, err := ParsePlanContract(string(body))
		if err != nil {
			continue
		}
		items = append(items, contract.Items...)
	}
	return items
}

// RenderPlan 把契约写成给人看的 Markdown，保留正文。
func RenderPlan(contract PlanContract, body string) string {
	body = stripLeadingHeading(body)
	var b strings.Builder
	title := strings.TrimSpace(contract.Title)
	if title == "" {
		title = strings.TrimSuffix(strings.TrimSpace(contract.PlanName), ".md")
	}
	if title != "" {
		b.WriteString("# ")
		b.WriteString(title)
		b.WriteString("\n\n")
	}
	b.WriteString("## 验收\n\n")
	for _, item := range contract.Items {
		if item.Passes {
			b.WriteString("- [x] ")
		} else {
			b.WriteString("- [ ] ")
		}
		b.WriteString(strings.TrimSpace(item.ID))
		if desc := strings.TrimSpace(item.Description); desc != "" {
			b.WriteByte(' ')
			b.WriteString(desc)
		}
		if cmd := strings.TrimSpace(item.VerifyCmd); cmd != "" {
			b.WriteString(" — `")
			b.WriteString(cmd)
			b.WriteString("`")
		}
		b.WriteByte('\n')
		if item.Passes && strings.TrimSpace(item.Evidence) != "" {
			b.WriteString("  依据：")
			b.WriteString(strings.TrimSpace(item.Evidence))
			b.WriteByte('\n')
		}
	}
	if body != "" {
		b.WriteByte('\n')
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// RequiresTestCommand 判断验收项是否必须绑定自动化测试。
func RequiresTestCommand(description, verifyCmd string) bool {
	text := strings.ToLower(description + " " + verifyCmd)
	if strings.Contains(text, "文档") || strings.Contains(text, "css") || strings.Contains(text, "文案") || strings.Contains(text, "manual") {
		return false
	}
	return strings.Contains(text, "server/") || strings.Contains(text, "packages/core") || strings.Contains(text, "pkg/") || strings.Contains(text, "internal/")
}

// IsTestVerifyCmd 判断验证命令是不是测试命令。
func IsTestVerifyCmd(cmd string) bool {
	cmd = strings.ToLower(strings.TrimSpace(cmd))
	return strings.Contains(cmd, "go test") || strings.Contains(cmd, "vitest") || strings.Contains(cmd, "pnpm test") || strings.HasSuffix(cmd, "test")
}

// ensurePlanNotWeakened 禁止删除已有条目或弱化验证命令。
func ensurePlanNotWeakened(existing, next PlanContract) error {
	byID := map[string]PlanItem{}
	for _, item := range next.Items {
		byID[item.ID] = item
	}
	for _, old := range existing.Items {
		cur, ok := byID[old.ID]
		if !ok {
			return fmt.Errorf("不能删除已有验收项 %s", old.ID)
		}
		if old.Passes && !cur.Passes {
			return fmt.Errorf("不能把已通过的验收项 %s 改回未通过", old.ID)
		}
		if weakenedVerify(old.VerifyCmd, cur.VerifyCmd) {
			return fmt.Errorf("不能弱化验收项 %s 的验证命令", old.ID)
		}
		if old.IsTestBound && !cur.IsTestBound {
			return fmt.Errorf("不能取消验收项 %s 的测试绑定", old.ID)
		}
	}
	return nil
}

// weakenedVerify 认为从具体测试改成 manual/空命令是弱化。
func weakenedVerify(oldCmd, newCmd string) bool {
	oldCmd = strings.TrimSpace(oldCmd)
	newCmd = strings.TrimSpace(newCmd)
	if oldCmd == newCmd {
		return false
	}
	if IsTestVerifyCmd(oldCmd) && !IsTestVerifyCmd(newCmd) {
		return true
	}
	return false
}

// parseMarkdownPlan 从标题和勾选列表抽出契约，验收节不进入正文。
func parseMarkdownPlan(content string) (PlanContract, string, error) {
	var contract PlanContract
	var body []string
	inAcceptance := false
	sawAcceptance := false
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimRight(raw, "\r")
		if contract.Title == "" && isATXHeading(line, 1) {
			contract.Title = headingText(line)
			continue
		}
		if isAcceptanceHeading(line) {
			inAcceptance = true
			sawAcceptance = true
			continue
		}
		if inAcceptance && isATXHeading(line, 0) && !isAcceptanceHeading(line) {
			inAcceptance = false
		}
		if inAcceptance || (!sawAcceptance && looksLikeAcceptanceItem(line)) {
			if item, ok := parseAcceptanceLine(line); ok {
				contract.Items = append(contract.Items, item)
				continue
			}
			if ev, ok := parseEvidenceLine(line); ok && len(contract.Items) > 0 {
				contract.Items[len(contract.Items)-1].Evidence = ev
				continue
			}
			if strings.TrimSpace(line) == "" {
				continue
			}
			if inAcceptance {
				continue
			}
		}
		if !inAcceptance {
			body = append(body, line)
		}
	}
	return contract, strings.TrimSpace(strings.Join(body, "\n")), nil
}

// stripLeadingHeading 去掉正文开头的一级标题，避免和契约标题重复。
func stripLeadingHeading(body string) string {
	body = strings.TrimLeft(body, "\n")
	line, rest, found := strings.Cut(body, "\n")
	if !found {
		if isATXHeading(line, 1) {
			return ""
		}
		return body
	}
	if isATXHeading(line, 1) {
		return strings.TrimLeft(rest, "\n")
	}
	return body
}

// isATXHeading 判断 ATX 标题；level 为 0 时接受任意级别。
func isATXHeading(line string, level int) bool {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "#") {
		return false
	}
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(line) || line[n] != ' ' {
		return false
	}
	if level > 0 && n != level {
		return false
	}
	return headingText(line) != ""
}

// headingText 取出 ATX 标题正文。
func headingText(line string) string {
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
}

// isAcceptanceHeading 判断验收清单小节标题。
func isAcceptanceHeading(line string) bool {
	if !isATXHeading(line, 0) {
		return false
	}
	switch strings.ToLower(headingText(line)) {
	case "验收", "验收清单", "acceptance", "checklist":
		return true
	default:
		return false
	}
}

// looksLikeAcceptanceItem 判断一行是不是勾选验收项。
func looksLikeAcceptanceItem(line string) bool {
	_, ok := parseAcceptanceLine(line)
	return ok
}

// parseAcceptanceLine 解析 `- [ ] V1 说明 — \`命令\“。
func parseAcceptanceLine(line string) (PlanItem, bool) {
	trimmed := strings.TrimSpace(line)
	checked := false
	rest := ""
	switch {
	case strings.HasPrefix(trimmed, "- [x] "), strings.HasPrefix(trimmed, "- [X] "), strings.HasPrefix(trimmed, "* [x] "), strings.HasPrefix(trimmed, "* [X] "):
		checked = true
		rest = strings.TrimSpace(trimmed[6:])
	case strings.HasPrefix(trimmed, "- [ ] "), strings.HasPrefix(trimmed, "* [ ] "):
		rest = strings.TrimSpace(trimmed[6:])
	default:
		return PlanItem{}, false
	}
	desc, verify := splitVerifySuffix(rest)
	id, desc := splitItemID(desc)
	if id == "" || strings.TrimSpace(desc) == "" {
		return PlanItem{}, false
	}
	return PlanItem{ID: id, Description: strings.TrimSpace(desc), VerifyCmd: verify, Passes: checked}, true
}

// splitVerifySuffix 抽出行尾验证命令。
func splitVerifySuffix(rest string) (string, string) {
	if i := strings.LastIndex(rest, " — "); i >= 0 {
		return strings.TrimSpace(rest[:i]), extractCmd(rest[i+len(" — "):])
	}
	if i := strings.LastIndex(rest, "（验证："); i >= 0 {
		return strings.TrimSpace(rest[:i]), extractCmd(strings.TrimSuffix(strings.TrimSpace(rest[i+len("（验证："):]), "）"))
	}
	if i := strings.LastIndex(strings.ToLower(rest), "(verify:"); i >= 0 {
		return strings.TrimSpace(rest[:i]), extractCmd(strings.TrimSuffix(strings.TrimSpace(rest[i+len("(verify:"):]), ")"))
	}
	return rest, ""
}

// extractCmd 去掉命令两侧的反引号。
func extractCmd(raw string) string {
	return strings.Trim(strings.TrimSpace(raw), "`")
}

// splitItemID 取出开头的验收编号。
func splitItemID(rest string) (string, string) {
	rest = strings.TrimSpace(strings.Trim(rest, "*"))
	id, desc, ok := strings.Cut(rest, " ")
	if !ok {
		return "", rest
	}
	id = strings.Trim(id, "*")
	if !isPlanItemID(id) {
		return "", rest
	}
	return id, desc
}

// isPlanItemID 判断短编号像 V1 / A12。
func isPlanItemID(id string) bool {
	if id == "" || len(id) > 16 {
		return false
	}
	if !((id[0] >= 'A' && id[0] <= 'Z') || (id[0] >= 'a' && id[0] <= 'z')) {
		return false
	}
	for _, r := range id[1:] {
		if r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

// parseEvidenceLine 解析勾选项下一行的依据。
func parseEvidenceLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	for _, prefix := range []string{"依据：", "依据:", "证据：", "证据:", "evidence:"} {
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix)), true
		}
	}
	return "", false
}

// splitFrontmatter 切开首个 --- 块。
func splitFrontmatter(content string) (string, string, bool) {
	content = strings.TrimPrefix(content, "\ufeff")
	if !strings.HasPrefix(content, "---") {
		return "", content, false
	}
	rest := strings.TrimPrefix(content, "---")
	rest = strings.TrimPrefix(rest, "\r\n")
	rest = strings.TrimPrefix(rest, "\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", content, false
	}
	meta := rest[:idx]
	body := rest[idx+len("\n---"):]
	body = strings.TrimPrefix(body, "\r\n")
	body = strings.TrimPrefix(body, "\n")
	return meta, body, true
}

// parsePlanMeta 把 frontmatter 解析成契约；JSON 优先，其次极简 YAML。
func parsePlanMeta(meta string) (PlanContract, error) {
	meta = strings.TrimSpace(meta)
	var contract PlanContract
	if strings.HasPrefix(meta, "{") {
		if err := json.Unmarshal([]byte(meta), &contract); err != nil {
			return PlanContract{}, err
		}
		applyLoosePlanMeta(meta, &contract)
		return normalizePlanItems(contract), nil
	}
	parsed, err := parsePlanYAML(meta)
	if err != nil {
		return PlanContract{}, err
	}
	return normalizePlanItems(parsed), nil
}

// normalizePlanItems 按 verify_cmd 补 IsTestBound。
func normalizePlanItems(contract PlanContract) PlanContract {
	for i, item := range contract.Items {
		if !item.IsTestBound {
			item.IsTestBound = IsTestVerifyCmd(item.VerifyCmd)
		}
		contract.Items[i] = item
	}
	return contract
}

// parsePlanYAML 解析本模块使用的验收清单 YAML。
func parsePlanYAML(raw string) (PlanContract, error) {
	var contract PlanContract
	var current *PlanItem
	inItems := false
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if isPlanItemsKey(trimmed) {
			inItems = true
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && inItems {
			contract.Items = append(contract.Items, PlanItem{})
			current = &contract.Items[len(contract.Items)-1]
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			if rest != "" {
				applyPlanYAMLField(&contract, current, rest, true)
			}
			continue
		}
		if inItems && current != nil {
			applyPlanYAMLField(&contract, current, trimmed, true)
			continue
		}
		applyPlanYAMLField(&contract, nil, trimmed, false)
	}
	return contract, nil
}

// applyPlanYAMLField 写入契约或当前验收项的一个字段。
func applyPlanYAMLField(contract *PlanContract, item *PlanItem, line string, inItem bool) {
	key, value, ok := strings.Cut(line, ":")
	if !ok {
		return
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	if inItem && item != nil {
		switch key {
		case "id", "item_id", "itemId":
			item.ID = value
		case "description", "desc", "text":
			item.Description = value
		case "verify_cmd", "verifyCmd", "verify", "cmd":
			item.VerifyCmd = value
		case "evidence":
			item.Evidence = value
		case "passes":
			item.Passes = value == "true" || value == "yes"
		case "is_test_bound", "isTestBound":
			item.IsTestBound = value == "true" || value == "yes"
		case "name":
			fillPlanItemName(item, value)
		case "title":
			if strings.TrimSpace(item.Description) == "" {
				item.Description = value
			}
		default:
			if item.ID == "" && item.Description == "" && value != "" {
				item.ID = key
				item.Description = value
			}
		}
		return
	}
	if key == "title" {
		contract.Title = value
	}
	if key == "plan_name" || key == "planName" {
		contract.PlanName = value
	}
}

// applyLoosePlanMeta 把 JSON frontmatter 里的别名字段填进契约。
func applyLoosePlanMeta(meta string, contract *PlanContract) {
	var raw map[string]any
	if json.Unmarshal([]byte(meta), &raw) != nil {
		return
	}
	if strings.TrimSpace(contract.Title) == "" {
		contract.Title = firstMapString(raw, "title", "name")
	}
	maps := planItemMaps(raw)
	if len(maps) == 0 {
		return
	}
	if len(contract.Items) == 0 {
		contract.Items = make([]PlanItem, len(maps))
	}
	for i, rawItem := range maps {
		if i >= len(contract.Items) {
			break
		}
		fillPlanItemFromMap(&contract.Items[i], rawItem)
	}
}

// planItemMaps 取出 items 或常见别名数组。
func planItemMaps(raw map[string]any) []map[string]any {
	for _, key := range []string{"items", "acceptance", "checklist", "todos", "tasks"} {
		arr, ok := raw[key].([]any)
		if !ok || len(arr) == 0 {
			continue
		}
		out := make([]map[string]any, 0, len(arr))
		for _, item := range arr {
			mapped, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, mapped)
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// fillPlanItemFromMap 用别名字段补 id / description / verify_cmd。
func fillPlanItemFromMap(item *PlanItem, raw map[string]any) {
	if item == nil {
		return
	}
	if strings.TrimSpace(item.ID) == "" {
		item.ID = firstMapString(raw, "id", "item_id", "itemId")
	}
	if strings.TrimSpace(item.Description) == "" {
		item.Description = firstMapString(raw, "description", "desc", "text", "title")
	}
	if strings.TrimSpace(item.VerifyCmd) == "" {
		item.VerifyCmd = firstMapString(raw, "verify_cmd", "verifyCmd", "verify", "cmd")
	}
	fillPlanItemName(item, firstMapString(raw, "name"))
}

// fillPlanItemName 短 name 当 id，否则当 description。
func fillPlanItemName(item *PlanItem, name string) {
	name = strings.TrimSpace(name)
	if name == "" || item == nil {
		return
	}
	if strings.TrimSpace(item.ID) == "" && !strings.Contains(name, " ") && len(name) <= 16 {
		item.ID = name
		return
	}
	if strings.TrimSpace(item.Description) == "" {
		item.Description = name
	}
}

// isPlanItemsKey 判断 YAML 行是否开始验收清单。
func isPlanItemsKey(trimmed string) bool {
	key, value, ok := strings.Cut(trimmed, ":")
	if !ok {
		return false
	}
	value = strings.TrimSpace(value)
	if value != "" && value != "[]" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "items", "acceptance", "checklist", "todos", "tasks":
		return true
	default:
		return false
	}
}

// firstMapString 按候选键取出第一个非空字符串。
func firstMapString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			return text
		}
	}
	return ""
}
