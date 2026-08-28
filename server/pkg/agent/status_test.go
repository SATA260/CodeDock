package agent

import "testing"

// TestCanTransition 校验合法迁移与终态/审批非法迁移。
func TestCanTransition(t *testing.T) {
	t.Parallel()
	if err := CanTransition(RunQueued, RunLoadingContext); err != nil {
		t.Fatal(err)
	}
	if err := CanTransition(RunCompleted, RunQueued); err == nil {
		t.Fatal("expected terminal transition to fail")
	}
	if err := CanTransition(RunWaitingApproval, RunExecutingTools); err != nil {
		t.Fatal(err)
	}
	if err := CanTransition(RunWaitingApproval, RunLoadingContext); err == nil {
		t.Fatal("approval must resume tools, not reload")
	}
}

// TestCountTokens 校验 UTF-8 字节 / 4 的估算。
func TestCountTokens(t *testing.T) {
	t.Parallel()
	if got := CountTokens("abcd"); got != 1 {
		t.Fatalf("CountTokens(abcd)=%d want 1", got)
	}
}
