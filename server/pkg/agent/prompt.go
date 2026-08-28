package agent

import "context"

// Prompt 包含组装模型调用所需的已准备数据。
type Prompt struct {
	Run     Run
	Turn    Turn
	Context ContextSnapshot
}

// Build 将上下文组装为模型调用。
// TODO: 将系统提示词、历史和工具定义组装为 Chat。
func Build(_ context.Context, _ Prompt) (Chat, error) {
	return Chat{}, nil
}
