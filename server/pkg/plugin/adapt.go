package plugin

import (
	"context"
	"encoding/json"

	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/seam"
	"codedock/pkg/agent/tool"
)

// runtime 把作者的 Plugin 接到宿主用的信封 OnEvent。
type runtime struct {
	Plugin
}

// OnEvent 按信封类型拆成各口结构体，再调作者实现。
func (r *runtime) OnEvent(ctx context.Context, ev seam.Envelope) (seam.Envelope, error) {
	return applyHooks(ctx, r.Plugin, ev)
}

// applyHooks 按 Type 分发给对应口；未实现该口则原样返回。
func applyHooks(ctx context.Context, p Plugin, ev seam.Envelope) (seam.Envelope, error) {
	switch ev.Type {
	case TypeInput:
		return applyInput(ctx, p, ev)
	case TypePreStep:
		return applyPreStep(ctx, p, ev)
	case TypeRequest:
		return applyRequest(ctx, p, ev)
	case TypeStream:
		return applyStream(ctx, p, ev)
	case TypePreExecute:
		return applyPreExecute(ctx, p, ev)
	case TypePostExecute:
		return applyPostExecute(ctx, p, ev)
	default:
		if h, ok := p.(LedgerNotifyHandler); ok {
			return ev, h.OnLedgerNotify(ctx, LedgerNotify{
				Type:      ev.Type,
				SessionID: ev.SessionID,
				RunID:     ev.RunID,
				TurnID:    ev.TurnID,
				Payload:   ev.Payload,
				Context:   DecodePluginContext(ev.Context),
			})
		}
		return ev, nil
	}
}

// applyInput 把信封转成 AgentInput / AgentInputResult。
func applyInput(ctx context.Context, p Plugin, ev seam.Envelope) (seam.Envelope, error) {
	h, ok := p.(AgentInputHandler)
	if !ok {
		return ev, nil
	}
	var payload pkgagent.InputPayload
	_ = json.Unmarshal(ev.Payload, &payload)
	out, err := h.OnAgentInput(ctx, AgentInput{
		SessionID: ev.SessionID,
		RunID:     ev.RunID,
		TurnID:    ev.TurnID,
		Content:   payload.Content,
		Mode:      payload.Mode,
		Context:   DecodePluginContext(ev.Context),
	})
	if err != nil {
		return ev, err
	}
	ev.Payload = mustPayload(pkgagent.InputPayload{Content: out.Content, Mode: out.Mode})
	ev.Context = out.Context.Raw()
	if out.Handled {
		ev.Type = TypeInputHandled
	}
	return ev, nil
}

// applyPreStep 把信封转成 AgentPreStep / AgentPreStepResult。
func applyPreStep(ctx context.Context, p Plugin, ev seam.Envelope) (seam.Envelope, error) {
	h, ok := p.(AgentPreStepHandler)
	if !ok {
		return ev, nil
	}
	var payload pkgagent.PreStepPayload
	_ = json.Unmarshal(ev.Payload, &payload)
	out, err := h.OnAgentPreStep(ctx, AgentPreStep{
		SessionID:    ev.SessionID,
		RunID:        ev.RunID,
		TurnID:       ev.TurnID,
		SystemPrompt: payload.SystemPrompt,
		Hidden:       payload.Hidden,
		Context:      DecodePluginContext(ev.Context),
	})
	if err != nil {
		return ev, err
	}
	ev.Payload = mustPayload(pkgagent.PreStepPayload{SystemPrompt: out.SystemPrompt, Hidden: out.Hidden})
	ev.Context = out.Context.Raw()
	if out.Blocked {
		ev.Type = TypeRunBlocked
	}
	return ev, nil
}

// applyRequest 把信封转成 AgentRequest / AgentRequestResult。
func applyRequest(ctx context.Context, p Plugin, ev seam.Envelope) (seam.Envelope, error) {
	h, ok := p.(AgentRequestHandler)
	if !ok {
		return ev, nil
	}
	var payload pkgagent.RequestPayload
	_ = json.Unmarshal(ev.Payload, &payload)
	out, err := h.OnAgentRequest(ctx, AgentRequest{
		SessionID:    ev.SessionID,
		RunID:        ev.RunID,
		TurnID:       ev.TurnID,
		SystemPrompt: payload.SystemPrompt,
		Messages:     payload.Messages,
		Context:      DecodePluginContext(ev.Context),
	})
	if err != nil {
		return ev, err
	}
	ev.Payload = mustPayload(pkgagent.RequestPayload{SystemPrompt: out.SystemPrompt, Messages: out.Messages})
	ev.Context = out.Context.Raw()
	return ev, nil
}

// applyStream 把信封转成 LLMStream / LLMStreamResult。
func applyStream(ctx context.Context, p Plugin, ev seam.Envelope) (seam.Envelope, error) {
	h, ok := p.(LLMStreamHandler)
	if !ok {
		return ev, nil
	}
	var payload pkgagent.StreamPayload
	_ = json.Unmarshal(ev.Payload, &payload)
	out, err := h.OnLLMStream(ctx, LLMStream{
		SessionID: ev.SessionID,
		RunID:     ev.RunID,
		TurnID:    ev.TurnID,
		Headers:   payload.Headers,
		Body:      payload.Body,
		Context:   DecodePluginContext(ev.Context),
	})
	if err != nil {
		return ev, err
	}
	ev.Payload = mustPayload(pkgagent.StreamPayload{Headers: out.Headers, Body: out.Body})
	ev.Context = out.Context.Raw()
	return ev, nil
}

// applyPreExecute 把信封转成 ToolPreExecute / ToolPreExecuteResult。
func applyPreExecute(ctx context.Context, p Plugin, ev seam.Envelope) (seam.Envelope, error) {
	h, ok := p.(ToolPreExecuteHandler)
	if !ok {
		return ev, nil
	}
	var payload tool.PreExecutePayload
	_ = json.Unmarshal(ev.Payload, &payload)
	out, err := h.OnToolPreExecute(ctx, ToolPreExecute{
		SessionID: ev.SessionID,
		RunID:     ev.RunID,
		TurnID:    ev.TurnID,
		Call:      payload.Call,
		Context:   DecodePluginContext(ev.Context),
	})
	if err != nil {
		return ev, err
	}
	ev.Payload = mustPayload(tool.PreExecutePayload{Call: out.Call})
	ev.Context = out.Context.Raw()
	if out.Denied {
		ev.Type = TypeToolsDenied
	} else if out.Ask {
		ev.Type = TypeToolsAsk
	}
	return ev, nil
}

// applyPostExecute 把信封转成 ToolPostExecute / ToolPostExecuteResult。
func applyPostExecute(ctx context.Context, p Plugin, ev seam.Envelope) (seam.Envelope, error) {
	h, ok := p.(ToolPostExecuteHandler)
	if !ok {
		return ev, nil
	}
	var payload tool.PostExecutePayload
	_ = json.Unmarshal(ev.Payload, &payload)
	out, err := h.OnToolPostExecute(ctx, ToolPostExecute{
		SessionID: ev.SessionID,
		RunID:     ev.RunID,
		TurnID:    ev.TurnID,
		Call:      payload.Call,
		Result:    payload.Result,
		Context:   DecodePluginContext(ev.Context),
	})
	if err != nil {
		return ev, err
	}
	ev.Payload = mustPayload(tool.PostExecutePayload{Call: out.Call, Result: out.Result})
	ev.Context = out.Context.Raw()
	return ev, nil
}

// mustPayload 把结构体编成信封载荷；失败时退回空对象。
func mustPayload(v any) json.RawMessage {
	if raw, ok := v.(json.RawMessage); ok {
		return raw
	}
	body, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return body
}
