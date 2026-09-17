package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const ChatKindReviewer = "reviewer"

// ReviewResult 是独立复审的结果。Escalate 表示说不清，交给人。
type ReviewResult struct {
	Decisions []ApprovalDecision
	Escalate  bool
	Reason    string
}

// Review 用独立复审模型裁定一批仍为 ask 的工具调用。
func Review(ctx context.Context, model ModelConfig, calls []ApprovalToolCall) (ReviewResult, error) {
	if err := ctx.Err(); err != nil {
		return ReviewResult{}, err
	}
	if len(calls) == 0 {
		return ReviewResult{Escalate: true, Reason: "no tool calls"}, nil
	}
	switch strings.ToLower(model.Provider) {
	case "", "fake":
		return reviewFake(model, calls)
	case "openai":
		return reviewOpenAI(ctx, model, calls)
	default:
		return ReviewResult{Escalate: true, Reason: "unsupported reviewer"}, nil
	}
}

func reviewFake(model ModelConfig, calls []ApprovalToolCall) (ReviewResult, error) {
	opts := ParseFakeOptions(model.Options)
	if opts.Review == nil || opts.Review.Escalate {
		return ReviewResult{Escalate: true, Reason: "fake review escalate"}, nil
	}
	if opts.Review.Fail {
		return ReviewResult{}, fmt.Errorf("fake review failed")
	}
	if !coverReview(opts.Review.Decisions, calls) {
		return ReviewResult{Escalate: true, Reason: "fake review incomplete"}, nil
	}
	return ReviewResult{Decisions: opts.Review.Decisions}, nil
}

func reviewOpenAI(ctx context.Context, model ModelConfig, calls []ApprovalToolCall) (ReviewResult, error) {
	var opts openaiOptions
	_ = json.Unmarshal(model.Options, &opts)
	if opts.APIKey == "" {
		return ReviewResult{Escalate: true, Reason: "openai api key is required"}, nil
	}
	base := strings.TrimRight(opts.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	chat := Chat{
		Kind:         ChatKindReviewer,
		Model:        model,
		SystemPrompt: reviewerPrompt(),
		Messages: []Message{{
			Role:    RoleUser,
			Content: EncodeText(reviewerUserText(calls)),
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
		return ReviewResult{Escalate: true, Reason: err.Error()}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ReviewResult{Escalate: true, Reason: err.Error()}, nil
	}
	req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ReviewResult{}, ctx.Err()
		}
		return ReviewResult{Escalate: true, Reason: err.Error()}, nil
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return ReviewResult{Escalate: true, Reason: fmt.Sprintf("openai status %d", resp.StatusCode)}, nil
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil || len(parsed.Choices) == 0 {
		return ReviewResult{Escalate: true, Reason: "openai review parse failed"}, nil
	}
	decisions, ok := parseReviewContent(parsed.Choices[0].Message.Content, calls)
	if !ok {
		return ReviewResult{Escalate: true, Reason: "openai review unclear"}, nil
	}
	return ReviewResult{Decisions: decisions}, nil
}

func reviewerPrompt() string {
	return `你是 CodeDock 的独立工具复审员。只根据工具名和参数裁定，不猜测参数里没有的用户意图，也不替用户扩大权限。

审核标准：
- approved：范围清楚、可逆或常见开发操作。例如项目内相对路径的读/搜/列目录、常规 write/edit、.cursor/ 下的 plan_write、非破坏性的 bash/powershell、看起来像项目笔记的 memory_write。
- denied：破坏性、越权或泄密。例如 rm -rf、sudo、curl|sh、格式化磁盘、改 SSH/密钥、外传密钥或环境变量、路径带 .. 或落到 /etc /usr /var 或家目录、清空仓库、force push、改 git remote。
- 说不清：参数残缺、路径看不出是否在项目内、命令意图含糊。不要编造 approved 或 denied。

输出：只输出 JSON 数组，每项含 tool_call_id、status（approved 或 denied）、reason（一句中文）。不要其他文字。任何一条说不清就不要给出完整数组，宁可整批漏项升级给人。`
}

func reviewerUserText(calls []ApprovalToolCall) string {
	raw, _ := json.Marshal(calls)
	return "请复审这些工具调用：\n" + string(raw)
}

func parseReviewContent(content string, calls []ApprovalToolCall) ([]ApprovalDecision, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, false
	}
	if i := strings.Index(content, "["); i >= 0 {
		if j := strings.LastIndex(content, "]"); j > i {
			content = content[i : j+1]
		}
	}
	var decisions []ApprovalDecision
	if err := json.Unmarshal([]byte(content), &decisions); err != nil {
		return nil, false
	}
	return decisions, coverReview(decisions, calls)
}

func coverReview(decisions []ApprovalDecision, calls []ApprovalToolCall) bool {
	if len(decisions) != len(calls) {
		return false
	}
	want := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		want[call.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(decisions))
	for _, item := range decisions {
		if item.Status != ApprovalApproved && item.Status != ApprovalDenied {
			return false
		}
		if _, ok := want[item.ToolCallID]; !ok {
			return false
		}
		if _, dup := seen[item.ToolCallID]; dup {
			return false
		}
		seen[item.ToolCallID] = struct{}{}
	}
	return len(seen) == len(want)
}
