package agent

import "context"

// Worker 用 channel 接收 Run，并用 goroutine 执行。本阶段为空实现。
type Worker struct {
	runtime *Runtime
	jobs    chan string
}

// NewWorker 创建 Worker。
func NewWorker(runtime *Runtime) *Worker {
	return &Worker{
		runtime: runtime,
		jobs:    make(chan string),
	}
}

// Submit 提交 Run ID。本阶段为空实现。
func (w *Worker) Submit(_ context.Context, _ string) error {
	_ = w.jobs
	_ = w.runtime
	return nil
}

// Cancel 取消正在执行的 Run。本阶段为空实现。
func (w *Worker) Cancel(_ string) {}
