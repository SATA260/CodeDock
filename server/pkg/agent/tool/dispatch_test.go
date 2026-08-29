package tool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type stubTool struct {
	def Definition
}

func (s stubTool) Definition() Definition { return s.def }

func (s stubTool) Execute(_ context.Context, input Input) (Result, error) {
	return Result{CallID: input.Call.ID, Name: s.def.Name, Output: json.RawMessage(`{"ok":true}`), Success: true}, nil
}

func TestDispatchModeCapabilityCoverage(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(stubTool{def: Definition{Name: "memory_read", Permission: Permission{Capabilities: []Capability{CapabilityMemory}}}})
	_ = reg.Register(stubTool{def: Definition{Name: "memory_write", Permission: Permission{Capabilities: []Capability{CapabilityMemory}, RequiresApproval: true}}})
	_ = reg.Register(stubTool{def: Definition{Name: "file_write", Permission: Permission{Capabilities: []Capability{CapabilityWrite}}}})
	_ = reg.Register(stubTool{def: Definition{Name: "ping", Permission: Permission{}}})

	cases := []struct {
		name    string
		mode    string
		tool    string
		wantErr bool
	}{
		{name: "plan memory read ok", mode: "plan", tool: "memory_read"},
		{name: "plan memory write ok", mode: "plan", tool: "memory_write"},
		{name: "plan write denied", mode: "plan", tool: "file_write", wantErr: true},
		{name: "plan empty caps ok", mode: "plan", tool: "ping"},
		{name: "ask write denied", mode: "ask", tool: "file_write", wantErr: true},
		{name: "approve write ok", mode: "auto_approve", tool: "file_write"},
		{name: "approve memory write ok", mode: "auto_approve", tool: "memory_write"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Dispatch(context.Background(), Invocation{
				Calls:     []Call{{ID: "c1", Name: tc.tool, Arguments: json.RawMessage(`{}`)}},
				AgentMode: tc.mode,
				Registry:  reg,
			})
			if tc.wantErr {
				if err == nil || !errors.Is(err, errPermissionDenied) {
					t.Fatalf("err = %v out=%+v", err, out)
				}
				return
			}
			if err != nil || len(out.Results) != 1 || !out.Results[0].Success {
				t.Fatalf("err = %v out=%+v", err, out)
			}
		})
	}
}

func TestDispatchWaitsEntireBatch(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(stubTool{def: Definition{Name: "memory_read", Permission: Permission{Capabilities: []Capability{CapabilityMemory}}}})
	_ = reg.Register(stubTool{def: Definition{Name: "memory_write", Permission: Permission{Capabilities: []Capability{CapabilityMemory}, RequiresApproval: true}}})

	out, err := Dispatch(context.Background(), Invocation{
		Calls: []Call{
			{ID: "r1", Name: "memory_read", Arguments: json.RawMessage(`{}`)},
			{ID: "w1", Name: "memory_write", Arguments: json.RawMessage(`{}`)},
		},
		AgentMode: "ask_for_approval",
		Registry:  reg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.WaitingApproval || len(out.Results) != 0 || len(out.PendingCalls) != 2 || len(out.ApprovalCalls) != 1 || out.ApprovalCalls[0].ID != "w1" {
		t.Fatalf("out=%+v", out)
	}
}

func TestDispatchDeniedCallSkipsExecute(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(stubTool{def: Definition{Name: "memory_write", Permission: Permission{Capabilities: []Capability{CapabilityMemory}, RequiresApproval: true}}})

	out, err := Dispatch(context.Background(), Invocation{
		Calls:         []Call{{ID: "w1", Name: "memory_write", Arguments: json.RawMessage(`{}`)}},
		AgentMode:     "ask_for_approval",
		Registry:      reg,
		DeniedCallIDs: []string{"w1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.WaitingApproval || len(out.Results) != 1 || out.Results[0].Success || out.Results[0].Error != "approval denied" {
		t.Fatalf("out=%+v", out)
	}
}
