package agent

import (
	"context"
	"sync"

	cderr "codedock/internal/errors"
	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db/sqlite"
)

const workerQueueSize = 64

// Worker 用 channel 接收 Run，并用 goroutine 执行。
type Worker struct {
	runtime *Runtime
	jobs    chan string
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	done    map[string]chan struct{}
	skipped map[string]struct{}
	queued  map[string]struct{}
}

// NewWorker 创建 Worker。
func NewWorker(runtime *Runtime) *Worker {
	return &Worker{
		runtime: runtime,
		jobs:    make(chan string, workerQueueSize),
		cancels: make(map[string]context.CancelFunc),
		done:    make(map[string]chan struct{}),
		skipped: make(map[string]struct{}),
		queued:  make(map[string]struct{}),
	}
}

// Start 启动领取循环；每个 Run 在独立 goroutine 中执行，避免堵住领取。
func (w *Worker) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case runID := <-w.jobs:
				go w.execute(ctx, runID)
			}
		}
	}()
}

// Submit 提交 Run ID。已在执行或排队中则去重；缓冲满时立即返回错误。
func (w *Worker) Submit(_ context.Context, runID string) error {
	if w == nil || runID == "" {
		return cderr.Invalid("run id is required")
	}
	w.mu.Lock()
	if _, running := w.cancels[runID]; running {
		w.mu.Unlock()
		return nil
	}
	if _, queued := w.queued[runID]; queued {
		w.mu.Unlock()
		return nil
	}
	w.queued[runID] = struct{}{}
	w.mu.Unlock()
	select {
	case w.jobs <- runID:
		w.runtime.logger().Debug("worker submit", "run_id", runID)
		return nil
	default:
		w.mu.Lock()
		delete(w.queued, runID)
		w.mu.Unlock()
		w.runtime.logger().Error("worker queue full", "run_id", runID)
		return cderr.Unavailable("worker queue full")
	}
}

// Cancel 取消正在执行或尚未领取的 Run。
func (w *Worker) Cancel(runID string) {
	if w == nil || runID == "" {
		return
	}
	w.mu.Lock()
	cancel, ok := w.cancels[runID]
	if !ok {
		w.skipped[runID] = struct{}{}
	}
	w.mu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
	w.runtime.logger().Info("worker cancel", "run_id", runID)
}

// CancelAndWait 取消并等待该 Run 的 Execute 结束。
func (w *Worker) CancelAndWait(runID string) {
	w.Cancel(runID)
	w.mu.Lock()
	done := w.done[runID]
	w.mu.Unlock()
	if done != nil {
		<-done
	}
}

// execute 为单个 Run 建立可取消 context，调用 Execute，结束后领取下一条排队。
func (w *Worker) execute(parent context.Context, runID string) {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	w.mu.Lock()
	delete(w.queued, runID)
	if _, skipped := w.skipped[runID]; skipped {
		delete(w.skipped, runID)
		cancel()
	}
	w.cancels[runID] = cancel
	w.done[runID] = done
	w.mu.Unlock()

	w.runtime.logger().Info("worker execute start", "run_id", runID)
	defer func() {
		cancel()
		w.mu.Lock()
		delete(w.cancels, runID)
		delete(w.done, runID)
		w.mu.Unlock()
		close(done)
		w.runtime.logger().Info("worker execute done", "run_id", runID)
		w.runtime.afterExecute(parent, runID)
	}()

	_ = w.runtime.Execute(ctx, runID)
}

// afterExecute 释放租约；若 Run 已终态则尝试领取下一条 queued。
func (r *Runtime) afterExecute(ctx context.Context, runID string) {
	if r.queries == nil {
		return
	}
	ctx = dbCtx(ctx)
	row, err := r.q(ctx).GetRun(ctx, runID)
	if err != nil {
		return
	}
	run := mapRun(row)
	r.releaseLease(ctx, run.SessionID)
	if !pkgagent.IsTerminal(run.Status) {
		return
	}
	_ = r.TryDequeue(ctx, run.SessionID, run.ID)
}

// TryDequeue 清除已结束 Run 的 active 标记并领取下一条排队 Run。
// 若 Session 已被其他 Run 占用则退出；Claim 成功后再 Submit。
func (r *Runtime) TryDequeue(ctx context.Context, sessionID, finishedRunID string) error {
	ctx = dbCtx(ctx)
	_ = r.clearActive(ctx, sessionID, finishedRunID)
	session, err := r.q(ctx).GetSession(ctx, sessionID)
	if err != nil {
		return wrapDB(err)
	}
	if session.ActiveRunID.Valid && session.ActiveRunID.String != finishedRunID {
		return nil
	}
	if session.ActiveRunID.Valid {
		_ = r.clearActive(ctx, sessionID, session.ActiveRunID.String)
	}
	queued, err := r.q(ctx).ListQueuedRuns(ctx, sessionID)
	if err != nil || len(queued) == 0 {
		return wrapDB(err)
	}
	next := queued[0]
	if _, err := r.q(ctx).ClaimActiveRun(ctx, sqlite.ClaimActiveRunParams{
		ActiveRunID: nullString(next.ID),
		UpdatedAt:   util.FormatTime(util.Now()),
		ID:          sessionID,
	}); err != nil {
		return wrapDB(err)
	}
	r.logger().Info("dequeue queued run", "session_id", sessionID, "finished_run_id", finishedRunID, "next_run_id", next.ID)
	return r.worker.Submit(ctx, next.ID)
}
