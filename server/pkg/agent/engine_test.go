package agent

import (
	"codedock/pkg/agent/tool"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"codedock/pkg/agent/seam"
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
		Permission: tool.Permission{Effect: tool.EffectAsk},
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
	cfg := DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(opts)})
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

func TestEngineRequestSeamRewritesChat(t *testing.T) {
	var gotSystem string
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Plugin")
		body, _ := io.ReadAll(r.Body)
		var req openaiChatRequest
		_ = json.Unmarshal(body, &req)
		if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
			gotSystem = req.Messages[0].Content
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	engine, _, _ := testEngine(t)
	engine.SetDispatcher(seam.Func(func(_ context.Context, ev seam.Envelope) (seam.Envelope, error) {
		switch ev.Type {
		case seam.TypeRequest:
			var payload RequestPayload
			_ = json.Unmarshal(ev.Payload, &payload)
			payload.SystemPrompt = "from-plugin"
			ev.Payload = MarshalPayload(payload)
		case seam.TypeStream:
			var payload StreamPayload
			_ = json.Unmarshal(ev.Payload, &payload)
			if payload.Headers == nil {
				payload.Headers = map[string]string{}
			}
			payload.Headers["X-Plugin"] = "1"
			ev.Payload = MarshalPayload(payload)
		}
		return ev, nil
	}))
	opts, _ := json.Marshal(map[string]string{"api_key": "sk-test", "base_url": srv.URL})
	cfg := DefaultRunConfig(WorkAgent, ModelConfig{Provider: "openai", Model: "gpt-test", Options: opts})
	hist := fakeHistory("run-1", FakeOptions{})
	hist.Run.Config = cfg
	got, err := engine.Step(context.Background(), StepInput{
		State:   AgentState{SessionID: "sess-1", RunID: "run-1", Config: cfg},
		Job:     StepJob{RunID: "run-1", StepIndex: 1, Phase: PhaseUserInput},
		History: hist,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunRunningLLM {
		t.Fatalf("status=%s", got.State.Status)
	}
	if gotSystem != "from-plugin" {
		t.Fatalf("system=%q", gotSystem)
	}
	if gotHeader != "1" {
		t.Fatalf("header=%q", gotHeader)
	}
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
		Config:    DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(FakeOptions{Turns: []FakeTurn{{Text: "hello"}}})}),
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
		Config:    DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(opts)}),
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
		Config:    DefaultRunConfig(WorkAgent, ModelConfig{Provider: "fake", Model: "fake"}),
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

func TestEngineAutoReviewApprovesWithoutApprovalEvent(t *testing.T) {
	engine, facts, _ := testEngine(t)
	cfg := DefaultRunConfig(WorkAgent, ModelConfig{
		Provider: "fake",
		Model:    "fake",
		Options: mustRaw(FakeOptions{Review: &FakeReview{
			Decisions: []ApprovalDecision{{ToolCallID: "c1", Status: ApprovalApproved, Reason: "ok"}},
		}}),
	})
	cfg.Approval = ApprovalAuto
	got, err := engine.Step(context.Background(), StepInput{
		State: AgentState{
			SessionID: "sess-1",
			RunID:     "run-1",
			Config:    cfg,
			Checkpoint: ToolCheckpoint{
				Pending: []tool.Call{{ID: "c1", Name: "ping", Arguments: json.RawMessage(`{}`)}},
			},
		},
		Job: StepJob{RunID: "run-1", StepIndex: 2, Phase: PhaseLLMResult},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunExecutingTools {
		t.Fatalf("status=%s", got.State.Status)
	}
	for _, fact := range append(append([]Fact{}, facts.facts...), got.Facts...) {
		if fact.Type == EventApprovalRequired {
			t.Fatalf("auto review should not emit approval_required: %+v", fact)
		}
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != RoleTool {
		t.Fatalf("tool messages=%+v", got.Messages)
	}
}

func TestEngineAutoReviewEscalateEmitsApprovalRequired(t *testing.T) {
	engine, facts, _ := testEngine(t)
	cfg := DefaultRunConfig(WorkAgent, ModelConfig{Provider: "fake", Model: "fake"})
	cfg.Approval = ApprovalAuto
	got, err := engine.Step(context.Background(), StepInput{
		State: AgentState{
			SessionID: "sess-1",
			RunID:     "run-1",
			Config:    cfg,
			Checkpoint: ToolCheckpoint{
				Pending: []tool.Call{{ID: "c1", Name: "ping", Arguments: json.RawMessage(`{}`)}},
			},
		},
		Job: StepJob{RunID: "run-1", StepIndex: 2, Phase: PhaseLLMResult},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Status != RunWaitingApproval {
		t.Fatalf("status=%s", got.State.Status)
	}
	if len(got.Facts) != 1 || got.Facts[0].Type != EventApprovalRequired {
		t.Fatalf("facts=%+v", got.Facts)
	}
	for _, fact := range facts.facts {
		if fact.Type == EventToolCallStarted || fact.Type == EventToolExecutionStarted {
			t.Fatalf("escalated auto review should not emit tool events yet: %+v", fact)
		}
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
			Config:    DefaultRunConfig(WorkAgent, ModelConfig{Provider: "fake", Model: "fake"}),
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
		State: AgentState{RunID: "run-1", Config: DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake"})},
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
			Mode:   WorkAgent,
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
		Config:    DefaultRunConfig(WorkAgent, ModelConfig{Provider: "fake", Model: "fake"}),
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
			Config: DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(opts)}),
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
			Config:    DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake"}),
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
			Config: DefaultRunConfig(WorkAgent, ModelConfig{Provider: "fake", Model: "fake"}),
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
			Config:    DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(opts)}),
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
	cfg := DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(compactOpts)})
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
				Config:    DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(opts)}),
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
	cfg := DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake"})
	cfg.Profile.Tools.Names = []string{"slow"}
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
			Config:    DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(FakeOptions{Turns: []FakeTurn{{Text: ""}}})}),
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
			Config:    DefaultYoloConfig(ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(FakeOptions{Turns: []FakeTurn{{Text: ""}}})}),
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

func TestRetryHelpers(t *testing.T) {
	if Retryable(nil) || Retryable(context.Canceled) || Retryable(ErrNonRetryable) || Retryable(ErrPermissionDenied) || Retryable(ErrInvalidArguments) || Retryable(ErrApprovalRequired) {
		t.Fatal("non-retryable")
	}
	if !Retryable(errors.New("temp")) {
		t.Fatal("retryable")
	}
	cfg := RetryConfig{MaxAttempts: 3, InitialBackoff: time.Millisecond, MaxBackoff: 10 * time.Millisecond, Multiplier: 2}
	if Backoff(cfg, 0) <= 0 || Backoff(RetryConfig{InitialBackoff: 0, Multiplier: 0}, 1) <= 0 {
		t.Fatal("backoff")
	}
	if Backoff(cfg, 8) != 10*time.Millisecond {
		t.Fatal("max backoff")
	}
	if ShouldRetry(cfg, 3, errors.New("x")) || !ShouldRetry(RetryConfig{}, 0, errors.New("x")) {
		t.Fatal("should retry")
	}
}

func TestContentAndPayloadHelpers(t *testing.T) {
	if DecodeText(nil) != "" || DecodeText(json.RawMessage(`"plain"`)) != "plain" || DecodeText(json.RawMessage(`not-json`)) != "not-json" {
		t.Fatal("decode")
	}
	if got := EncodeToolError("", ""); !strings.Contains(string(got), "tool failed") {
		t.Fatalf("%s", got)
	}
	if got := EncodeToolResult("c1", nil); !strings.Contains(string(got), "null") {
		t.Fatalf("%s", got)
	}
	if string(MarshalPayload(nil)) != "{}" {
		t.Fatal(MarshalPayload(nil))
	}
	got := CompleteToolResults(nil)
	if got != nil && len(got) != 0 {
		t.Fatalf("%+v", got)
	}
	msgs := CompleteToolResults([]Message{
		{Role: RoleAssistant, Content: EncodeText("no tools")},
		{Role: RoleSystem, Content: EncodeText("sys")},
		{Role: RoleAssistant, Content: EncodeText("call"), ToolCalls: []tool.Call{{ID: "c1", Name: "ping"}}},
		{Role: RoleTool, Content: EncodeToolResult("c1", json.RawMessage(`{"ok":true}`))},
	})
	if len(msgs) < 4 {
		t.Fatalf("%d", len(msgs))
	}
}

func TestDefaultsContextPromptStatus(t *testing.T) {
	cfg := DefaultRunConfig("", ModelConfig{})
	if cfg.Mode != WorkAgent || cfg.Approval != ApprovalManual || cfg.Model.Provider != "fake" || cfg.Model.Model != "fake" {
		t.Fatalf("%+v", cfg)
	}
	if ParseFakeOptions(nil).Turns[0].Text != "ok" {
		t.Fatal("empty opts")
	}
	if ParseFakeOptions(json.RawMessage(`not-json`)).Turns[0].Text != "ok" {
		t.Fatal("bad opts")
	}

	snap, err := Load(context.Background(), History{
		Run:           Run{SessionID: "s"},
		Checkpoint:    &CompactionCheckpoint{ID: "cp", Summary: "old", BaseEventSeq: 3},
		Messages:      []Message{{Role: RoleUser, Content: EncodeText("hi"), EventSeq: 2}},
		Prompt:        "sys",
		Tools:         []tool.Definition{{Name: "ping", Prompt: "p"}},
		MemoryIndexes: []string{"idx"},
	})
	if err != nil || snap.Summary == nil || snap.BaseEventSeq != 3 || snap.EstimatedTokens == 0 {
		t.Fatalf("%+v %v", snap, err)
	}
	need, err := NeedsCompaction(context.Background(), Compaction{Run: Run{Config: RunConfigSnapshot{Limits: RunLimits{MaxInputTokens: 0}}}, Snapshot: snap})
	if err != nil || need {
		t.Fatal(need, err)
	}
	need, err = NeedsCompaction(context.Background(), Compaction{Run: Run{Config: RunConfigSnapshot{Limits: RunLimits{MaxInputTokens: 1}}}, Snapshot: ContextSnapshot{}})
	if err != nil || need {
		t.Fatal("empty snapshot should not exceed 1 if tokens=0? wait EstimateTokens of empty is 0")
	}
	compacted, err := CompactIfNeeded(context.Background(), Compaction{
		Run:      Run{Config: RunConfigSnapshot{Model: ModelConfig{Provider: "fake", Options: mustRaw(FakeOptions{CompactSummary: "sum"})}, Limits: RunLimits{MaxInputTokens: 1}}},
		Snapshot: ContextSnapshot{Messages: []Message{{Role: RoleUser, Content: EncodeText(strings.Repeat("x", 20)), EventSeq: 9}}},
	})
	if err != nil || compacted.Summary == nil || compacted.Summary.Content != "sum" || len(compacted.Messages) != 0 {
		t.Fatalf("%+v %v", compacted, err)
	}
	fallback, err := CompactIfNeeded(context.Background(), Compaction{
		Run:      Run{Config: RunConfigSnapshot{Model: ModelConfig{Provider: "fake"}, Limits: RunLimits{MaxInputTokens: 1}}},
		Snapshot: ContextSnapshot{Messages: []Message{{Role: RoleUser, Content: EncodeText(strings.Repeat("keep me ", 20)), EventSeq: 4}}},
	})
	if err != nil || fallback.Summary == nil || !strings.Contains(fallback.Summary.Content, "keep me") {
		t.Fatalf("%+v %v", fallback, err)
	}
	emptySum, err := CompactIfNeeded(context.Background(), Compaction{
		Run:      Run{Config: RunConfigSnapshot{Model: ModelConfig{Provider: "fake"}, Limits: RunLimits{MaxInputTokens: 1}}},
		Snapshot: ContextSnapshot{EstimatedTokens: 10, Messages: []Message{{Role: RoleUser}}},
	})
	if err != nil || emptySum.Summary == nil || emptySum.Summary.Content == "" {
		t.Fatalf("%+v %v", emptySum, err)
	}
	if got, err := CompactIndex(context.Background(), ModelConfig{Provider: "fake"}, "  "); err != nil || got != "" {
		t.Fatal(got, err)
	}
	if got, err := CompactIndex(context.Background(), ModelConfig{Provider: "fake", Options: mustRaw(FakeOptions{IndexCompactSummary: "short"})}, "long"); err != nil || got != "short" {
		t.Fatal(got, err)
	}

	chat, err := Build(context.Background(), Prompt{Run: Run{Config: DefaultRunConfig(WorkAgent, ModelConfig{})}, Context: ContextSnapshot{}})
	if err != nil || chat.SystemPrompt == "" {
		t.Fatal(chat, err)
	}
	if ComposeSystemPrompt("", []tool.Definition{{Name: "ping", Prompt: "p"}}) == "" {
		t.Fatal("tools only")
	}
	if !NeedsUserRecover(RunQueued, false) || NeedsUserRecover(RunCompleted, false) || NeedsUserRecover(RunQueued, true) {
		t.Fatal("recover")
	}
	if lastMessageSeq(nil) != 0 {
		t.Fatal("seq")
	}
}

func TestReviewCoverAndParse(t *testing.T) {
	calls := []ApprovalToolCall{{ID: "c1"}, {ID: "c2"}}
	if _, ok := parseReviewContent("", calls); ok {
		t.Fatal("empty")
	}
	if coverReview([]ApprovalDecision{{ToolCallID: "c1", Status: "weird"}, {ToolCallID: "c2", Status: ApprovalDenied}}, calls) {
		t.Fatal("invalid status")
	}
	if coverReview([]ApprovalDecision{{ToolCallID: "z", Status: ApprovalApproved}, {ToolCallID: "c2", Status: ApprovalDenied}}, calls) {
		t.Fatal("unknown")
	}
	if coverReview([]ApprovalDecision{{ToolCallID: "c1", Status: ApprovalApproved}, {ToolCallID: "c1", Status: ApprovalDenied}}, calls) {
		t.Fatal("dup")
	}
	incomplete, err := Review(context.Background(), ModelConfig{Provider: "fake", Options: mustRaw(FakeOptions{Review: &FakeReview{
		Decisions: []ApprovalDecision{{ToolCallID: "c1", Status: ApprovalApproved}},
	}})}, calls)
	if err != nil || !incomplete.Escalate {
		t.Fatalf("%+v %v", incomplete, err)
	}
}

func TestFakeTurnsFollowLastUser(t *testing.T) {
	opts := FakeOptions{Turns: []FakeTurn{
		{ToolCalls: []FakeToolCall{{Name: "ping"}}},
		{Text: "second"},
	}}
	cfg := ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(opts)}
	prior := []Message{
		{Role: RoleUser, Content: EncodeText("old")},
		{Role: RoleAssistant, Content: EncodeText("old-a")},
		{Role: RoleAssistant, Content: EncodeText("old-b")},
		{Role: RoleUser, Content: EncodeText("now")},
	}
	first, err := Stream(context.Background(), Chat{Model: cfg, TurnID: "t1", Messages: prior})
	if err != nil {
		t.Fatal(err)
	}
	got, err := first.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "ping" {
		t.Fatalf("first turn should be ping: %+v", got)
	}
	second, err := Stream(context.Background(), Chat{
		Model:  cfg,
		TurnID: "t2",
		Messages: append(append([]Message{}, prior...), Message{
			Role: RoleAssistant, Content: EncodeText(""), ToolCalls: []tool.Call{{ID: "c1", Name: "ping"}},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err = second.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if DecodeText(got.Message.Content) != "second" || len(got.ToolCalls) != 0 {
		t.Fatalf("second turn should be text: %+v", got)
	}
}

func TestStreamUnsupportedAndToOpenAI(t *testing.T) {
	if _, err := Stream(context.Background(), Chat{Model: ModelConfig{Provider: "other"}}); err == nil {
		t.Fatal("unsupported")
	}
	applyOutputLimit(nil, "x", 1)
	applyOutputLimit(&openaiChatRequest{}, "x", 0)
	msgs := toOpenAIMessages(Chat{
		SystemPrompt: "sys",
		Messages: []Message{
			{Role: RoleAssistant, Content: EncodeText("a"), ToolCalls: []tool.Call{{ID: "c1", Name: "ping", Arguments: json.RawMessage(`{}`)}}},
			{Role: RoleTool, Content: EncodeToolResult("c1", json.RawMessage(`{"ok":true}`))},
			{Role: RoleSystem, Content: EncodeText("s")},
			{Role: RoleUser, Content: EncodeText("u")},
			{Role: RoleTool, Content: EncodeText("plain tool")},
			{Role: RoleDeveloper, Content: EncodeText("mode")},
		},
	})
	if len(msgs) != 7 || msgs[0].Role != "system" || msgs[0].Content != "sys" || msgs[1].Role != "developer" || msgs[1].Content != "mode" {
		t.Fatalf("%+v", msgs)
	}
	tools := toOpenAITools([]tool.Definition{{Name: "ping", Prompt: "p"}, {Name: "x"}})
	if len(tools) != 2 || string(tools[1].Function.Parameters) == "" {
		t.Fatalf("%+v", tools)
	}
	if toOpenAITools(nil) != nil {
		t.Fatal("empty tools")
	}
}
