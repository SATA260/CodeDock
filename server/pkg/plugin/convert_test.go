package plugin

import (
	"encoding/json"
	"testing"

	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/seam"
	"codedock/pkg/agent/tool"
)

// TestEnvelopeRoundTrip 确认信封与 proto 往返不丢字段。
func TestEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()
	in := seam.Envelope{
		Type:      TypeInput,
		ChainID:   "c1",
		Seen:      []string{"echo"},
		SessionID: "s1",
		RunID:     "r1",
		TurnID:    "t1",
		Payload:   json.RawMessage(`{"content":"hi"}`),
		Context:   json.RawMessage(`{"hello.marked":true}`),
	}
	got := envelopeFromPB(envelopeToPB(in))
	if got.Type != in.Type || got.ChainID != in.ChainID || got.SessionID != in.SessionID || got.RunID != in.RunID || got.TurnID != in.TurnID {
		t.Fatalf("got=%+v", got)
	}
	if string(got.Payload) != string(in.Payload) || len(got.Seen) != 1 || got.Seen[0] != "echo" {
		t.Fatalf("got=%+v", got)
	}
	if string(got.Context) != string(in.Context) {
		t.Fatalf("context=%s", got.Context)
	}
}

// TestMethodRoundTripAndDefinition 确认方法描述往返并能映射成工具定义。
func TestMethodRoundTripAndDefinition(t *testing.T) {
	t.Parallel()
	in := Method{
		Name:             "echo",
		Prompt:           "echo text",
		ParametersSchema: json.RawMessage(`{"type":"object"}`),
		Capabilities:     []string{"read"},
		RequiresApproval: true,
	}
	got := methodFromPB(methodToPB(in))
	if got.Name != in.Name || got.Prompt != in.Prompt || got.RequiresApproval != in.RequiresApproval || string(got.ParametersSchema) != string(in.ParametersSchema) {
		t.Fatalf("got=%+v", got)
	}
	def := MethodToDefinition(got)
	if def.Name != "echo" || def.Permission.Effect != tool.EffectAsk {
		t.Fatalf("def=%+v", def)
	}
}

// TestHiddenText 确认隐藏提示是一条 system 消息。
func TestHiddenText(t *testing.T) {
	t.Parallel()
	msg := HiddenText("note")
	if msg.Role != pkgagent.RoleSystem || pkgagent.DecodeText(msg.Content) != "note" {
		t.Fatalf("hidden=%+v", msg)
	}
}

// TestInputReplyAndHandle 确认 Reply 继续、Handle 标记不建 Run。
func TestInputReplyAndHandle(t *testing.T) {
	t.Parallel()
	in := AgentInput{Content: "x", Mode: "yolo"}
	in.Context.Set("hello.marked", true)
	if got := in.Reply(); got.Handled || got.Content != "x" || got.Mode != "yolo" || string(got.Context.Get("hello.marked")) != "true" {
		t.Fatalf("reply=%+v", got)
	}
	if got := in.Handle(); !got.Handled || got.Content != "x" {
		t.Fatalf("handle=%+v", got)
	}
}
