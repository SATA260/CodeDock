package agent

import "context"

// Prompt 包含组装模型调用所需的已准备数据。
type Prompt struct {
	Run     Run
	Turn    Turn
	Context ContextSnapshot
}

// Build 将上下文组装为模型调用。
func Build(_ context.Context, req Prompt) (Chat, error) {
	system := req.Context.SystemPrompt
	if system == "" {
		system = req.Run.Config.Profile.Prompt.Inline
	}
	if system == "" {
		system = DefaultSystemPrompt
	}
	var prefix []Message
	for _, index := range req.Context.MemoryIndexes {
		if index == "" {
			continue
		}
		prefix = append(prefix, Message{Role: RoleSystem, Content: EncodeText(index)})
	}
	if req.Context.Summary != nil && req.Context.Summary.Content != "" {
		prefix = append(prefix, Message{
			Role:    RoleSystem,
			Content: EncodeText("Conversation summary:\n" + req.Context.Summary.Content),
		})
	}
	messages := append(prefix, req.Context.Messages...)
	return Chat{
		SessionID:       req.Context.SessionID,
		RunID:           req.Run.ID,
		TurnID:          req.Turn.ID,
		Model:           req.Run.Config.Model,
		SystemPrompt:    system,
		Messages:        messages,
		Tools:           req.Context.Tools,
		MaxInputTokens:  req.Run.Config.Limits.MaxInputTokens,
		MaxOutputTokens: req.Run.Config.Limits.MaxOutputTokens,
		Attempt:         1,
	}, nil
}
