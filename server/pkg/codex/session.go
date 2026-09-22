package codex

// Session 是一条只走 Codex 的对话。session_id 即官方 thread_id。
type Session struct {
	ID           string `json:"id"`
	ThreadID     string `json:"thread_id"` // 与 ID 相同，方便看板对照官方编号。
	Title        string `json:"title,omitempty"`
	Preview      string `json:"preview,omitempty"`
	Cwd          string `json:"cwd,omitempty"`
	ActiveTurnID string `json:"active_turn_id,omitempty"` // 同时只能有一个进行中的回合。
	Running      *bool  `json:"running,omitempty"`        // 列表计算：有进行中的回合且没有待审批。空表示没带。
	Archived     bool   `json:"archived"`
	Ephemeral    bool   `json:"ephemeral,omitempty"`
	CreatedAt    int64  `json:"created_at,omitempty"`
	UpdatedAt    int64  `json:"updated_at,omitempty"`
}

// SessionPage 是 thread/list 的一页。
type SessionPage struct {
	Sessions   []Session `json:"sessions"`
	NextCursor string    `json:"next_cursor,omitempty"`
}
