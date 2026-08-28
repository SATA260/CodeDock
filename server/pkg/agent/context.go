package agent

import (
	"context"

	"codedock/pkg/agent/tool"
	einomodel "github.com/cloudwego/eino/components/model"
)

// History 是装载上下文所需的已准备数据。
type History struct {
	Run        Run
	Turn       Turn
	Checkpoint *CompactionCheckpoint
	Messages   []Message
	Tools      []tool.Definition
	Prompt     string
}

// Load 根据已准备数据构造上下文。
// TODO: 根据摘要和 checkpoint 之后的消息构造上下文。
func Load(_ context.Context, _ History) (ContextSnapshot, error) {
	return ContextSnapshot{}, nil
}

// Compaction 包含压缩判定所需的已准备数据。
type Compaction struct {
	Run      Run
	Turn     Turn
	Snapshot ContextSnapshot
}

// NeedsCompaction 判断上下文是否超过预算。
// TODO: 按 token 预算判断是否需要压缩。
func NeedsCompaction(_ context.Context, _ Compaction) (bool, error) {
	return false, nil
}

// CompactIfNeeded 使用 Eino ToolCallingChatModel 调用大模型生成摘要。
// TODO: 超预算时调用模型生成摘要并返回压缩后的上下文。
func CompactIfNeeded(_ context.Context, _ Compaction) (ContextSnapshot, error) {
	var chat einomodel.ToolCallingChatModel
	_ = chat
	return ContextSnapshot{}, nil
}
