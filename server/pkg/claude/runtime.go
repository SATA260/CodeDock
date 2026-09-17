package claude

import (
	"context"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
)

type memSession struct {
	ID              string
	ClaudeSessionID string
	Title           string
	Archived        bool
	ActiveTurnID    string
	CreatedAt       int64 // 本机实录或创建时刻，Unix 秒。
	UpdatedAt       int64 // 本机实录最近一条或上次写入，Unix 秒。
	Overrides       Settings
	Overridden      []string
	Draft           Input
	Queue           []queuedTurn
}

type queuedTurn struct {
	Content string
	Input   Input
}

type memTurn struct {
	ID              string
	SessionID       string
	ClaudeSessionID string
	Status          TurnStatus
	cmd             *exec.Cmd
	stdin           io.WriteCloser
	cancel          context.CancelFunc
	waiters         map[string]chan AskAnswer
}

type memAsk struct {
	ID     string
	TurnID string
	Ask    ApprovalAsk
}

type runtime struct {
	mu       sync.Mutex
	sessions map[string]*memSession
	turns    map[string]*memTurn
	asks     map[string]*memAsk
}

var rt = newRuntime()

func newRuntime() *runtime {
	return &runtime{
		sessions: map[string]*memSession{},
		turns:    map[string]*memTurn{},
		asks:     map[string]*memAsk{},
	}
}

func resetRuntime() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for _, turn := range rt.turns {
		if turn.cancel != nil {
			turn.cancel()
		}
		if turn.cmd != nil && turn.cmd.Process != nil {
			_ = turn.cmd.Process.Kill()
		}
		if turn.stdin != nil {
			_ = turn.stdin.Close()
		}
	}
	rt.sessions = map[string]*memSession{}
	rt.turns = map[string]*memTurn{}
	rt.asks = map[string]*memAsk{}
}

func internLocked(id string) *memSession {
	if id == "" {
		id = uuid.NewString()
	}
	if sess, ok := rt.sessions[id]; ok {
		return sess
	}
	for _, sess := range rt.sessions {
		if sess.ClaudeSessionID == id {
			return sess
		}
	}
	now := time.Now().Unix()
	sess := &memSession{
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
		Draft:     Input{Mentions: []string{}, Images: []string{}},
	}
	rt.sessions[id] = sess
	return sess
}

func (s *memSession) snapshot() Session {
	return Session{
		ID:              s.ID,
		ClaudeSessionID: s.ClaudeSessionID,
		Title:           s.Title,
		ActiveTurnID:    s.ActiveTurnID,
		Archived:        s.Archived,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
	}
}

func sessionByClaudeID(claudeSessionID string) *memSession {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if claudeSessionID == "" {
		return nil
	}
	for _, sess := range rt.sessions {
		if sess.ClaudeSessionID == claudeSessionID {
			return sess
		}
	}
	return internLocked(claudeSessionID)
}
