package memory

import (
	"strings"
	"time"
	"unicode/utf8"
)

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

const (
	// NameIndex 是目录条目的固定名称。
	NameIndex = "index"
	// IndexMaxLines 是目录行数上限。
	IndexMaxLines = 200
	// IndexMaxBytes 是目录字节上限（25KB）。
	IndexMaxBytes = 25 * 1024
	// DefaultSearchLimit 是冷层检索默认条数。
	DefaultSearchLimit = 10
)

// TextMemory 是一篇目录或专题 Markdown。
type TextMemory struct {
	ID         string
	Scope      TextMemoryScope
	ScopeID    string
	Kind       TextMemoryKind
	Name       string
	Content    string
	ByteLen    int
	OverBudget bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
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

// ByteLen 按 UTF-8 字节计算 Markdown 长度。
func ByteLen(content string) int {
	return len(content)
}

// IndexOverBudget 判断目录是否超过 200 行或 25KB（先到为准）。
func IndexOverBudget(content string) bool {
	return indexLineCount(content) > IndexMaxLines || len(content) > IndexMaxBytes
}

// ClipIndex 把目录裁到 200 行 / 25KB 以内，先按行再按字节。
func ClipIndex(content string) string {
	if content == "" {
		return ""
	}
	if indexLineCount(content) > IndexMaxLines {
		lines := strings.Split(content, "\n")
		content = strings.Join(lines[:IndexMaxLines], "\n")
	}
	if len(content) <= IndexMaxBytes {
		return content
	}
	clipped := content[:IndexMaxBytes]
	for !utf8.ValidString(clipped) && len(clipped) > 0 {
		clipped = clipped[:len(clipped)-1]
	}
	return clipped
}

// KindFromName 按名称推断 kind；空名称或 index 为目录。
func KindFromName(name string) TextMemoryKind {
	if name == "" || name == NameIndex {
		return KindIndex
	}
	return KindTopic
}

func indexLineCount(content string) int {
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}
