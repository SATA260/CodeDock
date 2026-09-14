package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"codedock/pkg/agent/tool"
)

type memFacts struct {
	mu    sync.Mutex
	facts []Fact
}

func (m *memFacts) Append(_ context.Context, _ string, fact Fact) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.facts = append(m.facts, fact)
	return nil
}

type stubPing struct{}

func (stubPing) Definition() tool.Definition {
	return tool.Definition{
		Name:       "ping",
		Prompt:     "ping",
		Permission: tool.Permission{RequiresApproval: true},
		Version:    "1",
	}
}

func (stubPing) Execute(_ context.Context, input tool.Input) (tool.Result, error) {
	return tool.Result{CallID: input.Call.ID, Name: "ping", Success: true, Output: json.RawMessage(`{"ok":true}`)}, nil
}

func testEngine(t *testing.T) (*Engine, *memFacts, tool.Registry) {
	t.Helper()
	facts := &memFacts{}
	reg := tool.NewRegistry()
	if err := reg.Register(stubPing{}); err != nil {
		t.Fatal(err)
	}
	return NewEngine(&Brain{}, facts, reg), facts, reg
}

func fakeHistory(runID string, opts FakeOptions) History {
	cfg := DefaultRunConfig(ModeAutoApprove, ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(opts)})
	return History{
		Run: Run{
			ID:        runID,
			SessionID: "sess-1",
			Config:    cfg,
		},
		Messages: []Message{{Role: RoleUser, Content: EncodeText("hi")}},
		Prompt:   "test",
	}
}

func mustRaw(v any) json.RawMessage {
	body, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return body
}

func TestEngineStepCallsDecide(t *testing.T) {
	engine := NewEngine(&Brain{}, nil, nil)
	got, err := engine.Step(context.Background(), StepInput{
		State: AgentState{RunID: "run-1"},
		Job:   StepJob{RunID: "run-1", StepIndex: 1, Phase: PhaseLLMResult},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunCompleted {
		t.Fatalf("status=%s want completed", got.State.Status)
	}
	if got.Next != nil {
		t.Fatal("text llm_result should finish")
	}
}

func TestEngineCallLLMText(t *testing.T) {
	engine, facts, _ := testEngine(t)
	state := AgentState{
		SessionID: "sess-1",
		RunID:     "run-1",
		Config:    DefaultRunConfig(ModeAutoApprove, ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(FakeOptions{Turns: []FakeTurn{{Text: "hello"}}})}),
	}
	got, err := engine.Step(context.Background(), StepInput{
		State:   state,
		Job:     StepJob{RunID: "run-1", StepIndex: 1, Phase: PhaseUserInput},
		History: fakeHistory("run-1", FakeOptions{Turns: []FakeTurn{{Text: "hello"}}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunRunningLLM {
		t.Fatalf("status=%s", got.State.Status)
	}
	if got.Next == nil || got.Next.Phase != PhaseLLMResult {
		t.Fatalf("next=%+v", got.Next)
	}
	if len(got.Messages) != 1 || DecodeText(got.Messages[0].Content) != "hello" {
		t.Fatalf("messages=%+v", got.Messages)
	}
	if len(facts.facts) == 0 {
		t.Fatal("expected assistant delta facts")
	}
}

func TestEngineCallLLMToolsThenBatch(t *testing.T) {
	engine, _, _ := testEngine(t)
	opts := FakeOptions{Turns: []FakeTurn{{ToolCalls: []FakeToolCall{{Name: "ping"}}}}}
	state := AgentState{
		SessionID: "sess-1",
		RunID:     "run-1",
		Config:    DefaultRunConfig(ModeAutoApprove, ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(opts)}),
	}
	llm, err := engine.Step(context.Background(), StepInput{
		State:   state,
		Job:     StepJob{RunID: "run-1", StepIndex: 1, Phase: PhaseUserInput},
		History: fakeHistory("run-1", opts),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(llm.State.Checkpoint.Pending) != 1 {
		t.Fatalf("pending=%d", len(llm.State.Checkpoint.Pending))
	}
	tools, err := engine.Step(context.Background(), StepInput{
		State: llm.State,
		Job:   StepJob{RunID: "run-1", StepIndex: 2, Phase: PhaseLLMResult},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tools.State.Status != RunExecutingTools {
		t.Fatalf("status=%s", tools.State.Status)
	}
	if tools.Next == nil || tools.Next.Phase != PhaseToolsBatchResult {
		t.Fatalf("next=%+v", tools.Next)
	}
	if len(tools.Messages) != 1 || tools.Messages[0].Role != RoleTool {
		t.Fatalf("tool messages=%+v", tools.Messages)
	}
}

func TestEngineCallToolsWaitingApproval(t *testing.T) {
	engine, _, _ := testEngine(t)
	state := AgentState{
		SessionID: "sess-1",
		RunID:     "run-1",
		Config:    DefaultRunConfig(ModeAskForApproval, ModelConfig{Provider: "fake", Model: "fake"}),
		Checkpoint: ToolCheckpoint{
			Pending: []tool.Call{{ID: "c1", Name: "ping", Arguments: json.RawMessage(`{}`)}},
		},
	}
	got, err := engine.Step(context.Background(), StepInput{
		State: state,
		Job:   StepJob{RunID: "run-1", StepIndex: 2, Phase: PhaseLLMResult},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunWaitingApproval {
		t.Fatalf("status=%s", got.State.Status)
	}
	if got.Next != nil {
		t.Fatal("waiting approval should not enqueue")
	}
	if len(got.Facts) != 1 || got.Facts[0].Type != EventApprovalRequired {
		t.Fatalf("facts=%+v", got.Facts)
	}
}

func TestEngineCallToolsApprovalEventListsWholeBatch(t *testing.T) {
	engine, _, reg := testEngine(t)
	if err := reg.Register(stubEcho{}); err != nil {
		t.Fatal(err)
	}
	got, err := engine.Step(context.Background(), StepInput{
		State: AgentState{
			SessionID: "sess-1",
			RunID:     "run-1",
			Config:    DefaultRunConfig(ModeAskForApproval, ModelConfig{Provider: "fake", Model: "fake"}),
			Checkpoint: ToolCheckpoint{
				Pending: []tool.Call{
					{ID: "c0", Name: "echo", Arguments: json.RawMessage(`{}`)},
					{ID: "c1", Name: "ping", Arguments: json.RawMessage(`{}`)},
				},
			},
		},
		Job: StepJob{RunID: "run-1", StepIndex: 2, Phase: PhaseLLMResult},
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload ApprovalRequiredPayload
	if err := json.Unmarshal(got.Facts[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.ToolCalls) != 2 {
		t.Fatalf("tool_calls=%+v", payload.ToolCalls)
	}
}

type stubEcho struct{}

func (stubEcho) Definition() tool.Definition {
	return tool.Definition{Name: "echo", Prompt: "echo", Version: "1"}
}

func (stubEcho) Execute(_ context.Context, input tool.Input) (tool.Result, error) {
	return tool.Result{CallID: input.Call.ID, Name: "echo", Success: true, Output: json.RawMessage(`{}`)}, nil
}

func TestEngineFinishAndCancel(t *testing.T) {
	engine, _, _ := testEngine(t)
	got, err := engine.Step(context.Background(), StepInput{
		State: AgentState{RunID: "run-1", CancelRequested: true},
		Job:   StepJob{RunID: "run-1", StepIndex: 1, Phase: PhaseUserInput},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunCancelled || got.Next != nil {
		t.Fatalf("got status=%s next=%v", got.State.Status, got.Next)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err = engine.Step(ctx, StepInput{
		State: AgentState{RunID: "run-1", Config: DefaultRunConfig(ModeAutoApprove, ModelConfig{Provider: "fake", Model: "fake"})},
		Job:   StepJob{RunID: "run-1", StepIndex: 1, Phase: PhaseUserInput},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunCancelled {
		t.Fatalf("canceled ctx status=%s", got.State.Status)
	}
}

func TestEngineNilAndMaxTurns(t *testing.T) {
	var engine *Engine
	got, err := engine.Step(context.Background(), StepInput{State: AgentState{RunID: "run-1"}})
	if err != nil || got.State.RunID != "run-1" {
		t.Fatalf("nil engine: %+v %v", got, err)
	}

	e, _, _ := testEngine(t)
	state := AgentState{
		RunID: "run-1",
		Config: RunConfigSnapshot{
			Mode:   ModeAutoApprove,
			Model:  ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(FakeOptions{Turns: []FakeTurn{{Text: "x"}}})},
			Limits: RunLimits{MaxTurns: 1},
		},
	}
	got, err = e.Step(context.Background(), StepInput{
		State:   state,
		Job:     StepJob{RunID: "run-1", StepIndex: 1, Phase: PhaseUserInput},
		History: History{Turn: Turn{Number: 2}, Run: Run{ID: "run-1", Config: state.Config}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.StopReason == nil || *got.State.StopReason != StopMaxTurns {
		t.Fatalf("want max_turns, got %+v", got.State.StopReason)
	}
}

func TestEngineHumanApprovedUsesCheckpoint(t *testing.T) {
	engine, _, _ := testEngine(t)
	state := AgentState{
		SessionID: "sess-1",
		RunID:     "run-1",
		Config:    DefaultRunConfig(ModeAskForApproval, ModelConfig{Provider: "fake", Model: "fake"}),
		Checkpoint: ToolCheckpoint{
			Pending:  []tool.Call{{ID: "c1", Name: "ping", Arguments: json.RawMessage(`{}`)}},
			Approved: []string{"c1"},
		},
	}
	got, err := engine.Step(context.Background(), StepInput{
		State: state,
		Job:   StepJob{RunID: "run-1", StepIndex: 3, Phase: PhaseHumanApproved},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunExecutingTools {
		t.Fatalf("status=%s", got.State.Status)
	}
}

func TestEngineCallLLMFail(t *testing.T) {
	engine, _, _ := testEngine(t)
	opts := FakeOptions{FailTimes: 1, Turns: []FakeTurn{{Text: "nope"}}}
	_, err := engine.Step(context.Background(), StepInput{
		State: AgentState{
			RunID:  "run-1",
			Config: DefaultRunConfig(ModeAutoApprove, ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(opts)}),
		},
		Job:     StepJob{RunID: "run-1", StepIndex: 1, Phase: PhaseUserInput},
		History: fakeHistory("run-1", opts),
	})
	if err == nil {
		t.Fatal("expected fake model failure")
	}
}

func TestNewEngineNilBrain(t *testing.T) {
	engine := NewEngine(nil, nil, nil)
	if engine.brain == nil {
		t.Fatal("expected default brain")
	}
}

func TestEngineFinishPayloadsAndCompress(t *testing.T) {
	engine, _, _ := testEngine(t)
	got, err := engine.finish(context.Background(), StepInput{
		State: AgentState{RunID: "run-1", CancelRequested: true},
		Job:   StepJob{},
	}, Instruction{Type: InstructionFinish, Payload: []byte(`{"status":"completed","reason":"completed"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunCancelled {
		t.Fatalf("cancel override status=%s", got.State.Status)
	}

	got, err = engine.finish(context.Background(), StepInput{
		State: AgentState{RunID: "run-1"},
		Job:   StepJob{StepIndex: 0},
	}, Instruction{Type: InstructionFinish, Payload: []byte(`{"status":"","reason":""}`)})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunCompleted || got.State.StopReason == nil || *got.State.StopReason != StopCompleted {
		t.Fatalf("empty payload %+v", got.State)
	}

	got, err = engine.Step(context.Background(), StepInput{
		State: AgentState{RunID: "run-1"},
		Job:   StepJob{RunID: "run-1", StepIndex: 1, Phase: PhaseHumanAbort},
	})
	if err != nil || got.State.Status != RunCancelled {
		t.Fatalf("human abort %+v %v", got.State.Status, err)
	}
}

func TestEngineCallToolsBatchPayloadAndErrors(t *testing.T) {
	engine, _, _ := testEngine(t)
	payload := MarshalPayload(CallToolsBatchPayload{
		Calls: []tool.Call{{ID: "c9", Name: "ping", Arguments: json.RawMessage(`{}`)}},
	})
	got, err := engine.callToolsBatch(context.Background(), StepInput{
		State: AgentState{
			SessionID: "sess-1",
			RunID:     "run-1",
			Config:    DefaultRunConfig(ModeAutoApprove, ModelConfig{Provider: "fake", Model: "fake"}),
		},
		Job: StepJob{RunID: "run-1", StepIndex: 2},
	}, Instruction{Type: InstructionCallToolsBatch, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunExecutingTools {
		t.Fatalf("status=%s", got.State.Status)
	}

	denied, err := engine.callToolsBatch(context.Background(), StepInput{
		State: AgentState{
			RunID:  "run-1",
			Config: DefaultRunConfig(ModeAskForApproval, ModelConfig{Provider: "fake", Model: "fake"}),
			Checkpoint: ToolCheckpoint{
				Pending: []tool.Call{{ID: "c1", Name: "ping", Arguments: json.RawMessage(`{}`)}},
				Denied:  []string{"c1"},
			},
		},
		Job: StepJob{StepIndex: 2},
	}, Instruction{Type: InstructionCallToolsBatch})
	if err != nil {
		t.Fatal(err)
	}
	if denied.Next == nil || denied.Next.Phase != PhaseToolsBatchResult {
		t.Fatalf("denied should still finish batch: %+v", denied.Next)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err = engine.callToolsBatch(ctx, StepInput{
		State: AgentState{RunID: "run-1"},
		Job:   StepJob{StepIndex: 1},
	}, Instruction{})
	if err != nil || got.State.Status != RunCancelled {
		t.Fatalf("canceled tools %+v %v", got.State.Status, err)
	}

	bare := NewEngine(&Brain{}, nil, nil)
	_, err = bare.callToolsBatch(context.Background(), StepInput{
		State: AgentState{
			RunID: "run-1",
			Checkpoint: ToolCheckpoint{
				Pending: []tool.Call{{ID: "c1", Name: "ping"}},
			},
		},
		Job: StepJob{StepIndex: 1},
	}, Instruction{})
	if err == nil {
		t.Fatal("expected dispatch error without registry")
	}
}

func TestEngineCallLLMHangCancelAndCompact(t *testing.T) {
	engine, _, _ := testEngine(t)
	opts := FakeOptions{Hang: true, Turns: []FakeTurn{{Text: "late"}}}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	got, err := engine.Step(ctx, StepInput{
		State: AgentState{
			SessionID: "sess-1",
			RunID:     "run-1",
			Config:    DefaultRunConfig(ModeAutoApprove, ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(opts)}),
		},
		Job:     StepJob{RunID: "run-1", StepIndex: 1, Phase: PhaseUserInput},
		History: fakeHistory("run-1", opts),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunCancelled {
		t.Fatalf("hang cancel status=%s", got.State.Status)
	}

	compactOpts := FakeOptions{Turns: []FakeTurn{{Text: "sum"}}, CompactSummary: "earlier"}
	cfg := DefaultRunConfig(ModeAutoApprove, ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(compactOpts)})
	cfg.Limits.MaxInputTokens = 1
	got, err = engine.callLLM(context.Background(), StepInput{
		State: AgentState{SessionID: "sess-1", RunID: "run-1", Config: cfg},
		Job:   StepJob{RunID: "run-1", StepIndex: 1},
		History: History{
			Run:      Run{ID: "run-1", SessionID: "sess-1", Config: cfg},
			Messages: []Message{{Role: RoleUser, Content: EncodeText(strings.Repeat("word ", 50))}},
			Prompt:   "p",
		},
	}, Instruction{Type: InstructionCallLLM})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunRunningLLM {
		t.Fatalf("compact llm status=%s", got.State.Status)
	}
}

type countingGate struct {
	slots chan struct{}
	cur   atomic.Int32
	max   atomic.Int32
}

func newCountingGate(n int) *countingGate {
	return &countingGate{slots: make(chan struct{}, n)}
}

func (g *countingGate) Acquire(ctx context.Context) error {
	select {
	case g.slots <- struct{}{}:
		n := g.cur.Add(1)
		for {
			old := g.max.Load()
			if n <= old || g.max.CompareAndSwap(old, n) {
				break
			}
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *countingGate) Release() {
	g.cur.Add(-1)
	select {
	case <-g.slots:
	default:
	}
}

func TestEngineLLMGateLimitsConcurrency(t *testing.T) {
	engine, _, _ := testEngine(t)
	gate := newCountingGate(1)
	engine.SetGates(gate, nil)
	opts := FakeOptions{Hang: true, Turns: []FakeTurn{{Text: "late"}}}
	input := func(id string) StepInput {
		return StepInput{
			State: AgentState{
				SessionID: "sess-1",
				RunID:     id,
				Config:    DefaultRunConfig(ModeAutoApprove, ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(opts)}),
			},
			Job:     StepJob{RunID: id, StepIndex: 1, Phase: PhaseUserInput},
			History: fakeHistory(id, opts),
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		id := fmt.Sprintf("run-%d", i)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			_, _ = engine.Step(ctx, input(id))
		}()
	}
	time.Sleep(40 * time.Millisecond)
	if gate.max.Load() != 1 {
		t.Fatalf("llm concurrency=%d want 1", gate.max.Load())
	}
	wg.Wait()
}

type slowTool struct {
	cur *atomic.Int32
	max *atomic.Int32
}

func (slowTool) Definition() tool.Definition {
	return tool.Definition{Name: "slow", Prompt: "slow", Version: "1"}
}

func (s slowTool) Execute(_ context.Context, input tool.Input) (tool.Result, error) {
	n := s.cur.Add(1)
	for {
		old := s.max.Load()
		if n <= old || s.max.CompareAndSwap(old, n) {
			break
		}
	}
	time.Sleep(30 * time.Millisecond)
	s.cur.Add(-1)
	return tool.Result{CallID: input.Call.ID, Name: "slow", Success: true, Output: json.RawMessage(`{}`)}, nil
}

func TestEngineToolGateLimitsConcurrency(t *testing.T) {
	facts := &memFacts{}
	reg := tool.NewRegistry()
	cur := &atomic.Int32{}
	max := &atomic.Int32{}
	if err := reg.Register(slowTool{cur: cur, max: max}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(&Brain{}, facts, reg)
	engine.SetGates(nil, NewSlotLimiter(1))
	cfg := DefaultRunConfig(ModeAutoApprove, ModelConfig{Provider: "fake", Model: "fake"})
	cfg.ToolExecutionMode = tool.ExecutionParallel
	cfg.Limits.MaxParallelTools = 4
	_, err := engine.callToolsBatch(context.Background(), StepInput{
		State: AgentState{
			SessionID: "sess-1",
			RunID:     "run-1",
			Config:    cfg,
			Checkpoint: ToolCheckpoint{
				Pending: []tool.Call{
					{ID: "c1", Name: "slow"},
					{ID: "c2", Name: "slow"},
					{ID: "c3", Name: "slow"},
				},
			},
		},
		Job: StepJob{RunID: "run-1", StepIndex: 1},
	}, Instruction{Type: InstructionCallToolsBatch})
	if err != nil {
		t.Fatal(err)
	}
	if max.Load() != 1 {
		t.Fatalf("tool concurrency=%d want 1", max.Load())
	}
}

type failGate struct{}

// Acquire 始终返回 Canceled，用于模拟占槽失败。
func (failGate) Acquire(context.Context) error { return context.Canceled }

// Release 空实现，满足 Gate 接口。
func (failGate) Release() {}

// TestEngineLLMGateAcquireCancelAndEmptyText 覆盖 LLM 占槽失败收束，以及空文本回复补 StepIndex。
func TestEngineLLMGateAcquireCancelAndEmptyText(t *testing.T) {
	engine, _, _ := testEngine(t)
	engine.SetGates(failGate{}, nil)
	got, err := engine.callLLM(context.Background(), StepInput{
		State: AgentState{
			SessionID: "sess-1",
			RunID:     "run-1",
			Config:    DefaultRunConfig(ModeAutoApprove, ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(FakeOptions{Turns: []FakeTurn{{Text: ""}}})}),
		},
		Job:     StepJob{RunID: "run-1", StepIndex: 0, Phase: PhaseUserInput},
		History: History{Messages: []Message{{Role: RoleUser, Content: EncodeText("hi")}}, Prompt: "p"},
	}, Instruction{Type: InstructionCallLLM})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunCancelled {
		t.Fatalf("gate cancel status=%s", got.State.Status)
	}

	engine.SetGates(nil, nil)
	got, err = engine.callLLM(context.Background(), StepInput{
		State: AgentState{
			SessionID: "sess-1",
			RunID:     "run-1",
			Config:    DefaultRunConfig(ModeAutoApprove, ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(FakeOptions{Turns: []FakeTurn{{Text: ""}}})}),
		},
		Job:     StepJob{RunID: "run-1", StepIndex: 0, Phase: PhaseUserInput},
		History: fakeHistory("run-1", FakeOptions{Turns: []FakeTurn{{Text: ""}}}),
	}, Instruction{Type: InstructionCallLLM})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunRunningLLM || got.State.StepIndex != 1 {
		t.Fatalf("empty text %+v", got.State)
	}
}
