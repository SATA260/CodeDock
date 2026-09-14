package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	cderr "codedock/internal/errors"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/seam"
)

// stepJobKey 返回 StepJob 的唯一去重键：run_id + step_index。
func stepJobKey(job pkgagent.StepJob) string {
	return fmt.Sprintf("%s/%d", job.RunID, job.StepIndex)
}

const workerQueueSize = 64

// Worker 从执行总线领取 StepJob，按一步推进 Run。
type Worker struct {
	runtime   *Runtime
	jobs      chan pkgagent.StepJob
	mu        sync.Mutex
	cancels   map[string]context.CancelFunc // 运行中 Run 的取消函数
	done      map[string]chan struct{}      // 运行中 Run 的完成通知
	skipped   map[string]struct{}           // 已被取消但尚未被 goroutine 感知的 Run
	queued    map[string]struct{}           // 已入队但未开始执行的 StepJob
	submitErr error                         // 测试注入的下一次提交错误
}

// NewWorker 创建 Worker。
func NewWorker(runtime *Runtime) *Worker {
	return &Worker{
		runtime: runtime,
		jobs:    make(chan pkgagent.StepJob, workerQueueSize),
		cancels: make(map[string]context.CancelFunc),
		done:    make(map[string]chan struct{}),
		skipped: make(map[string]struct{}),
		queued:  make(map[string]struct{}),
	}
}

// Start 启动 Worker 循环：每个 StepJob 在独立 goroutine 中执行。
func (w *Worker) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case job := <-w.jobs:
				go w.execute(ctx, job)
			}
		}
	}()
}

// InjectSubmitError 让下一次 Submit 返回指定错误，随后恢复正常。仅用于测试。
func (w *Worker) InjectSubmitError(err error) {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.submitErr = err
	w.mu.Unlock()
}

// Submit 投递 StepJob；按 run_id + step_index 去重，队列满时直接返回错误。
func (w *Worker) Submit(_ context.Context, job pkgagent.StepJob) error {
	if w == nil {
		return nil
	}
	if job.RunID == "" {
		return cderr.Invalid("run id is required")
	}
	key := stepJobKey(job)
	w.mu.Lock()
	if err := w.submitErr; err != nil {
		w.submitErr = nil
		w.mu.Unlock()
		return err
	}
	if _, queued := w.queued[key]; queued {
		w.mu.Unlock()
		return nil
	}
	w.queued[key] = struct{}{}
	w.mu.Unlock()
	select {
	case w.jobs <- job:
		w.runtime.logger().Debug("worker submit", "run_id", job.RunID, "step_index", job.StepIndex, "phase", job.Phase)
		return nil
	default:
		w.mu.Lock()
		delete(w.queued, key)
		w.mu.Unlock()
		w.runtime.logger().Error("worker queue full", "run_id", job.RunID)
		return cderr.Unavailable("worker queue full")
	}
}

// Busy 判断本进程是否已在执行或已入队该 Run。
func (w *Worker) Busy(runID string) bool {
	if w == nil || runID == "" {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.cancels[runID]; ok {
		return true
	}
	prefix := runID + "/"
	for key := range w.queued {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// Cancel 取消指定 Run 当前运行中的步骤。
func (w *Worker) Cancel(runID string) {
	if w == nil || runID == "" {
		return
	}
	w.mu.Lock()
	w.skipped[runID] = struct{}{}
	cancel := w.cancels[runID]
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	w.runtime.logger().Info("worker cancel", "run_id", runID)
}

// CancelAndWait 取消指定 Run 运行中的步骤，并等待其彻底结束。
func (w *Worker) CancelAndWait(runID string) {
	w.Cancel(runID)
	w.mu.Lock()
	done := w.done[runID]
	w.mu.Unlock()
	if done != nil {
		<-done
	}
}

// execute 执行一步：先尝试领取，再加载 AgentState，交给 Engine 执行，最后提交结果。
// 逻辑：登记 cancel/done → TryClaimStep → 标 running → Load → 已取消则走 finish；Step 失败且非取消则 failed；成功则 CommitStep。
func (w *Worker) execute(parent context.Context, job pkgagent.StepJob) {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	runID := job.RunID

	w.mu.Lock()
	delete(w.queued, stepJobKey(job))
	w.cancels[runID] = cancel
	w.done[runID] = done
	w.mu.Unlock()

	w.runtime.logger().Info("worker execute start", "run_id", runID, "step_index", job.StepIndex, "phase", job.Phase)
	defer func() {
		cancel()
		w.mu.Lock()
		delete(w.cancels, runID)
		delete(w.done, runID)
		w.mu.Unlock()
		close(done)
		w.runtime.logger().Info("worker execute done", "run_id", runID)
	}()

	ok, err := w.runtime.TryClaimStep(ctx, job.RunID, job.StepIndex)
	if err != nil || !ok {
		return
	}
	defer w.runtime.releaseStep(job.RunID, job.StepIndex)
	w.runtime.markStepJobStatus(ctx, job.RunID, job.StepIndex, stepJobRunning)

	state, history, err := w.runtime.LoadAgentState(ctx, job.RunID)
	if err != nil {
		return
	}
	w.mu.Lock()
	_, skipped := w.skipped[runID]
	w.mu.Unlock()
	blocked := false
	if !skipped && !state.CancelRequested && job.Phase == pkgagent.PhaseUserInput {
		var err error
		blocked, history, err = w.applyPreStep(ctx, job, state, history)
		if err != nil {
			blocked = true
		}
	}
	if skipped || state.CancelRequested || blocked {
		if !pkgagent.IsTerminal(state.Status) {
			result, ferr := w.runtime.engine.Step(ctx, pkgagent.StepInput{
				State:   cancelState(state),
				Job:     job,
				History: history,
			})
			if ferr == nil {
				_ = w.runtime.CommitStep(ctx, job.RunID, result)
			}
		}
		w.mu.Lock()
		delete(w.skipped, runID)
		w.mu.Unlock()
		return
	}
	result, err := w.runtime.engine.Step(ctx, pkgagent.StepInput{State: state, Job: job, History: history})
	if err != nil {
		if ctx.Err() != nil || state.CancelRequested {
			result, _ = w.runtime.engine.Step(context.Background(), pkgagent.StepInput{
				State:   cancelState(state),
				Job:     job,
				History: history,
			})
			_ = w.runtime.CommitStep(ctx, job.RunID, result)
			return
		}
		failed := failState(state, err)
		_ = w.runtime.CommitStep(ctx, job.RunID, pkgagent.StepResult{
			State: failed,
			Facts: []pkgagent.Fact{{
				Type:    pkgagent.EventRunFailed,
				Payload: pkgagent.MarshalPayload(pkgagent.RunTerminalPayload{Status: pkgagent.RunFailed, StopReason: failed.StopReason}),
			}},
		})
		return
	}
	_ = w.runtime.CommitStep(ctx, job.RunID, result)
}

// cancelState 复制状态并标上 CancelRequested，供取消路径走 finish。
func cancelState(state pkgagent.AgentState) pkgagent.AgentState {
	state.CancelRequested = true
	return state
}

// failState 把状态收成 failed，供 Step 出错且并非取消时落终态。
func failState(state pkgagent.AgentState, err error) pkgagent.AgentState {
	reason := pkgagent.StopModelError
	now := timeNow()
	state.Status = pkgagent.RunFailed
	state.StopReason = &reason
	state.FinishedAt = &now
	if state.StepIndex <= 0 {
		state.StepIndex = 1
	}
	_ = err
	return state
}

// timeNow 返回 UTC 当前时间，便于测试替换。
func timeNow() time.Time {
	return time.Now().UTC()
}

// applyPreStep 在首拍把系统提示和隐藏消息递给插件；换向或出错则取消本轮。
// 本轮已有 overlay 时不再喊插件，避免恢复或重入第一步时把 Hidden 再追加一遍。
func (w *Worker) applyPreStep(ctx context.Context, job pkgagent.StepJob, state pkgagent.AgentState, history pkgagent.History) (bool, pkgagent.History, error) {
	if w == nil || w.runtime == nil || w.runtime.Dispatcher() == nil {
		return false, history, nil
	}
	if w.runtime.hasOverlay(ctx, job.RunID) {
		return false, history, nil
	}
	ev, err := seam.Dispatch(ctx, w.runtime.Dispatcher(), seam.Envelope{
		Type:      seam.TypePreStep,
		SessionID: state.SessionID,
		RunID:     job.RunID,
		Payload: pkgagent.MarshalPayload(pkgagent.PreStepPayload{
			SystemPrompt: history.Prompt,
			Hidden:       history.Hidden,
		}),
	})
	if err != nil {
		w.runtime.logger().Info("pre-step blocked", "run_id", job.RunID, "error", err)
		return true, history, err
	}
	if ev.Type == seam.TypeRunBlocked {
		w.runtime.logger().Info("pre-step blocked", "run_id", job.RunID, "type", ev.Type)
		return true, history, nil
	}
	if ev.Type != seam.TypePreStep || len(ev.Payload) == 0 {
		return false, history, nil
	}
	var payload pkgagent.PreStepPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return false, history, nil
	}
	if err := w.runtime.saveOverlay(ctx, job.RunID, payload); err != nil {
		w.runtime.logger().Error("save overlay failed", "run_id", job.RunID, "error", err)
	}
	history.Prompt = payload.SystemPrompt
	history.Hidden = payload.Hidden
	return false, history, nil
}
