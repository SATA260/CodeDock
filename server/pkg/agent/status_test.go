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
	if err := CanTransition(RunQueued, RunQueued); err != nil {
		t.Fatal(err)
	}
	if TerminalEvent(RunFailed) != EventRunFailed {
		t.Fatal("failed event")
	}
	if TerminalEvent(RunCancelled) != EventRunCancelled {
		t.Fatal("cancelled event")
	}
	if TerminalEvent(RunCompleted) != EventRunCompleted {
		t.Fatal("completed event")
	}
}

// TestNeedsUserRecover 校验只有中断的执行态才需要用户恢复。
func TestNeedsUserRecover(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status RunStatus
		busy   bool
		want   bool
	}{
		{RunQueued, false, true},
		{RunLoadingContext, false, true},
		{RunRunningLLM, false, true},
		{RunExecutingTools, false, true},
		{RunRunningLLM, true, false},
		{RunWaitingApproval, false, false},
		{RunCancelling, false, false},
		{RunCompleted, false, false},
		{RunFailed, false, false},
		{RunCancelled, false, false},
	}
	if IsExecuting(RunRunningLLM) != true || IsExecuting(RunWaitingApproval) || IsExecuting(RunCompleted) {
		t.Fatal("executing should exclude approval and terminal")
	}
	for _, tc := range cases {
		if got := NeedsUserRecover(tc.status, tc.busy); got != tc.want {
			t.Fatalf("NeedsUserRecover(%s, busy=%v)=%v want %v", tc.status, tc.busy, got, tc.want)
		}
	}
}

// TestCountTokens 校验 UTF-8 字节 / 4 的估算。
func TestCountTokens(t *testing.T) {
	t.Parallel()
	if got := CountTokens("abcd"); got != 1 {
		t.Fatalf("CountTokens(abcd)=%d want 1", got)
	}
}
