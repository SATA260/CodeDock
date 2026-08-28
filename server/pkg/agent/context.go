package agent

import (
	"context"
	"strings"

	"codedock/pkg/agent/tool"
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
func Load(_ context.Context, hist History) (ContextSnapshot, error) {
	snapshot := ContextSnapshot{
		SessionID:    hist.Run.SessionID,
		Messages:     hist.Messages,
		Tools:        hist.Tools,
		SystemPrompt: hist.Prompt,
	}
	if hist.Checkpoint != nil {
		snapshot.BaseEventSeq = hist.Checkpoint.BaseEventSeq
		snapshot.Summary = &CompactionSummary{
			CheckpointID: hist.Checkpoint.ID,
			Content:      hist.Checkpoint.Summary,
			BaseEventSeq: hist.Checkpoint.BaseEventSeq,
		}
	}
	snapshot.EstimatedTokens = EstimateTokens(snapshot)
	return snapshot, nil
}

// Compaction 包含压缩判定所需的已准备数据。
type Compaction struct {
	Run      Run
	Turn     Turn
	Snapshot ContextSnapshot
}

// NeedsCompaction 判断上下文是否超过预算。
func NeedsCompaction(_ context.Context, req Compaction) (bool, error) {
	limit := req.Run.Config.Limits.MaxInputTokens
	if limit <= 0 {
		return false, nil
	}
	tokens := req.Snapshot.EstimatedTokens
	if tokens == 0 {
		tokens = EstimateTokens(req.Snapshot)
	}
	return tokens > limit, nil
}

// CompactIfNeeded 超预算时生成摘要并返回压缩后的上下文。
// 未超预算原样返回；压缩后清空 Messages，只保留 Summary 与新的 BaseEventSeq。
func CompactIfNeeded(ctx context.Context, req Compaction) (ContextSnapshot, error) {
	needed, err := NeedsCompaction(ctx, req)
	if err != nil || !needed {
		return req.Snapshot, err
	}

	summary := compactSummary(ctx, req)
	compacted := req.Snapshot
	compacted.Summary = &CompactionSummary{
		Content:      summary,
		BaseEventSeq: lastMessageSeq(req.Snapshot.Messages),
	}
	compacted.BaseEventSeq = compacted.Summary.BaseEventSeq
	compacted.Messages = nil
	compacted.EstimatedTokens = EstimateTokens(compacted)
	return compacted, nil
}

// EstimateTokens 估算快照的 token 数。
func EstimateTokens(snapshot ContextSnapshot) int64 {
	total := CountTokens(snapshot.SystemPrompt)
	if snapshot.Summary != nil {
		total += CountTokens(snapshot.Summary.Content)
	}
	for _, msg := range snapshot.Messages {
		total += CountTokens(DecodeText(msg.Content))
	}
	for _, def := range snapshot.Tools {
		total += CountTokens(def.Name) + CountTokens(def.Prompt)
	}
	return total
}

// lastMessageSeq 返回消息列表中最大的 event_seq，用作压缩 checkpoint 的起点。
func lastMessageSeq(messages []Message) int64 {
	var seq int64
	for _, msg := range messages {
		if msg.EventSeq > seq {
			seq = msg.EventSeq
		}
	}
	return seq
}

// compactSummary 生成压缩摘要：openai 走模型，否则用脚本或拼接历史。
func compactSummary(ctx context.Context, req Compaction) string {
	opts := ParseFakeOptions(req.Run.Config.Model.Options)
	if strings.EqualFold(req.Run.Config.Model.Provider, "openai") {
		if summary, err := compactWithModel(ctx, req); err == nil && summary != "" {
			return summary
		}
	}
	if opts.CompactSummary != "" {
		return opts.CompactSummary
	}
	var b strings.Builder
	b.WriteString("Summary of earlier conversation:\n")
	for _, msg := range req.Snapshot.Messages {
		text := DecodeText(msg.Content)
		if text == "" {
			continue
		}
		b.WriteString(string(msg.Role))
		b.WriteString(": ")
		b.WriteString(text)
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		return "Earlier conversation was compacted."
	}
	return strings.TrimSpace(b.String())
}

// compactWithModel 用当前 ModelConfig 再调一次模型，把对话压成纯文本摘要。
func compactWithModel(ctx context.Context, req Compaction) (string, error) {
	chat := Chat{
		SessionID:    req.Snapshot.SessionID,
		RunID:        req.Run.ID,
		TurnID:       req.Turn.ID,
		Model:        req.Run.Config.Model,
		SystemPrompt: "Summarize the conversation so later turns can continue. Reply with plain text only.",
		Messages:     req.Snapshot.Messages,
		Attempt:      1,
	}
	stream, err := Stream(ctx, chat)
	if err != nil {
		return "", err
	}
	defer stream.Close()
	result, err := stream.Result(ctx)
	if err != nil {
		return "", err
	}
	return DecodeText(result.Message.Content), nil
}
