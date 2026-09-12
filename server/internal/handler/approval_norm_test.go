package handler

import (
	"testing"

	pkgagent "codedock/pkg/agent"
)

// TestFillBatchDecisions 校验整单同一裁决时会补齐未列出的 tool_call。
func TestFillBatchDecisions(t *testing.T) {
	calls := []pkgagent.ApprovalToolCall{{ID: "c0"}, {ID: "c1"}}
	got := fillBatchDecisions([]ToolDecision{{ToolCallID: "c1", Status: pkgagent.ApprovalApproved}}, calls)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	byID := map[string]pkgagent.ApprovalStatus{}
	for _, item := range got {
		byID[item.ToolCallID] = item.Status
	}
	if byID["c0"] != pkgagent.ApprovalApproved || byID["c1"] != pkgagent.ApprovalApproved {
		t.Fatalf("got=%+v", got)
	}
}

// TestNormalizeDecisionsCoversBatch 校验只点一条时也能整单通过。
func TestNormalizeDecisionsCoversBatch(t *testing.T) {
	calls := []pkgagent.ApprovalToolCall{{ID: "c0"}, {ID: "c1"}}
	got, err := normalizeDecisions(DecideApprovalRequest{
		Decisions: []ToolDecision{{ToolCallID: "c1", Status: pkgagent.ApprovalDenied}},
	}, calls)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d %+v", len(got), got)
	}
}
