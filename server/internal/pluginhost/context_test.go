package pluginhost

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/seam"
	sdk "codedock/pkg/plugin"
)

// TestHostPluginContextPersistsAcrossSeams 确认 input 写下的键在 pre-step（已有 Run）还能读到。
func TestHostPluginContextPersistsAcrossSeams(t *testing.T) {
	t.Parallel()
	h := &Host{contexts: map[string]json.RawMessage{}}
	first, err := h.Dispatch(context.Background(), seam.Envelope{
		Type:      seam.TypeInput,
		SessionID: "s1",
		Context:   json.RawMessage(`{"hello.marked":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(sdk.DecodePluginContext(first.Context).Get("hello.marked")) != "true" {
		t.Fatalf("input context=%s", first.Context)
	}

	second, err := h.Dispatch(context.Background(), seam.Envelope{
		Type:      seam.TypePreStep,
		SessionID: "s1",
		RunID:     "r1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(sdk.DecodePluginContext(second.Context).Get("hello.marked")) != "true" {
		t.Fatalf("pre-step context=%s", second.Context)
	}
}

// TestHostPluginContextRejectedKeepsPrevious 确认超限袋子不会盖掉上一份。
func TestHostPluginContextRejectedKeepsPrevious(t *testing.T) {
	t.Parallel()
	h := &Host{contexts: map[string]json.RawMessage{}}
	if _, err := h.Dispatch(context.Background(), seam.Envelope{
		Type:      seam.TypeInput,
		SessionID: "s1",
		Context:   json.RawMessage(`{"hello.marked":true}`),
	}); err != nil {
		t.Fatal(err)
	}
	tooBig := json.RawMessage(`{"k":"` + strings.Repeat("x", sdk.MaxPluginContextBytes) + `"}`)
	got := h.attachPluginContext(seam.Envelope{
		Type:      seam.TypePreStep,
		SessionID: "s1",
		Context:   tooBig,
	})
	if string(sdk.DecodePluginContext(got.Context).Get("hello.marked")) != "true" {
		t.Fatalf("kept=%s", got.Context)
	}
}

// TestHostPluginContextDroppedOnHandledAndTerminal 确认 handled 与终态会清掉袋子。
func TestHostPluginContextDroppedOnHandledAndTerminal(t *testing.T) {
	t.Parallel()
	h := &Host{contexts: map[string]json.RawMessage{}}
	if _, err := h.Dispatch(context.Background(), seam.Envelope{
		Type:      seam.TypeInput,
		SessionID: "s1",
		Context:   json.RawMessage(`{"hello.marked":true}`),
	}); err != nil {
		t.Fatal(err)
	}
	h.dropSessionPluginContext("s1")
	if raw := h.lookupPluginContext("s1", ""); raw != nil {
		t.Fatalf("handled should drop session bag: %s", raw)
	}

	h.persistPluginContext(seam.Envelope{SessionID: "s2", RunID: "r2", Context: json.RawMessage(`{"a":1}`)})
	h.notify(context.Background(), seam.Envelope{Type: string(pkgagent.EventRunCompleted), SessionID: "s2", RunID: "r2"})
	if raw := h.lookupPluginContext("s2", "r2"); raw != nil {
		t.Fatalf("terminal should drop: %s", raw)
	}
}
