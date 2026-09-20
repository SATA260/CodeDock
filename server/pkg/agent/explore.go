package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codedock/pkg/agent/tool"
)

const (
	defaultExploreBudgetTokens = 10000
	defaultExploreMaxTurns     = 6
	defaultExploreMaxTools     = 12
	defaultExploreTimeout      = 60 * time.Second
	defaultExploreOutputTokens = 2000
)

// ExploreInput 只读探索子任务的输入。
type ExploreInput struct {
	Query           string   `json:"query"`
	PathHints       []string `json:"pathHints,omitempty"`
	MaxBudgetTokens int      `json:"maxBudgetTokens,omitempty"`
}

// Citation 探索子代理提炼出的代码引用。
type Citation struct {
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	Snippet   string `json:"snippet,omitempty"`
}

// ExploreOutput 探索子代理的结构化结论。
type ExploreOutput struct {
	Summary    string     `json:"summary"`
	Citations  []Citation `json:"citations,omitempty"`
	TokensUsed int        `json:"tokens_used"`
	Completed  bool       `json:"completed"`
	Error      string     `json:"error,omitempty"`
}

// ExploreRequest 是底座小循环的完整入参。
type ExploreRequest struct {
	Input          ExploreInput
	Model          ModelConfig
	Registry       tool.Registry
	WorkspaceRoot  string
	SessionID      string
	RunID          string
	BoundNames     []string
	ActivePlan     string   // 本会话已绑定的计划；探索时也不读其他计划
	MentionedPlans []string // 用户点名的计划
}

// Explore 用便宜模型跑一个只读小循环，返回带引用的摘要。
func Explore(ctx context.Context, req ExploreRequest) (ExploreOutput, error) {
	if strings.TrimSpace(req.Input.Query) == "" {
		return ExploreOutput{Completed: false, Error: "query is required"}, nil
	}
	if req.Registry == nil {
		return ExploreOutput{Completed: false, Error: "explore 失败：registry is required，请直接用 read / grep"}, nil
	}
	budget := req.Input.MaxBudgetTokens
	if budget <= 0 {
		budget = defaultExploreBudgetTokens
	}
	ctx, cancel := context.WithTimeout(ctx, defaultExploreTimeout)
	defer cancel()

	names := req.BoundNames
	if len(names) == 0 {
		names = []string{"read", "grep", "find", "ls", "memory_search"}
	}
	defs := visibleExploreTools(req.Registry, names)
	messages := []Message{{
		Role:    RoleUser,
		Content: EncodeText(exploreUserText(req.Input)),
	}}
	tokens := 0
	toolCalls := 0
	for turn := 0; turn < defaultExploreMaxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return ExploreOutput{Summary: "explore 失败：超时，请直接用 read / grep", TokensUsed: tokens, Completed: false, Error: err.Error()}, nil
		}
		if tokens >= budget {
			return ExploreOutput{Summary: "探索未完成：已达 token 预算。", TokensUsed: tokens, Completed: false}, nil
		}
		chat := Chat{
			SessionID:       req.SessionID,
			RunID:           req.RunID,
			TurnID:          fmt.Sprintf("explore-%d", turn+1),
			Kind:            "subagent",
			Model:           req.Model,
			SystemPrompt:    exploreSystemPrompt(),
			Messages:        messages,
			Tools:           defs,
			MaxOutputTokens: 1024,
		}
		stream, err := Stream(ctx, chat)
		if err != nil {
			return ExploreOutput{Summary: "explore 失败：" + err.Error() + "，请直接用 read / grep", TokensUsed: tokens, Completed: false, Error: err.Error()}, nil
		}
		result, err := stream.Result(ctx)
		_ = stream.Close()
		if err != nil {
			return ExploreOutput{Summary: "explore 失败：" + err.Error() + "，请直接用 read / grep", TokensUsed: tokens, Completed: false, Error: err.Error()}, nil
		}
		tokens += int(result.Usage.TotalTokens)
		if result.Usage.TotalTokens == 0 {
			tokens += int(CountTokens(DecodeText(result.Message.Content)))
		}
		if len(result.ToolCalls) == 0 {
			return parseExploreAnswer(DecodeText(result.Message.Content), tokens, true), nil
		}
		assistant := result.Message
		if assistant.Role == "" {
			assistant.Role = RoleAssistant
		}
		assistant.ToolCalls = result.ToolCalls
		messages = append(messages, assistant)
		remain := defaultExploreMaxTools - toolCalls
		if remain <= 0 {
			return parseExploreAnswer(DecodeText(result.Message.Content), tokens, false), nil
		}
		batch := result.ToolCalls
		if len(batch) > remain {
			batch = batch[:remain]
		}
		inv := tool.Invocation{
			SessionID:      req.SessionID,
			RunID:          req.RunID,
			TurnID:         chat.TurnID,
			WorkspaceRoot:  req.WorkspaceRoot,
			ActivePlan:     req.ActivePlan,
			MentionedPlans: req.MentionedPlans,
			Calls:          batch,
			Mode:           tool.ExecutionSerial,
			FailurePolicy:  tool.FailureBestEffort,
			MaxParallel:    1,
			BoundNames:     names,
			Approval:       tool.ApprovalYolo,
			Registry:       req.Registry,
		}
		out, err := tool.Dispatch(ctx, inv)
		if err != nil {
			return ExploreOutput{Summary: "explore 失败：" + err.Error() + "，请直接用 read / grep", TokensUsed: tokens, Completed: false, Error: err.Error()}, nil
		}
		for _, item := range out.Results {
			toolCalls++
			content := EncodeToolResult(item.CallID, item.Output)
			if !item.Success {
				content = EncodeToolError(item.CallID, item.Error)
			}
			messages = append(messages, Message{Role: RoleTool, Content: content})
		}
	}
	return ExploreOutput{Summary: "探索未完成：已达轮次上限。", TokensUsed: tokens, Completed: false}, nil
}

// visibleExploreTools 只暴露五个只读工具定义。
func visibleExploreTools(reg tool.Registry, names []string) []tool.Definition {
	out := make([]tool.Definition, 0, len(names))
	for _, name := range names {
		item, err := reg.Get(tool.Reference{Name: name})
		if err != nil {
			continue
		}
		out = append(out, item.Definition())
	}
	return out
}

// exploreSystemPrompt 要求子代理只读并输出带引用的短结论。
func exploreSystemPrompt() string {
	return `你是只读代码探索助手。只用 read、grep、find、ls、memory_search。不要写文件或跑命令。未点名时不要读 .cursor 下的其他计划文件。
结论必须短，并给出文件:行号 与原文片段。最后只输出 JSON：{"summary":"...","citations":[{"file_path":"...","start_line":1,"end_line":2,"snippet":"..."}]}。`
}

// exploreUserText 拼探索问题与范围提示。
func exploreUserText(input ExploreInput) string {
	var b strings.Builder
	b.WriteString(input.Query)
	if len(input.PathHints) > 0 {
		b.WriteString("\n范围：")
		b.WriteString(strings.Join(input.PathHints, ", "))
	}
	return b.String()
}

// parseExploreAnswer 从子代理正文抽出结构化结论。
func parseExploreAnswer(text string, tokens int, completed bool) ExploreOutput {
	text = strings.TrimSpace(text)
	out := ExploreOutput{Summary: text, TokensUsed: tokens, Completed: completed}
	if i := strings.Index(text, "{"); i >= 0 {
		if j := strings.LastIndex(text, "}"); j > i {
			var parsed ExploreOutput
			if json.Unmarshal([]byte(text[i:j+1]), &parsed) == nil && parsed.Summary != "" {
				parsed.TokensUsed = tokens
				parsed.Completed = completed
				return parsed
			}
		}
	}
	if len(out.Summary) > defaultExploreOutputTokens*4 {
		out.Summary = out.Summary[:defaultExploreOutputTokens*4]
	}
	return out
}
