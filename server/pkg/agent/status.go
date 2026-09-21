package agent

import "fmt"

var allowedTransitions = map[RunStatus][]RunStatus{
	RunQueued:          {RunLoadingContext, RunCancelling, RunCancelled, RunFailed},
	RunLoadingContext:  {RunRunningLLM, RunFailed, RunCancelling, RunCancelled},
	RunRunningLLM:      {RunExecutingTools, RunVerifying, RunCompleted, RunFailed, RunCancelling, RunCancelled},
	RunExecutingTools:  {RunLoadingContext, RunWaitingApproval, RunCompleted, RunFailed, RunCancelling, RunCancelled},
	RunWaitingApproval: {RunExecutingTools, RunRunningLLM, RunCompleted, RunFailed, RunCancelling, RunCancelled},
	RunVerifying:       {RunRunningLLM, RunEvaluating, RunWaitingApproval, RunCompleted, RunFailed, RunCancelling, RunCancelled},
	RunEvaluating:      {RunRunningLLM, RunCompleted, RunWaitingApproval, RunFailed, RunCancelling, RunCancelled},
	RunCancelling:      {RunCancelled, RunFailed},
}

// CanTransition 校验 Run 状态迁移是否合法。
func CanTransition(from, next RunStatus) error {
	if from == next {
		return nil
	}
	if IsTerminal(from) {
		return fmt.Errorf("cannot transition from terminal status %q", from)
	}
	for _, allowed := range allowedTransitions[from] {
		if allowed == next {
			return nil
		}
	}
	return fmt.Errorf("invalid run transition %q -> %q", from, next)
}

// IsTerminal 判断 Run 是否已结束。
func IsTerminal(status RunStatus) bool {
	switch status {
	case RunCompleted, RunFailed, RunCancelled:
		return true
	default:
		return false
	}
}

// NeedsUserRecover 判断未完成的 Run 是否因执行中断而需要用户点恢复。
// Worker 仍在跑、等待审批、取消中或已终态时返回 false。
func NeedsUserRecover(status RunStatus, workerBusy bool) bool {
	if workerBusy || IsTerminal(status) || status == RunWaitingApproval || status == RunCancelling {
		return false
	}
	switch status {
	case RunQueued, RunLoadingContext, RunRunningLLM, RunExecutingTools, RunVerifying, RunEvaluating:
		return true
	default:
		return false
	}
}

// TerminalEvent 返回终态对应的事件类型。
func TerminalEvent(status RunStatus) EventType {
	switch status {
	case RunFailed:
		return EventRunFailed
	case RunCancelled:
		return EventRunCancelled
	default:
		return EventRunCompleted
	}
}
