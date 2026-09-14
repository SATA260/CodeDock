package plugin

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestPluginContextSetGet 确认 Set / Get / Clone / Raw 往返。
func TestPluginContextSetGet(t *testing.T) {
	t.Parallel()
	var bag PluginContext
	bag.Set("hello.marked", true)
	bag.Set("", "ignore")
	if string(bag.Get("hello.marked")) != "true" || bag.Get("missing") != nil {
		t.Fatalf("bag=%+v", bag)
	}
	clone := bag.Clone()
	clone.Set("hello.marked", false)
	if string(bag.Get("hello.marked")) != "true" {
		t.Fatalf("clone mutated original: %s", bag.Get("hello.marked"))
	}
	decoded := DecodePluginContext(bag.Raw())
	if string(decoded.Get("hello.marked")) != "true" {
		t.Fatalf("raw=%s", bag.Raw())
	}
}

// TestAcceptableContext 确认空对象可通过、超限和数组被拒。
func TestAcceptableContext(t *testing.T) {
	t.Parallel()
	if !AcceptableContext(nil) || !AcceptableContext(json.RawMessage(`{}`)) {
		t.Fatal("empty should pass")
	}
	if AcceptableContext(json.RawMessage(`[]`)) {
		t.Fatal("array should fail")
	}
	tooBig := json.RawMessage(`{"k":"` + strings.Repeat("x", MaxPluginContextBytes) + `"}`)
	if AcceptableContext(tooBig) {
		t.Fatal("oversize should fail")
	}
}
