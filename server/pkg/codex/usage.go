package codex

import "encoding/json"

// TokenUsage 是一条 thread 当前占了多少上下文。
type TokenUsage struct {
	Used   int64 `json:"used"`
	Window int64 `json:"window"`
}

// TokenUsageEvent 是官方用量通知。
type TokenUsageEvent struct {
	ThreadID string
	TurnID   string
	Usage    TokenUsage
}

// Remaining 返回还剩多少上下文；窗口未知时返回 0, false。
func (u TokenUsage) Remaining() (left int64, ok bool) {
	if u.Window <= 0 {
		return 0, false
	}
	left = u.Window - u.Used
	if left < 0 {
		left = 0
	}
	return left, true
}

// RemainingPercent 返回剩余百分比。
func (u TokenUsage) RemainingPercent() (int, bool) {
	left, ok := u.Remaining()
	if !ok {
		return 0, false
	}
	return int((left*100 + u.Window/2) / u.Window), true
}

// ParseTokenUsage 读官方 thread/tokenUsage/updated 或旧版 token_count。
func ParseTokenUsage(msg Message) (TokenUsageEvent, bool) {
	if msg.Kind != KindNotification {
		return TokenUsageEvent{}, false
	}
	switch msg.Method {
	case MethodThreadTokenUsageUpdated:
		return parseTokenUsageV2(msg.Params)
	case MethodTokenCount:
		return parseTokenUsageV1(msg.Params)
	default:
		return TokenUsageEvent{}, false
	}
}

func parseTokenUsageV2(raw json.RawMessage) (TokenUsageEvent, bool) {
	var params struct {
		ThreadID   string `json:"threadId"`
		TurnID     string `json:"turnId"`
		TokenUsage struct {
			ModelContextWindow int64 `json:"modelContextWindow"`
			Last               struct {
				TotalTokens int64 `json:"totalTokens"`
			} `json:"last"`
			Total struct {
				TotalTokens int64 `json:"totalTokens"`
			} `json:"total"`
		} `json:"tokenUsage"`
	}
	if err := json.Unmarshal(raw, &params); err != nil || params.ThreadID == "" {
		return TokenUsageEvent{}, false
	}
	used := pickUsed(params.TokenUsage.Last.TotalTokens, params.TokenUsage.Total.TotalTokens)
	window := params.TokenUsage.ModelContextWindow
	return TokenUsageEvent{
		ThreadID: params.ThreadID,
		TurnID:   params.TurnID,
		Usage:    TokenUsage{Used: used, Window: window},
	}, window > 0 || used > 0
}

func parseTokenUsageV1(raw json.RawMessage) (TokenUsageEvent, bool) {
	var params struct {
		ConversationID string `json:"conversationId"`
		ThreadID       string `json:"threadId"`
		Msg            struct {
			Info struct {
				ModelContextWindow int64 `json:"model_context_window"`
				LastTokenUsage     struct {
					TotalTokens int64 `json:"total_tokens"`
				} `json:"last_token_usage"`
				TotalTokenUsage struct {
					TotalTokens int64 `json:"total_tokens"`
				} `json:"total_token_usage"`
			} `json:"info"`
		} `json:"msg"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return TokenUsageEvent{}, false
	}
	threadID := firstNonEmpty(params.ThreadID, params.ConversationID)
	if threadID == "" {
		return TokenUsageEvent{}, false
	}
	used := pickUsed(params.Msg.Info.LastTokenUsage.TotalTokens, params.Msg.Info.TotalTokenUsage.TotalTokens)
	window := params.Msg.Info.ModelContextWindow
	return TokenUsageEvent{
		ThreadID: threadID,
		Usage:    TokenUsage{Used: used, Window: window},
	}, window > 0 || used > 0
}

func pickUsed(last, total int64) int64 {
	if last > 0 {
		return last
	}
	return total
}
