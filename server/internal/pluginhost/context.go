package pluginhost

import (
	"encoding/json"

	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/seam"
	sdk "codedock/pkg/plugin"
)

// attachPluginContext 把已存的袋子填进信封；信封上已有合法键则盖在已存之上。
func (h *Host) attachPluginContext(ev seam.Envelope) seam.Envelope {
	if h == nil {
		return ev
	}
	stored := h.lookupPluginContext(ev.SessionID, ev.RunID)
	if len(ev.Context) > 0 && sdk.AcceptableContext(ev.Context) {
		bag := sdk.DecodePluginContext(stored)
		incoming := sdk.DecodePluginContext(ev.Context)
		for key, value := range incoming.Values {
			bag.Values[key] = value
		}
		ev.Context = bag.Raw()
		return ev
	}
	if len(stored) > 0 {
		ev.Context = append(json.RawMessage(nil), stored...)
		return ev
	}
	if len(ev.Context) == 0 {
		ev.Context = json.RawMessage(`{}`)
	}
	return ev
}

// persistPluginContext 按 Run（没有则按会话）保存袋子；超限或非法则丢掉这次写入。
func (h *Host) persistPluginContext(ev seam.Envelope) {
	if h == nil {
		return
	}
	raw := ev.Context
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if !sdk.AcceptableContext(raw) {
		if h.log != nil {
			h.log.Warn("plugin context not persisted", "bytes", len(raw), "session_id", ev.SessionID, "run_id", ev.RunID)
		}
		return
	}
	key := pluginContextKey(ev.SessionID, ev.RunID)
	if key == "" {
		return
	}
	h.ctxMu.Lock()
	defer h.ctxMu.Unlock()
	if h.contexts == nil {
		h.contexts = map[string]json.RawMessage{}
	}
	h.contexts[key] = append(json.RawMessage(nil), raw...)
	if ev.RunID != "" && ev.SessionID != "" {
		delete(h.contexts, sessionPluginContextKey(ev.SessionID))
	}
}

// lookupPluginContext 取出已存袋子；有 Run 但还没有对应条目时，把会话袋迁过去。
func (h *Host) lookupPluginContext(sessionID, runID string) json.RawMessage {
	if h == nil {
		return nil
	}
	h.ctxMu.Lock()
	defer h.ctxMu.Unlock()
	if h.contexts == nil {
		return nil
	}
	if runID != "" {
		if raw, ok := h.contexts[runPluginContextKey(runID)]; ok {
			return append(json.RawMessage(nil), raw...)
		}
		if sessionID != "" {
			if raw, ok := h.contexts[sessionPluginContextKey(sessionID)]; ok {
				copied := append(json.RawMessage(nil), raw...)
				h.contexts[runPluginContextKey(runID)] = copied
				delete(h.contexts, sessionPluginContextKey(sessionID))
				return append(json.RawMessage(nil), copied...)
			}
		}
		return nil
	}
	if sessionID == "" {
		return nil
	}
	raw, ok := h.contexts[sessionPluginContextKey(sessionID)]
	if !ok {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

// dropSessionPluginContext 丢掉还没挂到 Run 的会话袋。
func (h *Host) dropSessionPluginContext(sessionID string) {
	if h == nil || sessionID == "" {
		return
	}
	h.ctxMu.Lock()
	defer h.ctxMu.Unlock()
	if h.contexts != nil {
		delete(h.contexts, sessionPluginContextKey(sessionID))
	}
}

// dropPluginContext 丢掉该 Run 和该会话上的袋子。
func (h *Host) dropPluginContext(runID, sessionID string) {
	if h == nil {
		return
	}
	h.ctxMu.Lock()
	defer h.ctxMu.Unlock()
	if h.contexts == nil {
		return
	}
	if runID != "" {
		delete(h.contexts, runPluginContextKey(runID))
	}
	if sessionID != "" {
		delete(h.contexts, sessionPluginContextKey(sessionID))
	}
}

// pluginContextKey 优先用 Run，否则用会话。
func pluginContextKey(sessionID, runID string) string {
	if runID != "" {
		return runPluginContextKey(runID)
	}
	if sessionID != "" {
		return sessionPluginContextKey(sessionID)
	}
	return ""
}

// sessionPluginContextKey 是建 Run 之前的袋子键。
func sessionPluginContextKey(sessionID string) string {
	return "session:" + sessionID
}

// runPluginContextKey 是本轮 Run 的袋子键。
func runPluginContextKey(runID string) string {
	return "run:" + runID
}

// isTerminalLedger 判断账本事件是否表示 Run 已结束。
func isTerminalLedger(typ string) bool {
	switch typ {
	case string(pkgagent.EventRunCompleted), string(pkgagent.EventRunFailed), string(pkgagent.EventRunCancelled):
		return true
	default:
		return false
	}
}
