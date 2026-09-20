package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	ChatKindEvaluator      = "evaluator"
	defaultEvaluateMaxB    = 60 * 1024
	defaultEvaluateTimeout = 30 * time.Second
)

// EvaluationVerdict 独立旁路复审裁决。
type EvaluationVerdict string

const (
	VerdictPass      EvaluationVerdict = "pass"       // 准予收工
	VerdictNeedsWork EvaluationVerdict = "needs_work" // 打回主模型修复
	VerdictEscalate  EvaluationVerdict = "escalate"   // 说不清，转人工
)

// IssueCategory 复审缺陷分类。
type IssueCategory string

const (
	IssueDeletedTest    IssueCategory = "deleted_test"
	IssueWeakenedAssert IssueCategory = "weakened_assert"
	IssueFalseClaim     IssueCategory = "false_claim"
	IssueOutOfScope     IssueCategory = "out_of_scope"
	IssueLogicDefect    IssueCategory = "logic_defect"
)

// EvaluationIssue 审查发现的一条缺陷。
type EvaluationIssue struct {
	FilePath     string        `json:"file_path,omitempty"`
	Line         int           `json:"line,omitempty"`
	Category     IssueCategory `json:"category"`
	Reason       string        `json:"reason"`
	SuggestedFix string        `json:"suggested_fix,omitempty"`
}

// EvaluationResult 独立旁路复审报告。
type EvaluationResult struct {
	Verdict         EvaluationVerdict `json:"verdict"`
	Issues          []EvaluationIssue `json:"issues,omitempty"`
	Summary         string            `json:"summary,omitempty"`
	DiffFingerprint string            `json:"diff_fingerprint,omitempty"`
	TokensUsed      int               `json:"tokens_used,omitempty"`
}

// SanitizeDiff 去掉 lock / 生成文件与纯空白变动，并按字节上限截断。
func SanitizeDiff(rawDiff string, maxBytes int) (string, error) {
	if maxBytes <= 0 {
		maxBytes = defaultEvaluateMaxB
	}
	filtered := filterDiffNoise(rawDiff)
	if len(filtered) <= maxBytes {
		return filtered, nil
	}
	return filtered[:maxBytes] + "\n...[diff truncated]...", nil
}

// DiffFingerprint 计算送审 diff 的稳定哈希。
func DiffFingerprint(sanitizedDiff string) string {
	sum := sha256.Sum256([]byte(sanitizedDiff))
	return hex.EncodeToString(sum[:16])
}

// EvaluateRun 对照改动与验收契约做一次客观复审。
func EvaluateRun(ctx context.Context, model ModelConfig, sanitizedDiff string, items []PlanItem, verifySummary, lastFingerprint string) (EvaluationResult, error) {
	fp := DiffFingerprint(sanitizedDiff)
	if nothingToReview(sanitizedDiff, items) {
		return EvaluationResult{
			Verdict:         VerdictPass,
			Summary:         "没有可审的代码改动，跳过复审。",
			DiffFingerprint: fp,
		}, nil
	}
	if lastFingerprint != "" && fp == lastFingerprint {
		return EvaluationResult{
			Verdict:         VerdictNeedsWork,
			Summary:         "代码相对上次复审没有改动，复用上次驳回。",
			DiffFingerprint: fp,
			Issues: []EvaluationIssue{{
				Category: IssueFalseClaim,
				Reason:   "申请结束时 diff 与上次驳回相同，没有新的修复。",
			}},
		}, nil
	}
	switch strings.ToLower(model.Provider) {
	case "", "fake":
		result, err := evaluateFake(model)
		result.DiffFingerprint = fp
		return result, err
	case "openai":
		result, err := evaluateOpenAI(ctx, model, sanitizedDiff, items, verifySummary)
		result.DiffFingerprint = fp
		return result, err
	default:
		return EvaluationResult{Verdict: VerdictEscalate, Summary: "unsupported evaluator", DiffFingerprint: fp}, nil
	}
}

// evaluateFake 按脚本产出确定性复审结论。
func evaluateFake(model ModelConfig) (EvaluationResult, error) {
	opts := ParseFakeOptions(model.Options)
	if len(opts.Evaluate) == 0 {
		return EvaluationResult{Verdict: VerdictPass, Summary: "fake evaluate pass"}, nil
	}
	item := opts.Evaluate[0]
	if item.Fail {
		return EvaluationResult{}, fmt.Errorf("fake evaluate failed")
	}
	verdict := EvaluationVerdict(item.Verdict)
	if verdict == "" {
		verdict = VerdictPass
	}
	return EvaluationResult{Verdict: verdict, Issues: item.Issues, Summary: item.Summary}, nil
}

// evaluateOpenAI 用独立聊天请求做结构化复审；正文读不出来时再试一次。
func evaluateOpenAI(ctx context.Context, model ModelConfig, sanitizedDiff string, items []PlanItem, verifySummary string) (EvaluationResult, error) {
	first, err := requestEvaluateCompletion(ctx, model, sanitizedDiff, items, verifySummary)
	if err != nil {
		if ctx.Err() != nil {
			return EvaluationResult{}, ctx.Err()
		}
		return EvaluationResult{Verdict: VerdictEscalate, Summary: err.Error()}, nil
	}
	result, ok := resolveEvaluationContent(first.content, items)
	if ok {
		result.TokensUsed = first.tokens
		return result, nil
	}
	retry, retryErr := requestEvaluateCompletion(ctx, model, sanitizedDiff, items, verifySummary)
	if retryErr == nil {
		result, ok = resolveEvaluationContent(retry.content, items)
		if ok {
			result.TokensUsed = first.tokens + retry.tokens
			return result, nil
		}
		first.content = retry.content
	}
	snippet := strings.TrimSpace(first.content)
	if len(snippet) > 160 {
		snippet = snippet[:160] + "…"
	}
	if snippet == "" {
		snippet = "empty model content"
	}
	return EvaluationResult{Verdict: VerdictEscalate, Summary: "openai evaluate unclear: " + snippet}, nil
}

// evaluateCompletion 是一次复审 HTTP 回复。
type evaluateCompletion struct {
	content string
	tokens  int
}

// requestEvaluateCompletion 发一次非流式复审请求。
func requestEvaluateCompletion(ctx context.Context, model ModelConfig, sanitizedDiff string, items []PlanItem, verifySummary string) (evaluateCompletion, error) {
	var opts openaiOptions
	_ = json.Unmarshal(model.Options, &opts)
	if opts.APIKey == "" {
		return evaluateCompletion{}, fmt.Errorf("openai api key is required")
	}
	base := strings.TrimRight(opts.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	chat := Chat{
		Kind:         ChatKindEvaluator,
		Model:        model,
		SystemPrompt: evaluatorPrompt(),
		Messages: []Message{{
			Role:    RoleUser,
			Content: EncodeText(evaluatorUserText(sanitizedDiff, items, verifySummary)),
		}},
		MaxOutputTokens: 1024,
	}
	reqBody := openaiChatRequest{
		Model:    model.Model,
		Stream:   false,
		Messages: toOpenAIMessages(chat),
	}
	applyOutputLimit(&reqBody, model.Model, chat.MaxOutputTokens)
	body, err := json.Marshal(reqBody)
	if err != nil {
		return evaluateCompletion{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, defaultEvaluateTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return evaluateCompletion{}, err
	}
	req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return evaluateCompletion{}, err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return evaluateCompletion{}, fmt.Errorf("openai status %d", resp.StatusCode)
	}
	var parsed struct {
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil || len(parsed.Choices) == 0 {
		return evaluateCompletion{}, fmt.Errorf("openai evaluate parse failed")
	}
	return evaluateCompletion{content: parsed.Choices[0].Message.Content, tokens: parsed.Usage.TotalTokens}, nil
}

// resolveEvaluationContent 解析复审正文；空结论且无验收项时按验证结果放行。
func resolveEvaluationContent(content string, items []PlanItem) (EvaluationResult, bool) {
	result, ok := parseEvaluationContent(content)
	if ok {
		return result, true
	}
	if strings.TrimSpace(content) == "" && len(items) == 0 {
		return EvaluationResult{
			Verdict: VerdictPass,
			Summary: "复审模型未返回结论，无验收项，按验证结果放行。",
		}, true
	}
	return EvaluationResult{}, false
}

// evaluatorPrompt 要求复审模型只输出短 JSON。
func evaluatorPrompt() string {
	return `你是独立代码复审员。只根据 diff、验收清单和验证摘要判断，不猜测对话里没有的意图。

必须检查：是否删除既有测试、是否弱化断言、验收项是否空口宣称通过、改动是否越出需求、有无明显逻辑缺陷。也要看新增测试是不是只覆盖最顺利路径或断言永远为真。

diff 为空只表示相对快照没有文件改动，不是补丁提交失败。没有 diff 且没有验收项时不要驳回。验收清单或验证摘要为空时，只根据已有字段判断，不要因为缺字段就 escalate。

只输出 JSON 对象：{"verdict":"pass|needs_work|escalate","summary":"...","issues":[{"file_path":"","line":0,"category":"deleted_test|weakened_assert|false_claim|out_of_scope|logic_defect","reason":"...","suggested_fix":"..."}]}。
verdict=pass 时 issues 为空。说不清就 escalate。`
}

// evaluatorUserText 拼出复审用户消息。
func evaluatorUserText(diff string, items []PlanItem, verifySummary string) string {
	if items == nil {
		items = []PlanItem{}
	}
	raw, _ := json.Marshal(map[string]any{
		"diff":           diff,
		"plan_items":     items,
		"verify_summary": verifySummary,
	})
	return "请复审这次改动：\n" + string(raw)
}

// nothingToReview 没有代码 diff 时不送模型；单有验收项也无法对照改动。
func nothingToReview(diff string, _ []PlanItem) bool {
	return strings.TrimSpace(diff) == ""
}

// parseEvaluationContent 从模型正文抽出复审 JSON，兼容常见别名和缺字段。
func parseEvaluationContent(content string) (EvaluationResult, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return EvaluationResult{}, false
	}
	if i := strings.Index(content, "{"); i >= 0 {
		if j := strings.LastIndex(content, "}"); j > i {
			content = content[i : j+1]
		}
	}
	var raw struct {
		Verdict string            `json:"verdict"`
		Issues  []EvaluationIssue `json:"issues"`
		Summary string            `json:"summary"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return EvaluationResult{}, false
	}
	result := EvaluationResult{Issues: raw.Issues, Summary: raw.Summary}
	result.Verdict = normalizeVerdict(raw.Verdict, result.Issues)
	switch result.Verdict {
	case VerdictPass, VerdictNeedsWork, VerdictEscalate:
	default:
		return EvaluationResult{}, false
	}
	if result.Verdict == VerdictNeedsWork && len(result.Issues) == 0 {
		reason := strings.TrimSpace(result.Summary)
		if reason == "" {
			reason = "复审要求继续修改，但没有列出具体问题。"
		}
		result.Issues = []EvaluationIssue{{Category: IssueLogicDefect, Reason: reason}}
	}
	return result, true
}

// normalizeVerdict 把模型给出的通过/驳回说法收成三种裁决。
func normalizeVerdict(raw string, issues []EvaluationIssue) EvaluationVerdict {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pass", "passed", "ok", "approve", "approved", "通过", "合格":
		return VerdictPass
	case "needs_work", "needs-work", "need_work", "reject", "rejected", "fail", "failed", "不通过", "需修改", "驳回":
		return VerdictNeedsWork
	case "escalate", "unclear", "unknown", "转人工", "说不清":
		return VerdictEscalate
	}
	if len(issues) > 0 {
		return VerdictNeedsWork
	}
	return ""
}

// filterDiffNoise 去掉 lock、生成物和纯空白 hunk。
func filterDiffNoise(raw string) string {
	blocks := strings.Split(raw, "diff --git ")
	var kept []string
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		header, _, _ := strings.Cut(block, "\n")
		path := diffPathFromHeader(header)
		if shouldDropDiffPath(path) {
			continue
		}
		if isWhitespaceOnlyDiff(block) {
			continue
		}
		kept = append(kept, "diff --git "+block)
	}
	return strings.Join(kept, "\n")
}

// diffPathFromHeader 从 diff --git 行取出 b 路径。
func diffPathFromHeader(header string) string {
	fields := strings.Fields(header)
	if len(fields) == 0 {
		return header
	}
	last := fields[len(fields)-1]
	return strings.TrimPrefix(last, "b/")
}

// shouldDropDiffPath 判断路径是否不值得送审。
func shouldDropDiffPath(path string) bool {
	name := strings.ToLower(filepathBase(path))
	if name == "go.sum" || name == "pnpm-lock.yaml" || name == "package-lock.json" || name == "yarn.lock" || name == "cargo.lock" {
		return true
	}
	if strings.Contains(path, "generated") || strings.HasSuffix(path, ".pb.go") || strings.HasSuffix(path, ".gen.go") {
		return true
	}
	return false
}

// isWhitespaceOnlyDiff 判断 hunk 是否只有空白变化。
func isWhitespaceOnlyDiff(block string) bool {
	hasPlus := false
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			if strings.TrimSpace(strings.TrimPrefix(line, "+")) != "" {
				return false
			}
			hasPlus = true
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			if strings.TrimSpace(strings.TrimPrefix(line, "-")) != "" {
				return false
			}
			hasPlus = true
		}
	}
	return hasPlus
}

func filepathBase(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
