package board

import (
	"encoding/json"
	"strings"

	cderr "codedock/internal/errors"
)

// Engine 是会话归属引擎在库内的名字。
type Engine string

const (
	EngineNative Engine = "native" // 自有 Agent；HTTP / 前端写成 agent。
	EngineClaude Engine = "claude"
	EngineCodex  Engine = "codex"
)

// CheckoutKind 是挂到 Work 上的目录种类。
type CheckoutKind string

const (
	CheckoutFolder   CheckoutKind = "folder"
	CheckoutPrimary  CheckoutKind = "primary"
	CheckoutWorktree CheckoutKind = "worktree"
)

// Work 是一张看板卡，可挂多目录、多路会话。
type Work struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Info 是 Work 级或目录级说明；Checkout 空表示卡片本身。
type Info struct {
	WorkID    string `json:"work_id"`
	Checkout  string `json:"checkout"`
	Body      string `json:"body"`
	UpdatedAt string `json:"updated_at"`
}

// Checkout 是挂在一张卡上的目录。
type Checkout struct {
	WorkID string       `json:"work_id"`
	Path   string       `json:"path"`
	Kind   CheckoutKind `json:"kind"`
}

// Placement 把一路会话挂到一张卡；Checkout 空表示问答。
type Placement struct {
	Engine    Engine `json:"engine"`
	SessionID string `json:"session_id"`
	WorkID    string `json:"work_id"`
	Checkout  string `json:"checkout"`
}

// IssueSnap 是会话挂上的 Issue 快照，来自本机 gh。
type IssueSnap struct {
	Engine    Engine `json:"engine"`
	SessionID string `json:"session_id"`
	Repo      string `json:"repo"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	URL       string `json:"url"`
	State     string `json:"state"`
	UpdatedAt string `json:"updated_at"`
}

// PullSnap 是会话挂上的一条 PR 快照。
type PullSnap struct {
	Engine    Engine `json:"engine"`
	SessionID string `json:"session_id"`
	Repo      string `json:"repo"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	URL       string `json:"url"`
	State     string `json:"state"`
	UpdatedAt string `json:"updated_at"`
}

// SessionLinks 是一路会话头上的 Issue/PR。
type SessionLinks struct {
	Issue *IssueSnap `json:"issue,omitempty"`
	Pulls []PullSnap `json:"pulls"`
}

// SessionView 是看板/侧栏用的会话摘要，不带对话正文。
type SessionView struct {
	Engine    string `json:"engine"` // 对外：agent / claude / codex
	SessionID string `json:"session_id"`
	Summary   string `json:"summary"`
	Checkout  string `json:"checkout"`
	Running   bool   `json:"running"`
	Pending   int    `json:"pending"`
	UpdatedAt string `json:"updated_at"`
}

// DirView 是目录现问 Git 的状态。
type DirView struct {
	Path          string   `json:"path"`
	Kind          string   `json:"kind"`
	Info          Info     `json:"info"`
	Branch        string   `json:"branch"`
	Dirty         bool     `json:"dirty"`
	SharedWriters []string `json:"shared_writers"`
}

// Card 是一张 Work 在看板上的聚合。
type Card struct {
	Work     Work          `json:"work"`
	Info     Info          `json:"info"`
	Dirs     []DirView     `json:"dirs"`
	Sessions []SessionView `json:"sessions"`
	Running  int           `json:"running"`
	Pending  int           `json:"pending"`
}

// BoardView 是横向看板：已归组的卡 + 未归组列。
type BoardView struct {
	Cards     []Card        `json:"cards"`
	Ungrouped []SessionView `json:"ungrouped"`
}

// InboxItem 是一张卡下的一条待批；payload 保持各引擎原结构。
type InboxItem struct {
	Engine    string          `json:"engine"`
	SessionID string          `json:"session_id"`
	TicketID  string          `json:"ticket_id"`
	Summary   string          `json:"summary"`
	Payload   json.RawMessage `json:"payload"`
}

// Packet 是开回合时装给模型的只读前缀材料。
type Packet struct {
	WorkInfo Info       `json:"work_info"`
	DirInfo  *Info      `json:"dir_info,omitempty"`
	Issue    *IssueSnap `json:"issue,omitempty"`
	Pulls    []PullSnap `json:"pulls"`
	Text     string     `json:"text"`
}

// StartSpec 描述从某张卡开一路新会话。
type StartSpec struct {
	Engine      Engine
	UserID      string
	TenantID    string
	AgentID     string
	WorkspaceID string // StartInDir 时为目录；问答可空。
	Talk        bool
}

// Started 是新建会话后回给 Placement 的身份。
type Started struct {
	ID        string
	Summary   string
	UpdatedAt string
}

// SessionMeta 是三引擎会话的看板摘要。
type SessionMeta struct {
	ID        string
	Summary   string
	UpdatedAt string
	Running   bool
	Pending   int
	Archived  bool
}

// DecideAnswer 把 Inbox 裁决转给已有引擎，不统一三套问票。
type DecideAnswer struct {
	Native *NativeDecision `json:"native,omitempty"`
	Claude *AskDecision    `json:"claude,omitempty"`
	Codex  *AskDecision    `json:"codex,omitempty"`
}

// NativeDecision 是自有 Agent 的审批提交。
type NativeDecision struct {
	Decisions []ToolDecision `json:"decisions"`
	Status    string         `json:"status"`
	Scope     string         `json:"scope"`
	ActorID   string         `json:"actor_id"`
	Reason    string         `json:"reason"`
	Override  string         `json:"override,omitempty"`
}

// ToolDecision 是一条工具的批/拒。
type ToolDecision struct {
	ToolCallID string `json:"tool_call_id"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
}

// AskDecision 是 Claude / Codex 原问票作答。
type AskDecision struct {
	Approved bool              `json:"approved"`
	Scope    string            `json:"scope"`
	Choice   string            `json:"choice"`
	Values   []string          `json:"values"`
	Answers  map[string]string `json:"answers,omitempty"`
}

// ParseEngine 把 HTTP / 前端引擎名收到库内值；agent 与 native 同义。
func ParseEngine(raw string) (Engine, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "agent", "native":
		return EngineNative, nil
	case "claude":
		return EngineClaude, nil
	case "codex":
		return EngineCodex, nil
	default:
		return "", cderr.Invalid("unknown engine %q", raw)
	}
}

// PublicEngine 把库内引擎名写成前端用的 agent / claude / codex。
func PublicEngine(engine Engine) string {
	if engine == EngineNative {
		return "agent"
	}
	return string(engine)
}

// ParseCheckoutKind 校验目录种类。
func ParseCheckoutKind(raw string) (CheckoutKind, error) {
	switch CheckoutKind(strings.TrimSpace(raw)) {
	case CheckoutFolder, CheckoutPrimary, CheckoutWorktree:
		return CheckoutKind(strings.TrimSpace(raw)), nil
	case "":
		return CheckoutFolder, nil
	default:
		return "", cderr.Invalid("unknown checkout kind %q", raw)
	}
}
