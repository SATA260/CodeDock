package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
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

func dispatchInv(reg Registry, calls []Call, approval ApprovalMode) Invocation {
	names := make([]string, 0, 8)
	for _, def := range Definitions(reg) {
		names = append(names, def.Name)
	}
	if len(names) == 0 {
		for _, call := range calls {
			names = append(names, call.Name)
		}
	}
	return Invocation{
		Calls:      calls,
		BoundNames: names,
		Approval:   approval,
		Registry:   reg,
	}
}

func TestDispatchPipeline(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(stubTool{def: Definition{Name: "memory_read", Permission: Permission{Effect: EffectAllow}}})
	_ = reg.Register(stubTool{def: Definition{Name: "memory_write", Permission: Permission{Effect: EffectAsk}}})
	_ = reg.Register(stubTool{def: Definition{Name: "file_write", Permission: Permission{Effect: EffectAsk}}})
	_ = reg.Register(stubTool{def: Definition{Name: "ping", Permission: Permission{Effect: EffectAsk}}})

	t.Run("allow skips later layers", func(t *testing.T) {
		inv := dispatchInv(reg, []Call{{ID: "c1", Name: "memory_read", Arguments: json.RawMessage(`{}`)}}, ApprovalManual)
		inv.Effects = map[string]Effect{"memory_read": EffectDeny}
		out, err := Dispatch(context.Background(), inv)
		if err != nil || len(out.Results) != 1 || !out.Results[0].Success {
			t.Fatalf("err=%v out=%+v", err, out)
		}
	})

	t.Run("ask plus yolo executes", func(t *testing.T) {
		out, err := Dispatch(context.Background(), dispatchInv(reg, []Call{{ID: "c1", Name: "file_write", Arguments: json.RawMessage(`{}`)}}, ApprovalYolo))
		if err != nil || len(out.Results) != 1 || !out.Results[0].Success {
			t.Fatalf("err=%v out=%+v", err, out)
		}
	})

	t.Run("ask plus agent deny", func(t *testing.T) {
		inv := dispatchInv(reg, []Call{{ID: "c1", Name: "file_write", Arguments: json.RawMessage(`{}`)}}, ApprovalYolo)
		inv.Effects = map[string]Effect{"file_write": EffectDeny}
		out, err := Dispatch(context.Background(), inv)
		if err != nil || len(out.Results) != 1 || out.Results[0].Success {
			t.Fatalf("err=%v out=%+v", err, out)
		}
	})

	t.Run("unbound fails", func(t *testing.T) {
		inv := dispatchInv(reg, []Call{{ID: "c1", Name: "file_write", Arguments: json.RawMessage(`{}`)}}, ApprovalYolo)
		inv.BoundNames = []string{"ping"}
		out, err := Dispatch(context.Background(), inv)
		if err != nil || out.WaitingApproval || len(out.Results) != 1 || out.Results[0].Success || out.Results[0].Error != "tool is not bound" {
			t.Fatalf("err=%v out=%+v", err, out)
		}
	})
	t.Run("unbound allow tool still denied", func(t *testing.T) {
		inv := dispatchInv(reg, []Call{{ID: "c1", Name: "memory_read", Arguments: json.RawMessage(`{}`)}}, ApprovalYolo)
		inv.BoundNames = []string{"ping"}
		inv.ApprovedCallIDs = []string{"c1"}
		out, err := Dispatch(context.Background(), inv)
		if err != nil || out.WaitingApproval || len(out.Results) != 1 || out.Results[0].Success || out.Results[0].Error != "tool is not bound" {
			t.Fatalf("err=%v out=%+v", err, out)
		}
	})
}

func TestDispatchWaitsEntireBatch(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(stubTool{def: Definition{Name: "memory_read", Permission: Permission{Effect: EffectAllow}}})
	_ = reg.Register(stubTool{def: Definition{Name: "memory_write", Permission: Permission{Effect: EffectAsk}}})

	out, err := Dispatch(context.Background(), dispatchInv(reg, []Call{
		{ID: "r1", Name: "memory_read", Arguments: json.RawMessage(`{}`)},
		{ID: "w1", Name: "memory_write", Arguments: json.RawMessage(`{}`)},
	}, ApprovalManual))
	if err != nil {
		t.Fatal(err)
	}
	if !out.WaitingApproval || len(out.Results) != 0 || len(out.PendingCalls) != 2 || len(out.ApprovalCalls) != 1 || out.ApprovalCalls[0].ID != "w1" {
		t.Fatalf("out=%+v", out)
	}
}

func TestDispatchDeniedCallSkipsExecute(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(stubTool{def: Definition{Name: "memory_write", Permission: Permission{Effect: EffectAsk}}})

	inv := dispatchInv(reg, []Call{{ID: "w1", Name: "memory_write", Arguments: json.RawMessage(`{}`)}}, ApprovalManual)
	inv.DeniedCallIDs = []string{"w1"}
	out, err := Dispatch(context.Background(), inv)
	if err != nil {
		t.Fatal(err)
	}
	if out.WaitingApproval || len(out.Results) != 1 || out.Results[0].Success || out.Results[0].Error != "approval denied" {
		t.Fatalf("out=%+v", out)
	}
}

func TestDispatchBestEffortContinuesAfterFailure(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(stubTool{def: Definition{Name: "boom", Permission: Permission{Effect: EffectAllow}}})
	_ = reg.Register(stubTool{def: Definition{Name: "ping", Permission: Permission{Effect: EffectAllow}}})

	inv := dispatchInv(reg, []Call{
		{ID: "b1", Name: "boom", Arguments: json.RawMessage(`{}`)},
		{ID: "p1", Name: "ping", Arguments: json.RawMessage(`{}`)},
	}, ApprovalYolo)
	inv.FailurePolicy = FailureBestEffort
	out, err := Dispatch(context.Background(), inv)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 2 || out.Results[0].Success || !out.Results[1].Success {
		t.Fatalf("out=%+v", out)
	}
}

func TestDispatchFailFastStopsOnFirstError(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(stubTool{def: Definition{Name: "boom", Permission: Permission{Effect: EffectAllow}}})
	_ = reg.Register(stubTool{def: Definition{Name: "ping", Permission: Permission{Effect: EffectAllow}}})

	inv := dispatchInv(reg, []Call{
		{ID: "b1", Name: "boom", Arguments: json.RawMessage(`{}`)},
		{ID: "p1", Name: "ping", Arguments: json.RawMessage(`{}`)},
	}, ApprovalYolo)
	inv.FailurePolicy = FailureFast
	out, err := Dispatch(context.Background(), inv)
	if err != nil {
		t.Fatalf("fail_fast should not return error, got %v", err)
	}
	if len(out.Results) != 2 || out.Results[0].Success || out.Results[1].Success || out.Results[1].Error != "tool did not execute" {
		t.Fatalf("out=%+v", out)
	}
}

type outsideTool struct{ stubTool }

func (outsideTool) Inspect(context.Context, Input) error { return ErrOutsideWorkspace }

type allowAskTool struct{ stubTool }

func (t allowAskTool) ResolveEffect(context.Context, Input) Effect {
	return EffectAllow
}

func TestDispatchResolveEffectCanSkipApproval(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(allowAskTool{stubTool{def: Definition{Name: "write", Permission: Permission{Effect: EffectAsk}}}})

	out, err := Dispatch(context.Background(), dispatchInv(reg, []Call{{ID: "c1", Name: "write", Arguments: json.RawMessage(`{}`)}}, ApprovalManual))
	if err != nil || out.WaitingApproval || len(out.Results) != 1 || !out.Results[0].Success {
		t.Fatalf("resolver allow should skip approval: %v %+v", err, out)
	}
}

func TestDispatchOutsideWorkspaceNeedsApproval(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(outsideTool{stubTool{def: Definition{Name: "read", Permission: Permission{Effect: EffectAllow}}}})

	inv := dispatchInv(reg, []Call{{ID: "c1", Name: "read", Arguments: json.RawMessage(`{"path":"../x"}`)}}, ApprovalYolo)
	out, err := Dispatch(context.Background(), inv)
	if err != nil {
		t.Fatal(err)
	}
	if !out.WaitingApproval || len(out.ApprovalCalls) != 1 {
		t.Fatalf("yolo must not skip outside-workspace approval: %+v", out)
	}

	inv.ApprovedCallIDs = []string{"c1"}
	out, err = Dispatch(context.Background(), inv)
	if err != nil || len(out.Results) != 1 || !out.Results[0].Success {
		t.Fatalf("approved outside %v %+v", err, out)
	}
}

func TestDispatchUnknownToolIsFailedResult(t *testing.T) {
	reg := NewRegistry()
	out, err := Dispatch(context.Background(), Invocation{
		Calls:      []Call{{ID: "c1", Name: "missing_tool", Arguments: json.RawMessage(`{}`)}},
		BoundNames: []string{"missing_tool"},
		Approval:   ApprovalYolo,
		Registry:   reg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 1 || out.Results[0].Success || out.Results[0].Error == "" {
		t.Fatalf("out=%+v", out)
	}
}

type errTool struct{}

func (errTool) Definition() Definition {
	return Definition{Name: "errt", Permission: Permission{Effect: EffectAllow}}
}

func (errTool) Execute(_ context.Context, input Input) (Result, error) {
	return Result{}, fmt.Errorf("boom")
}

type emptyNameTool struct{}

func (emptyNameTool) Definition() Definition {
	return Definition{Name: "emptyname", Permission: Permission{Effect: EffectAllow}}
}

func (emptyNameTool) Execute(_ context.Context, input Input) (Result, error) {
	return Result{CallID: input.Call.ID, Success: true}, nil
}

func TestDispatchNilRegistryAndFailFastPrepare(t *testing.T) {
	if _, err := Dispatch(context.Background(), Invocation{}); err == nil {
		t.Fatal("nil registry")
	}
	reg := NewRegistry()
	_ = reg.Register(stubTool{def: Definition{Name: "ping", Permission: Permission{Effect: EffectAllow}}})
	inv := dispatchInv(reg, []Call{
		{ID: "m1", Name: "missing"},
		{ID: "p1", Name: "ping"},
	}, ApprovalYolo)
	inv.FailurePolicy = FailureFast
	out, err := Dispatch(context.Background(), inv)
	if err != nil || len(out.Results) != 2 || out.Results[0].Success || out.Results[1].Success {
		t.Fatalf("err=%v out=%+v", err, out)
	}
}

func TestDispatchExecuteErrorAndEmptyName(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(errTool{})
	_ = reg.Register(emptyNameTool{})
	out, err := Dispatch(context.Background(), dispatchInv(reg, []Call{{ID: "e1", Name: "errt"}}, ApprovalYolo))
	if err != nil || len(out.Results) != 1 || out.Results[0].Success || out.Results[0].Error != "boom" {
		t.Fatalf("err=%v out=%+v", err, out)
	}
	out, err = Dispatch(context.Background(), dispatchInv(reg, []Call{{ID: "n1", Name: "emptyname"}}, ApprovalYolo))
	if err != nil || len(out.Results) != 1 || !out.Results[0].Success || out.Results[0].Name != "emptyname" {
		t.Fatalf("err=%v out=%+v", err, out)
	}
}

func TestDispatchParallelSkipAndCancel(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(stubTool{def: Definition{Name: "ping", Permission: Permission{Effect: EffectAllow}}})
	_ = reg.Register(cancelTool{})
	inv := dispatchInv(reg, []Call{
		{ID: "d1", Name: "ping"},
		{ID: "p1", Name: "ping"},
	}, ApprovalYolo)
	inv.DeniedCallIDs = []string{"d1"}
	inv.Mode = ExecutionParallel
	inv.MaxParallel = 2
	out, err := Dispatch(context.Background(), inv)
	if err != nil || len(out.Results) != 2 || out.Results[0].Success || !out.Results[1].Success {
		t.Fatalf("err=%v out=%+v", err, out)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inv = dispatchInv(reg, []Call{
		{ID: "c1", Name: "cancel"},
		{ID: "c2", Name: "ping"},
	}, ApprovalYolo)
	inv.Mode = ExecutionParallel
	inv.MaxParallel = 2
	_, _ = Dispatch(ctx, inv)
}

func TestValidateSchemaAndMax(t *testing.T) {
	if err := validateArguments(Definition{ParametersSchema: json.RawMessage(`not-json`)}, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := validateArguments(Definition{ParametersSchema: json.RawMessage(`{"required":[1,"x"]}`)}, json.RawMessage(`{"x":1}`)); err != nil {
		t.Fatal(err)
	}
	if max(2, 1) != 2 || max(1, 3) != 3 {
		t.Fatal(max(2, 1))
	}
	got := appendSkippedResults(nil, []preparedCall{
		{skip: true, result: Result{CallID: "s", Success: false}},
		{call: Call{ID: "r", Name: "x"}},
	})
	if len(got) != 2 || got[0].CallID != "s" || got[1].CallID != "r" {
		t.Fatalf("%+v", got)
	}
}

func TestDispatchParallelInterruptAndRetryCancel(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(cancelTool{})
	_ = reg.Register(&retryTool{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inv := dispatchInv(reg, []Call{
		{ID: "a", Name: "cancel"},
		{ID: "b", Name: "cancel"},
	}, ApprovalYolo)
	inv.Mode = ExecutionParallel
	inv.MaxParallel = 2
	out, err := Dispatch(ctx, inv)
	if err == nil && (len(out.Results) != 2 || out.Results[0].Success) {
		t.Fatalf("err=%v out=%+v", err, out)
	}

	inv = dispatchInv(reg, []Call{{ID: "r", Name: "retry", Arguments: json.RawMessage(`{"x":"1"}`)}}, ApprovalYolo)
	inv.OnEvent = func(string, Call, int, *Result) {}
	out, err = Dispatch(context.Background(), inv)
	if err != nil || len(out.Results) != 1 {
		t.Fatalf("retry err=%v out=%+v", err, out)
	}
}

func TestLimitedGroupCancelAndFail(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	group, _ := withLimit(ctx, 0)
	group.go_(func() error { return nil })
	_ = group.wait()
	group.fail(fmt.Errorf("first"))
	group.fail(fmt.Errorf("second"))
}

type retryTool struct {
	fails atomic.Int32
}

func (retryTool) Definition() Definition {
	return Definition{
		Name:             "retry",
		Permission:       Permission{Effect: EffectAllow},
		SupportsRetry:    true,
		ParametersSchema: json.RawMessage(`{"type":"object","required":["x"],"properties":{"x":{"type":"string"}}}`),
	}
}

func (t *retryTool) Execute(_ context.Context, input Input) (Result, error) {
	if t.fails.Add(1) < 2 {
		return Result{CallID: input.Call.ID, Name: "retry", Success: false, Error: "temp"}, nil
	}
	return Result{CallID: input.Call.ID, Name: "retry", Success: true, Output: json.RawMessage(`{"ok":true}`)}, nil
}

type inspectDeny struct{ stubTool }

func (inspectDeny) Inspect(context.Context, Input) error {
	return errInvalidArguments
}

type cancelTool struct{}

func (cancelTool) Definition() Definition {
	return Definition{Name: "cancel", Permission: Permission{Effect: EffectAllow}}
}

func (cancelTool) Execute(ctx context.Context, input Input) (Result, error) {
	return Result{CallID: input.Call.ID, Name: "cancel", Success: false, Error: ctx.Err().Error()}, ctx.Err()
}

func TestDispatchParallelRetryInspectAndSchema(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&retryTool{})
	_ = reg.Register(stubTool{def: Definition{Name: "ping", Permission: Permission{Effect: EffectAllow}}})
	_ = reg.Register(inspectDeny{stubTool: stubTool{def: Definition{Name: "askme", Permission: Permission{Effect: EffectAsk}}}})
	_ = reg.Register(cancelTool{})

	t.Run("schema missing", func(t *testing.T) {
		inv := dispatchInv(reg, []Call{{ID: "c1", Name: "retry", Arguments: json.RawMessage(`{}`)}}, ApprovalYolo)
		out, err := Dispatch(context.Background(), inv)
		if err != nil || len(out.Results) != 1 || out.Results[0].Success {
			t.Fatalf("err=%v out=%+v", err, out)
		}
	})

	t.Run("retry then ok", func(t *testing.T) {
		inv := dispatchInv(reg, []Call{{ID: "c1", Name: "retry", Arguments: json.RawMessage(`{"x":"1"}`)}}, ApprovalYolo)
		inv.OnEvent = func(string, Call, int, *Result) {}
		out, err := Dispatch(context.Background(), inv)
		if err != nil || len(out.Results) != 1 || !out.Results[0].Success {
			t.Fatalf("err=%v out=%+v", err, out)
		}
	})

	t.Run("inspect deny", func(t *testing.T) {
		inv := dispatchInv(reg, []Call{{ID: "c1", Name: "askme", Arguments: json.RawMessage(`{}`)}}, ApprovalYolo)
		out, err := Dispatch(context.Background(), inv)
		if err != nil || len(out.Results) != 1 || out.Results[0].Success {
			t.Fatalf("err=%v out=%+v", err, out)
		}
	})

	t.Run("parallel", func(t *testing.T) {
		inv := dispatchInv(reg, []Call{
			{ID: "p1", Name: "ping", Arguments: json.RawMessage(`{}`)},
			{ID: "p2", Name: "ping", Arguments: json.RawMessage(`{}`)},
		}, ApprovalYolo)
		inv.Mode = ExecutionParallel
		inv.MaxParallel = 2
		out, err := Dispatch(context.Background(), inv)
		if err != nil || len(out.Results) != 2 || !out.Results[0].Success || !out.Results[1].Success {
			t.Fatalf("err=%v out=%+v", err, out)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		inv := dispatchInv(reg, []Call{{ID: "c1", Name: "cancel", Arguments: json.RawMessage(`{}`)}}, ApprovalYolo)
		out, err := Dispatch(ctx, inv)
		if err == nil && (len(out.Results) != 1 || out.Results[0].Success) {
			t.Fatalf("want cancel err=%v out=%+v", err, out)
		}
	})
}

func TestRegistryHelpers(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(nil); err == nil {
		t.Fatal("nil tool")
	}
	if err := reg.Register(stubTool{def: Definition{}}); err == nil {
		t.Fatal("empty name")
	}
	_ = reg.Register(stubTool{def: Definition{Name: "ping", Version: "1", Permission: Permission{Effect: EffectAllow}}})
	if _, err := reg.Get(Reference{Name: "ping", Version: "2"}); err == nil {
		t.Fatal("version mismatch")
	}
	items, err := reg.GetAll([]Reference{{Name: "ping"}})
	if err != nil || len(items) != 1 {
		t.Fatalf("%v %d", err, len(items))
	}
	if _, err := reg.GetAll([]Reference{{Name: "missing"}}); err == nil {
		t.Fatal("expected missing")
	}
	if got := Definitions(nil); got != nil {
		t.Fatalf("nil registry %v", got)
	}
}

func TestValidateArguments(t *testing.T) {
	def := Definition{ParametersSchema: json.RawMessage(`{"required":["a"]}`)}
	if err := validateArguments(def, json.RawMessage(`not-json`)); err == nil {
		t.Fatal("expected invalid json")
	}
	if err := validateArguments(def, nil); err == nil {
		t.Fatal("expected missing a")
	}
	if err := validateArguments(Definition{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := validateArguments(def, json.RawMessage(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
}

func TestLayerAgentInvalidKeepsAsk(t *testing.T) {
	if Pipeline(PipelineInput{Default: EffectAsk, Bound: true, HasAgentEffect: true, AgentEffect: Effect("nope"), Approval: ApprovalManual}) != EffectAsk {
		t.Fatal("invalid agent effect should stay ask")
	}
}

func TestFailGateAndRetryable(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(stubTool{def: Definition{Name: "ping", Permission: Permission{Effect: EffectAllow}}})
	inv := dispatchInv(reg, []Call{{ID: "c1", Name: "ping", Arguments: json.RawMessage(`{}`)}}, ApprovalYolo)
	inv.Gate = failGate{}
	out, err := Dispatch(context.Background(), inv)
	if err == nil && (len(out.Results) != 1 || out.Results[0].Success) {
		t.Fatalf("gate fail err=%v out=%+v", err, out)
	}
	if retryableTool(nil) || retryableTool(context.Canceled) || !retryableTool(errInvalidArguments) {
		t.Fatal("retryable mismatch")
	}
	if maxAttempts(inv) != 3 {
		t.Fatal(maxAttempts(inv))
	}
}

type failGate struct{}

func (failGate) Acquire(context.Context) error { return context.Canceled }
func (failGate) Release()                      {}

func TestSkippedRemaining(t *testing.T) {
	items := skippedRemaining([]Call{{ID: "a", Name: "x"}})
	if len(items) != 1 || !items[0].skip {
		t.Fatalf("%+v", items)
	}
	got := appendSkippedResults(nil, []preparedCall{{call: Call{ID: "b", Name: "y"}}})
	if len(got) != 1 || got[0].Success {
		t.Fatalf("%+v", got)
	}
	_ = time.Second
}
