package plugin

import (
	"context"
	"encoding/json"
	"testing"

	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/seam"
	"codedock/pkg/agent/tool"
)

type stubPlugin struct{}

// Bootstrap 返回空订阅的占位清单。
func (stubPlugin) Bootstrap(context.Context, Host) (Manifest, error) {
	return Manifest{Name: "stub"}, nil
}

// ExecuteMethod 声明占位插件没有方法。
func (stubPlugin) ExecuteMethod(context.Context, MethodInput) (MethodResult, error) {
	return MethodResult{Success: false, Error: "no methods"}, nil
}

type rewritePlugin struct{ stubPlugin }

// OnAgentInput 给正文加方括号；内容为 skip 则不建 Run。
func (rewritePlugin) OnAgentInput(_ context.Context, in AgentInput) (AgentInputResult, error) {
	if in.Content == "skip" {
		return in.Handle(), nil
	}
	in.Content = "[" + in.Content + "]"
	in.Context.Set("rewrite.seen", true)
	return in.Reply(), nil
}

// OnAgentPreStep 把系统提示改成 new；stop 则取消本轮。
func (rewritePlugin) OnAgentPreStep(_ context.Context, in AgentPreStep) (AgentPreStepResult, error) {
	if in.SystemPrompt == "stop" {
		return in.Block(), nil
	}
	in.SystemPrompt = "new"
	return in.Reply(), nil
}

// OnAgentRequest 把系统提示改成 req。
func (rewritePlugin) OnAgentRequest(_ context.Context, in AgentRequest) (AgentRequestResult, error) {
	in.SystemPrompt = "req"
	return in.Reply(), nil
}

// OnLLMStream 加上 X=1 请求头。
func (rewritePlugin) OnLLMStream(_ context.Context, in LLMStream) (LLMStreamResult, error) {
	if in.Headers == nil {
		in.Headers = map[string]string{}
	}
	in.Headers["X"] = "1"
	return in.Reply(), nil
}

// OnToolPreExecute 按工具名否决、送审或改参。
func (rewritePlugin) OnToolPreExecute(_ context.Context, in ToolPreExecute) (ToolPreExecuteResult, error) {
	switch in.Call.Name {
	case "deny":
		return in.Deny(), nil
	case "ask":
		return in.AskApproval(), nil
	}
	in.Call.Arguments = json.RawMessage(`{"x":2}`)
	return in.Reply(), nil
}

// OnToolPostExecute 把工具输出改成 ok。
func (rewritePlugin) OnToolPostExecute(_ context.Context, in ToolPostExecute) (ToolPostExecuteResult, error) {
	in.Result.Output = json.RawMessage(`{"ok":true}`)
	return in.Reply(), nil
}

// TestApplyHooksIdentityWithoutHandler 确认未实现的口原样通过。
func TestApplyHooksIdentityWithoutHandler(t *testing.T) {
	t.Parallel()
	ev := seam.Envelope{Type: TypeInput, Payload: json.RawMessage(`{"content":"hi"}`)}
	got, err := applyHooks(context.Background(), stubPlugin{}, ev)
	if err != nil || got.Type != TypeInput || string(got.Payload) != `{"content":"hi"}` {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

// TestApplyHooksTypedResults 覆盖六个口的改写和换向。
func TestApplyHooksTypedResults(t *testing.T) {
	t.Parallel()
	p := rewritePlugin{}

	input, err := applyHooks(context.Background(), p, seam.Envelope{
		Type:      TypeInput,
		SessionID: "s1",
		Payload:   pkgagent.MarshalPayload(pkgagent.InputPayload{Content: "hi"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	var in pkgagent.InputPayload
	_ = json.Unmarshal(input.Payload, &in)
	if input.Type != TypeInput || in.Content != "[hi]" {
		t.Fatalf("input=%+v payload=%+v", input, in)
	}
	if string(DecodePluginContext(input.Context).Get("rewrite.seen")) != "true" {
		t.Fatalf("context=%s", input.Context)
	}

	handled, err := applyHooks(context.Background(), p, seam.Envelope{
		Type:    TypeInput,
		Payload: pkgagent.MarshalPayload(pkgagent.InputPayload{Content: "skip"}),
	})
	if err != nil || handled.Type != TypeInputHandled {
		t.Fatalf("handled=%+v err=%v", handled, err)
	}

	blocked, err := applyHooks(context.Background(), p, seam.Envelope{
		Type:    TypePreStep,
		Payload: pkgagent.MarshalPayload(pkgagent.PreStepPayload{SystemPrompt: "stop"}),
	})
	if err != nil || blocked.Type != TypeRunBlocked {
		t.Fatalf("blocked=%+v err=%v", blocked, err)
	}

	req, err := applyHooks(context.Background(), p, seam.Envelope{
		Type:    TypeRequest,
		Payload: pkgagent.MarshalPayload(pkgagent.RequestPayload{SystemPrompt: "old"}),
	})
	if err != nil || req.Type != TypeRequest {
		t.Fatalf("request=%+v err=%v", req, err)
	}
	var reqPayload pkgagent.RequestPayload
	_ = json.Unmarshal(req.Payload, &reqPayload)
	if reqPayload.SystemPrompt != "req" {
		t.Fatalf("request payload=%+v", reqPayload)
	}

	stream, err := applyHooks(context.Background(), p, seam.Envelope{
		Type:    TypeStream,
		Payload: pkgagent.MarshalPayload(pkgagent.StreamPayload{Headers: map[string]string{}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	var streamPayload pkgagent.StreamPayload
	_ = json.Unmarshal(stream.Payload, &streamPayload)
	if streamPayload.Headers["X"] != "1" {
		t.Fatalf("stream=%+v", streamPayload)
	}

	rewritten, err := applyHooks(context.Background(), p, seam.Envelope{
		Type:    TypePreExecute,
		Payload: pkgagent.MarshalPayload(tool.PreExecutePayload{Call: tool.Call{Name: "ping"}}),
	})
	if err != nil || rewritten.Type != TypePreExecute {
		t.Fatalf("pre=%+v err=%v", rewritten, err)
	}
	var pre tool.PreExecutePayload
	_ = json.Unmarshal(rewritten.Payload, &pre)
	if string(pre.Call.Arguments) != `{"x":2}` {
		t.Fatalf("pre payload=%+v", pre)
	}

	denied, err := applyHooks(context.Background(), p, seam.Envelope{
		Type:    TypePreExecute,
		Payload: pkgagent.MarshalPayload(tool.PreExecutePayload{Call: tool.Call{Name: "deny"}}),
	})
	if err != nil || denied.Type != TypeToolsDenied {
		t.Fatalf("denied=%+v err=%v", denied, err)
	}

	ask, err := applyHooks(context.Background(), p, seam.Envelope{
		Type:    TypePreExecute,
		Payload: pkgagent.MarshalPayload(tool.PreExecutePayload{Call: tool.Call{Name: "ask"}}),
	})
	if err != nil || ask.Type != TypeToolsAsk {
		t.Fatalf("ask=%+v err=%v", ask, err)
	}

	post, err := applyHooks(context.Background(), p, seam.Envelope{
		Type: TypePostExecute,
		Payload: pkgagent.MarshalPayload(tool.PostExecutePayload{
			Result: tool.Result{Output: json.RawMessage(`{}`)},
		}),
	})
	if err != nil || post.Type != TypePostExecute {
		t.Fatalf("post=%+v err=%v", post, err)
	}
	var postPayload tool.PostExecutePayload
	_ = json.Unmarshal(post.Payload, &postPayload)
	if string(postPayload.Result.Output) != `{"ok":true}` {
		t.Fatalf("post payload=%+v", postPayload)
	}
}

type notifyPlugin struct {
	stubPlugin
	got string
}

// OnLedgerNotify 记下事件类型供断言。
func (p *notifyPlugin) OnLedgerNotify(_ context.Context, in LedgerNotify) error {
	p.got = in.Type
	return nil
}

// TestApplyHooksNotify 确认非口事件走进 OnLedgerNotify。
func TestApplyHooksNotify(t *testing.T) {
	t.Parallel()
	p := &notifyPlugin{}
	ev, err := applyHooks(context.Background(), p, seam.Envelope{Type: "run.completed"})
	if err != nil || ev.Type != "run.completed" || p.got != "run.completed" {
		t.Fatalf("ev=%+v got=%s err=%v", ev, p.got, err)
	}
}
