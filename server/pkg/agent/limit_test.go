package agent

import (
	"context"
	"testing"
)

// TestNewSlotLimiterAcquireRelease 覆盖不限流、取消占槽和重复 Release。
func TestNewSlotLimiterAcquireRelease(t *testing.T) {
	if NewSlotLimiter(0) != nil || NewSlotLimiter(-1) != nil {
		t.Fatal("n<=0 should be unlimited")
	}

	var nilLim *slotLimiter
	if err := nilLim.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	nilLim.Release()

	g := NewSlotLimiter(1)
	if err := g.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.Acquire(ctx); err == nil {
		t.Fatal("canceled acquire")
	}
	g.Release()
	g.Release()
}

// TestSetGatesNilEngine 覆盖空 Engine 设闸、空 runID 写 Fact 与空串指针。
func TestSetGatesNilEngine(t *testing.T) {
	var engine *Engine
	engine.SetGates(NewSlotLimiter(1), NewSlotLimiter(1))
	e := NewEngine(nil, nil, nil)
	e.SetGates(nil, nil)
	if err := e.appendFact(context.Background(), "", Fact{Type: EventAssistantDelta}); err != nil {
		t.Fatal(err)
	}
	if ptrValue("") != nil {
		t.Fatal("empty ptr")
	}
}
