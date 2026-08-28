package memory

import "time"

// TextMemoryScope 标识 Markdown 记忆的归属范围。
type TextMemoryScope string

const (
	ScopeUser      TextMemoryScope = "user"      // ScopeID 为 user_id。
	ScopeWorkspace TextMemoryScope = "workspace" // ScopeID 为 workspace_id。
)

// TextMemoryKind 区分目录与专题。
type TextMemoryKind string

const (
	KindIndex TextMemoryKind = "index" // 目录，Name 固定为 NameIndex。
	KindTopic TextMemoryKind = "topic" // 专题，Name 为文件名。
)

// NameIndex 是目录条目的固定名称。
const NameIndex = "index"

// TextMemory 是一篇目录或专题 Markdown。
type TextMemory struct {
	ID        string
	Scope     TextMemoryScope
	ScopeID   string
	Kind      TextMemoryKind
	Name      string
	Content   string
	ByteLen   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TextMemoryKey 定位一篇目录或专题。
type TextMemoryKey struct {
	Scope   TextMemoryScope
	ScopeID string
	Kind    TextMemoryKind
	Name    string
}

// ContextMessage 是供检索使用的上下文 message 索引条目。
type ContextMessage struct {
	ID          string
	WorkspaceID string
	SessionID   string
	RunID       string
	Role        string
	Content     string
	CreatedAt   time.Time
}

// Search 是检索已索引 context message 的条件。
type Search struct {
	WorkspaceID string
	Query       string
	Limit       int
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

// IndexOverBudget 判断目录是否超过 200 行或 25KB。
// TODO: 按 200 行 / 25KB 先到为准判定目录是否超限。
func IndexOverBudget(_ string) bool {
	return false
}
