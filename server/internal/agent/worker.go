package agent

import "context"

// Worker 用 channel 接收 Run，并用 goroutine 执行。
// TODO: 启动 goroutine 从 jobs 领取 Run 并调用 Execute。
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

// Submit 提交 Run ID。
// TODO: 将 Run ID 送入 jobs。
func (w *Worker) Submit(_ context.Context, _ string) error {
	_ = w.jobs
	_ = w.runtime
	return nil
}

// Cancel 取消正在执行的 Run。
// TODO: 取消正在执行的 Run。
func (w *Worker) Cancel(_ string) {}
