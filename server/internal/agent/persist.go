package agent

import (
	"context"
	"database/sql"
	"errors"

	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db"
	"codedock/pkg/db/sqlite"
)

const (
	stepJobQueued    = "queued"
	stepJobRunning   = "running"
	stepJobDone      = "done"
	stepJobCancelled = "cancelled"
)

// q 返回当前上下文可用的 Queries：若上下文存在事务则返回 WithTx 版本，否则返回主 Queries。
func (r *Runtime) q(ctx context.Context) *sqlite.Queries {
	if r.queries == nil {
		return nil
	}
	if tx, ok := db.TxFromContext(ctx); ok {
		return r.queries.WithTx(tx)
	}
	return r.queries
}

// stepJobPayload 把 Job 的 Payload 转成落库字符串；空则写 "{}"。
func stepJobPayload(job pkgagent.StepJob) string {
	if len(job.Payload) == 0 {
		return "{}"
	}
	return string(job.Payload)
}

// persistStepJob 按 run_id + step_index upsert 一条 step_job。attempt 为负时按 0 写入。
func (r *Runtime) persistStepJob(ctx context.Context, job pkgagent.StepJob, status string) error {
	if r == nil || r.q(ctx) == nil || job.RunID == "" {
		return nil
	}
	now := util.FormatTime(util.Now())
	attempt := job.Attempt
	if attempt < 0 {
		attempt = 0
	}
	_, err := r.q(ctx).UpsertStepJob(ctx, sqlite.UpsertStepJobParams{
		RunID:     job.RunID,
		StepIndex: int64(job.StepIndex),
		Phase:     string(job.Phase),
		Payload:   stepJobPayload(job),
		Status:    status,
		Attempt:   int64(attempt),
		CreatedAt: now,
		UpdatedAt: now,
	})
	return err
}

// markStepJobStatus 更新指定步骤的状态；失败只记日志，不打断主流程。
func (r *Runtime) markStepJobStatus(ctx context.Context, runID string, stepIndex int, status string) {
	if r == nil || r.q(ctx) == nil || runID == "" || stepIndex <= 0 {
		return
	}
	if err := r.q(ctx).UpdateStepJobStatus(ctx, sqlite.UpdateStepJobStatusParams{
		Status:    status,
		UpdatedAt: util.FormatTime(util.Now()),
		RunID:     runID,
		StepIndex: int64(stepIndex),
	}); err != nil {
		r.logger().Error("update step job status failed", "run_id", runID, "step_index", stepIndex, "status", status, "error", err)
	}
}

// cancelOpenStepJobs 把该 Run 尚未结束的 step_job 标为 cancelled。
func (r *Runtime) cancelOpenStepJobs(ctx context.Context, runID string) {
	if r == nil || r.q(ctx) == nil || runID == "" {
		return
	}
	if err := r.q(ctx).CancelOpenStepJobs(ctx, sqlite.CancelOpenStepJobsParams{
		UpdatedAt: util.FormatTime(util.Now()),
		RunID:     runID,
	}); err != nil {
		r.logger().Error("cancel step jobs failed", "run_id", runID, "error", err)
	}
}

// latestOpenStepJob 读取该 Run 最新一条 queued/running 的 step_job；没有则 ok=false。
func (r *Runtime) latestOpenStepJob(ctx context.Context, runID string) (sqlite.StepJob, bool, error) {
	if r == nil || r.q(ctx) == nil || runID == "" {
		return sqlite.StepJob{}, false, nil
	}
	row, err := r.q(ctx).GetLatestOpenStepJob(ctx, runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sqlite.StepJob{}, false, nil
		}
		return sqlite.StepJob{}, false, err
	}
	return row, true, nil
}

// stepJobFromRow 把库行转成内存 StepJob；空 payload 不回填。
func stepJobFromRow(row sqlite.StepJob) pkgagent.StepJob {
	job := pkgagent.StepJob{
		RunID:     row.RunID,
		StepIndex: int(row.StepIndex),
		Phase:     pkgagent.Phase(row.Phase),
		Attempt:   int(row.Attempt),
	}
	if row.Payload != "" && row.Payload != "{}" {
		job.Payload = []byte(row.Payload)
	}
	return job
}
