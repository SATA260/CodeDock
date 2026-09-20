package agent

import (
	"context"
	"encoding/json"

	cderr "codedock/internal/errors"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db/sqlite"
)

// applyHarness 把 runs.harness / snapshot_oid 写回 AgentState。
func (r *Runtime) applyHarness(ctx context.Context, runID string, state *pkgagent.AgentState) {
	if r == nil || state == nil || runID == "" {
		return
	}
	row, err := r.q(ctx).GetRunHarness(ctx, runID)
	if err != nil {
		return
	}
	if row.SnapshotOid.Valid {
		state.SnapshotID = row.SnapshotOid.String
	}
	if row.Harness == "" || row.Harness == "{}" {
		return
	}
	var harness pkgagent.RunHarness
	if json.Unmarshal([]byte(row.Harness), &harness) != nil {
		return
	}
	state.ApplyHarness(harness)
	if state.SnapshotID == "" && row.SnapshotOid.Valid {
		state.SnapshotID = row.SnapshotOid.String
	}
}

// saveHarness 把正确性字段写回 runs 表。
func (r *Runtime) saveHarness(ctx context.Context, runID string, state pkgagent.AgentState) error {
	if r == nil || runID == "" {
		return nil
	}
	body, err := json.Marshal(state.Harness())
	if err != nil {
		body = []byte("{}")
	}
	return r.q(ctx).UpdateRunHarness(ctx, sqlite.UpdateRunHarnessParams{
		SnapshotOid: nullString(state.SnapshotID),
		Harness:     string(body),
		ID:          runID,
	})
}

// RestoreRun 按快照模式还原工作区。
func (r *Runtime) RestoreRun(ctx context.Context, runID string, mode pkgagent.RestoreMode) error {
	if r == nil || runID == "" {
		return cderr.Invalid("run id is required")
	}
	state, _, err := r.LoadAgentState(ctx, runID)
	if err != nil {
		return err
	}
	if mode == "" {
		mode = pkgagent.RestoreFiles
	}
	if mode == pkgagent.RestoreMessages {
		return nil
	}
	if r.engine != nil {
		r.engine.RestoreSnapshot(state, mode)
	}
	return nil
}
