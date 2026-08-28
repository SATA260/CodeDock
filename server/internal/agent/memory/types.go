package memory

import "time"

// TextMemoryScope 标识 Markdown 记忆的归属范围。
type TextMemoryScope string

const (
	ScopeUser    TextMemoryScope = "user"    // ScopeID 为 user_id。
	ScopeProject TextMemoryScope = "project" // ScopeID 为 project_id。
)

// TextMemory 是一段按 scope 存储的 Markdown 文档。
type TextMemory struct {
	ID        string
	Scope     TextMemoryScope
	ScopeID   string
	Content   string
	ByteLen   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TextMemoryKey 定位一条 Markdown 记忆。
type TextMemoryKey struct {
	Scope   TextMemoryScope
	ScopeID string
}

// ContextMessage 是供检索使用的上下文 message 索引条目。
type ContextMessage struct {
	ID        string
	ProjectID string
	GroupID   string
	SessionID string
	RunID     string
	Role      string
	Content   string
	CreatedAt time.Time
}

// Search 是检索已索引 context message 的条件。
type Search struct {
	ProjectID string
	GroupID   string
	SessionID string
	Query     string
	Limit     int
}

// MessageHit 是一次检索命中。
type MessageHit struct {
	Message ContextMessage
	Rank    float64
}

// ByteLen 计算 Markdown 字节长度。
// TODO: 按 UTF-8 字节计算 Content 长度。
func ByteLen(_ string) int {
	return 0
}
