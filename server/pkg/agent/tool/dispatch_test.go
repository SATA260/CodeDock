package tool

import (
	"context"
	"encoding/json"
	"testing"
)

type stubTool struct {
	def Definition
}

func (s stubTool) Definition() Definition { return s.def }

func (s stubTool) Execute(_ context.Context, input Input) (Result, error) {
	if s.def.Name == "boom" {
		return Result{CallID: input.Call.ID, Name: s.def.Name, Success: false, Error: "boom"}, nil
	}
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
				if err != nil || len(out.Results) != 1 || out.Results[0].Success || out.Results[0].Error == "" {
					t.Fatalf("want failed result, err = %v out=%+v", err, out)
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

func TestDispatchBestEffortContinuesAfterFailure(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(stubTool{def: Definition{Name: "boom", Permission: Permission{}}})
	_ = reg.Register(stubTool{def: Definition{Name: "ping", Permission: Permission{}}})

	out, err := Dispatch(context.Background(), Invocation{
		Calls: []Call{
			{ID: "b1", Name: "boom", Arguments: json.RawMessage(`{}`)},
			{ID: "p1", Name: "ping", Arguments: json.RawMessage(`{}`)},
		},
		AgentMode:     "yolo",
		FailurePolicy: FailureBestEffort,
		Registry:      reg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 2 || out.Results[0].Success || !out.Results[1].Success {
		t.Fatalf("out=%+v", out)
	}
}

func TestDispatchFailFastStopsOnFirstError(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(stubTool{def: Definition{Name: "boom", Permission: Permission{}}})
	_ = reg.Register(stubTool{def: Definition{Name: "ping", Permission: Permission{}}})

	out, err := Dispatch(context.Background(), Invocation{
		Calls: []Call{
			{ID: "b1", Name: "boom", Arguments: json.RawMessage(`{}`)},
			{ID: "p1", Name: "ping", Arguments: json.RawMessage(`{}`)},
		},
		AgentMode:     "yolo",
		FailurePolicy: FailureFast,
		Registry:      reg,
	})
	if err != nil {
		t.Fatalf("fail_fast should not return error, got %v", err)
	}
	if len(out.Results) != 2 || out.Results[0].Success || out.Results[1].Success || out.Results[1].Error != "tool did not execute" {
		t.Fatalf("out=%+v", out)
	}
}

func TestDispatchUnknownToolIsFailedResult(t *testing.T) {
	reg := NewRegistry()
	out, err := Dispatch(context.Background(), Invocation{
		Calls:     []Call{{ID: "c1", Name: "missing_tool", Arguments: json.RawMessage(`{}`)}},
		AgentMode: "yolo",
		Registry:  reg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 1 || out.Results[0].Success || out.Results[0].Error == "" {
		t.Fatalf("out=%+v", out)
	}
}
