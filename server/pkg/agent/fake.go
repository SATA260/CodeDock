package agent

import (
	"context"
	"fmt"

	"codedock/pkg/agent/tool"
)

// streamFake 按 Model.Options 脚本产出确定性回复，供测试与离线闭环使用。
// 先按 Attempt 决定是否失败；再按历史助手消息数选取对应 Turn 的 text / tool_calls。
func streamFake(ctx context.Context, chat Chat) (ModelStream, error) {
	opts := ParseFakeOptions(chat.Model.Options)
	if opts.FailTimes > 0 && chat.Attempt <= opts.FailTimes {
		return nil, fmt.Errorf("fake model failed on attempt %d", chat.Attempt)
	}

	turn := opts.Turns[0]
	index := assistantTurns(chat.Messages)
	if index < len(opts.Turns) {
		turn = opts.Turns[index]
	} else if len(opts.Turns) > 0 {
		turn = opts.Turns[len(opts.Turns)-1]
	}

	calls := make([]tool.Call, 0, len(turn.ToolCalls))
	for i, item := range turn.ToolCalls {
		args := item.Arguments
		if len(args) == 0 {
			args = []byte("{}")
		}
		calls = append(calls, tool.Call{
			ID:        fmt.Sprintf("call_%s_%d", chat.TurnID, i+1),
			Name:      item.Name,
			Arguments: args,
			Attempt:   1,
		})
	}

	text := turn.Text
	usage := ProviderUsage{
		RequestID:    fmt.Sprintf("fake-%s-%d", chat.TurnID, chat.Attempt),
		Provider:     "fake",
		Model:        chat.Model.Model,
		OutputTokens: CountTokens(text),
		TotalTokens:  CountTokens(text) + CountTokens(chat.SystemPrompt),
		Estimated:    true,
	}
	return emitStream(ctx, chat, text, calls, usage, opts.Hang), nil
}

// assistantTurns 统计历史中的助手消息数，用来选取 fake 脚本的第几轮。
func assistantTurns(messages []Message) int {
	count := 0
	for _, msg := range messages {
		if msg.Role == RoleAssistant {
			count++
		}
	}
	return count
}
