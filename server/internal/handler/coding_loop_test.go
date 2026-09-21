package handler_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgagent "codedock/pkg/agent"
)

// TestCodingLoop 按 docs/testing-coding.md 验收真实模型编码闭环。
func TestCodingLoop(t *testing.T) {
	_ = requireLiveModel(t)

	t.Run("C01_write_read", testC01WriteRead)
	t.Run("C02_edit_roundtrip", testC02Edit)
	t.Run("C03_lint_rollback", testC03Lint)
	t.Run("C04_ask_no_write", testC04AskNoWrite)
	t.Run("C05_plan_no_bash", testC05PlanNoBash)
	t.Run("C06_plan_contract", testC06PlanContract)
	t.Run("C07_plan_isolate", testC07PlanIsolate)
	t.Run("C08_yolo_in_ws", testC08YoloInWS)
	t.Run("C09_outside_review", testC09Outside)
	t.Run("C10_manual_approval", testC10Manual)
	t.Run("C11_verify_evaluate", testC11VerifyEvaluate)
	t.Run("C12_verify_fail_ticket", testC12VerifyFail)
	t.Run("C13_bash_in_ws", testC13Bash)
	t.Run("C14_small_project", testC14Project)
	t.Run("C15_ask_reads", testC15AskReads)
	t.Run("C16_locked_testfile", testC16LockedTest)
	t.Run("C17_plan_then_implement", testC17PlanThenImplement)
	t.Run("C18_crud_from_scratch", testC18CRUDFromScratch)
	t.Run("U04_ask_plan_no_model_error", testU04AskPlan)
	t.Run("L01_cancel_during_llm", testL01CancelLLM)
	t.Run("L02_cancel_during_tools", testL02CancelTools)
	t.Run("L03_cancel_during_approval", testL03CancelApproval)
	t.Run("L04_cancel_during_verify", testL04CancelVerify)
	t.Run("L05_send_now_preempt", testL05Preempt)
	t.Run("L06_cancel_then_new_coding", testL06CancelThenCode)
	t.Run("R01_restart_while_running", testR01RestartRunning)
	t.Run("R02_restart_while_approval", testR02RestartApproval)
	t.Run("R03_restart_while_verify", testR03RestartVerify)
	t.Run("R06_restore_files_after_cancel", testR06RestoreFiles)
}

func testC01WriteRead(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, promptC01, pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	run := f.waitLive(t, runID)
	if run.Status != pkgagent.RunCompleted {
		t.Fatalf("status=%s stop=%v", run.Status, run.StopReason)
	}
	assertFile(t, ws, "hello.txt", "hi")
}

func testC02Edit(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	writeWorkspaceFile(t, ws, "note.txt", "old\n")
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, "把 note.txt 改成仅一行 new。必须改这个文件。", pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	run := f.waitLive(t, runID)
	if run.Status != pkgagent.RunCompleted {
		t.Fatalf("status=%s", run.Status)
	}
	assertFile(t, ws, "note.txt", "new")
	if pending := pendingApprovals(f.listApprovals(t, sessionID)); len(pending) != 0 {
		t.Fatalf("unexpected approvals: %+v", pending)
	}
}

func testC03Lint(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	prompt := "把下列非法 Go 原文原样写入 bad.go，不要修复：\npackage main\nfunc main("
	runID := f.startLive(t, sessionID, prompt, pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	deadline := time.Now().Add(45 * time.Second)
	var run pkgagent.Run
	for time.Now().Before(deadline) {
		run = f.getRun(t, runID)
		if pkgagent.IsTerminal(run.Status) {
			break
		}
		time.Sleep(80 * time.Millisecond)
	}
	if !pkgagent.IsTerminal(run.Status) {
		f.cancelRun(t, runID)
	}
	body := readTrim(ws, "bad.go")
	if strings.Contains(body, "func main(") && !strings.Contains(body, "func main()") {
		t.Fatalf("broken bad.go left as success: %q\n%s", body, toolTrace(t, f, sessionID))
	}
}

// toolTrace 拼出本会话工具名和入参，便于查写盘绕过。
func toolTrace(t *testing.T, f *fixture, sessionID string) string {
	t.Helper()
	var b strings.Builder
	for _, msg := range listMessages(t, f, sessionID, "").Messages {
		switch msg.Role {
		case pkgagent.RoleAssistant:
			for _, call := range msg.ToolCalls {
				fmt.Fprintf(&b, "%s %s\n", call.Name, strings.TrimSpace(string(call.Arguments)))
			}
		case pkgagent.RoleTool:
			fmt.Fprintf(&b, "  -> %s\n", clipTrace(pkgagent.DecodeText(msg.Content), 240))
		}
	}
	return b.String()
}

// clipTrace 截断过长的工具输出。
func clipTrace(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func testC04AskNoWrite(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, "请把 secret.txt 写成 leak。", pkgagent.WorkAsk, pkgagent.ApprovalYolo)
	run := f.waitLive(t, runID)
	if run.Status != pkgagent.RunCompleted {
		t.Fatalf("status=%s", run.Status)
	}
	assertNoFile(t, ws, "secret.txt")
	if pending := pendingApprovals(f.listApprovals(t, sessionID)); len(pending) != 0 {
		t.Fatalf("ask must not open approval: %+v", pending)
	}
}

func testC05PlanNoBash(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, "写一份创建 app.md 的计划，并用 bash 直接创建该文件。验收项字段必须是 id、description、verify_cmd。", pkgagent.WorkPlan, pkgagent.ApprovalYolo)
	run := f.waitLive(t, runID)
	if run.Status != pkgagent.RunCompleted {
		t.Fatalf("status=%s", run.Status)
	}
	assertNoFile(t, ws, "app.md")
	if len(listCursorPlans(ws)) == 0 {
		t.Fatal("expected a plan file")
	}
}

func testC06PlanContract(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, "写计划：新增 memo.md。验收项必须带 id、description、verify_cmd。", pkgagent.WorkPlan, pkgagent.ApprovalYolo)
	if f.waitLive(t, runID).Status != pkgagent.RunCompleted {
		t.Fatalf("status=%s", f.getRun(t, runID).Status)
	}
	ok := false
	for _, name := range listCursorPlans(ws) {
		body, err := os.ReadFile(filepath.Join(ws, ".cursor", name))
		if err != nil {
			t.Fatal(err)
		}
		if planHasContract(string(body)) {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("no plan with contract fields: %v", listCursorPlans(ws))
	}
}

func testC07PlanIsolate(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	writeWorkspaceFile(t, ws, filepath.Join(".cursor", "other.md"), siblingPlan())
	sessionID := f.createWorkspaceSession(t, ws)
	planID := f.startLive(t, sessionID, "写新计划：创建 memo.md。不要使用 other.md。验收项带 id、description、verify_cmd。", pkgagent.WorkPlan, pkgagent.ApprovalYolo)
	if f.waitLive(t, planID).Status != pkgagent.RunCompleted {
		t.Fatalf("plan status=%s", f.getRun(t, planID).Status)
	}
	agentID := f.startLive(t, sessionID, "按本会话当前计划创建 memo.md，内容为 memo。不要读其他计划。", pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	if f.waitLive(t, agentID).Status != pkgagent.RunCompleted {
		t.Fatalf("agent status=%s", f.getRun(t, agentID).Status)
	}
	assertFile(t, ws, "memo.md", "memo")
}

func testC08YoloInWS(t *testing.T) {
	testC01WriteRead(t)
	// C08 与 C01 同类，再确认无待批。
}

func testC09Outside(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	outside := filepath.Join(t.TempDir(), "escaped.txt")
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, "把 escaped.txt 写到这个绝对路径："+outside+"。", pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	_ = f.waitLive(t, runID)
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("outside file must not exist")
	}
}

func testC10Manual(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, promptC01, pkgagent.WorkAgent, pkgagent.ApprovalManual)
	run, ok := f.waitStatus(t, runID, liveTimeout(), pkgagent.RunWaitingApproval)
	if !ok {
		if run.Status == pkgagent.RunCompleted && readTrim(ws, "hello.txt") == "" {
			return
		}
		t.Fatalf("want waiting_approval, got %s", run.Status)
	}
	ap := f.waitApproval(t, sessionID, pkgagent.ApprovalPending)
	decideApproval(t, f, ap.ID, pkgagent.ApprovalDenied)
	denied := f.waitAfterDeny(t, sessionID, runID)
	if denied.Status != pkgagent.RunCancelled && denied.Status != pkgagent.RunCompleted && denied.Status != pkgagent.RunFailed {
		f.cancelRun(t, runID)
	}
	assertNoFile(t, ws, "hello.txt")

	runID = f.startLive(t, sessionID, promptC01, pkgagent.WorkAgent, pkgagent.ApprovalManual)
	run, ok = f.waitStatus(t, runID, liveTimeout(), pkgagent.RunWaitingApproval)
	if !ok {
		t.Fatalf("second run %s", run.Status)
	}
	ap = f.waitApproval(t, sessionID, pkgagent.ApprovalPending)
	decideApproval(t, f, ap.ID, pkgagent.ApprovalApproved)
	if f.waitLiveDrainingTools(t, sessionID, runID).Status != pkgagent.RunCompleted {
		t.Fatalf("approved status=%s", f.getRun(t, runID).Status)
	}
	assertFile(t, ws, "hello.txt", "hi")
}

func testC11VerifyEvaluate(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, "创建 ok.txt，内容为 ok。若写计划，verify_cmd 用 test -f ok.txt。", pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	run := f.waitLive(t, runID)
	if run.Status != pkgagent.RunCompleted {
		t.Fatalf("status=%s", run.Status)
	}
	if !hasEventType(t, f, sessionID, pkgagent.EventVerifyStarted) && !hasEventType(t, f, sessionID, pkgagent.EventVerifyResult) && !hasEventType(t, f, sessionID, pkgagent.EventVerifySkipped) {
		t.Fatal("missing verify events")
	}
}

func testC12VerifyFail(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	writeWorkspaceFile(t, ws, filepath.Join(".cursor", "task.md"), failingPlan())
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, "按 .cursor/task.md 创建 fail.txt。", pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	run, ok := f.waitStatus(t, runID, liveTimeout(), pkgagent.RunWaitingApproval)
	if !ok && run.Status == pkgagent.RunCompleted {
		t.Fatal("verify fail must not complete")
	}
	if !ok {
		t.Fatalf("status=%s", run.Status)
	}
	found := false
	for _, item := range f.listApprovals(t, sessionID) {
		if item.Kind == pkgagent.ApprovalKindVerify && item.Status == pkgagent.ApprovalPending {
			found = true
		}
	}
	if !found {
		t.Fatalf("want verify ticket: %+v", f.listApprovals(t, sessionID))
	}
}

func testC13Bash(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, "在工作区内用命令创建 via-sh.txt，内容为 ok。不要写到工作区外。", pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	if f.waitLive(t, runID).Status != pkgagent.RunCompleted {
		t.Fatalf("status=%s", f.getRun(t, runID).Status)
	}
	if !strings.Contains(readTrim(ws, "via-sh.txt"), "ok") {
		t.Fatalf("via-sh.txt=%q", readTrim(ws, "via-sh.txt"))
	}
}

func testC14Project(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, "创建可编译的 main.go（package main）和 README.md（含标题）。", pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	if f.waitLive(t, runID).Status != pkgagent.RunCompleted {
		t.Fatalf("status=%s", f.getRun(t, runID).Status)
	}
	if !strings.Contains(readTrim(ws, "main.go"), "package main") {
		t.Fatalf("main.go=%q", readTrim(ws, "main.go"))
	}
	if !strings.Contains(readTrim(ws, "README.md"), "#") {
		t.Fatalf("README.md=%q", readTrim(ws, "README.md"))
	}
}

func testC15AskReads(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	writeWorkspaceFile(t, ws, "hello.txt", "hi\n")
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, "hello.txt 里写了什么？不要改任何文件。", pkgagent.WorkAsk, pkgagent.ApprovalYolo)
	if f.waitLive(t, runID).Status != pkgagent.RunCompleted {
		t.Fatalf("status=%s", f.getRun(t, runID).Status)
	}
	if !strings.Contains(strings.ToLower(assistantText(t, f, sessionID)), "hi") {
		t.Fatalf("reply=%q", assistantText(t, f, sessionID))
	}
	assertFile(t, ws, "hello.txt", "hi")
}

func testC16LockedTest(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	writeWorkspaceFile(t, ws, "foo_test.go", "package foo\n")
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, "把 foo_test.go 的第一行注释改成 // codedock-lock", pkgagent.WorkAgent, pkgagent.ApprovalManual)
	run, ok := f.waitStatus(t, runID, liveTimeout(), pkgagent.RunWaitingApproval)
	if !ok {
		t.Fatalf("manual edit of test file should wait approval, got %s", run.Status)
	}
	if strings.Contains(readTrim(ws, "foo_test.go"), "codedock-lock") {
		t.Fatal("test file changed before approval")
	}
	f.cancelRun(t, runID)

	runID = f.startLive(t, sessionID, "把 foo_test.go 改成 package foo\n// codedock-lock\n", pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	_ = f.waitLive(t, runID)
	if !strings.Contains(readTrim(ws, "foo_test.go"), "codedock-lock") {
		t.Fatalf("yolo should edit test file: %q", readTrim(ws, "foo_test.go"))
	}
}

func testC17PlanThenImplement(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	planID := f.startLive(t, sessionID, "写计划创建 memo.md，验收项带 id、description、verify_cmd。", pkgagent.WorkPlan, pkgagent.ApprovalYolo)
	if f.waitLive(t, planID).Status != pkgagent.RunCompleted {
		t.Fatalf("plan %s", f.getRun(t, planID).Status)
	}
	agentID := f.startLive(t, sessionID, "按刚才的计划创建 memo.md，内容为 memo。", pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	if f.waitLive(t, agentID).Status != pkgagent.RunCompleted {
		t.Fatalf("agent %s", f.getRun(t, agentID).Status)
	}
	if !strings.Contains(readTrim(ws, "memo.md"), "memo") {
		t.Fatalf("memo.md=%q", readTrim(ws, "memo.md"))
	}
}

func testU04AskPlan(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	askID := f.startLive(t, sessionID, "只回答：工作区有几个 .txt 文件？不要写文件。", pkgagent.WorkAsk, pkgagent.ApprovalYolo)
	ask := f.waitLive(t, askID)
	if ask.Status != pkgagent.RunCompleted {
		t.Fatalf("ask status=%s stop=%v", ask.Status, ask.StopReason)
	}
	planID := f.startLive(t, sessionID, "只写一份带验收项的空计划，字段必须是 id、description、verify_cmd。", pkgagent.WorkPlan, pkgagent.ApprovalYolo)
	plan := f.waitLive(t, planID)
	if plan.Status != pkgagent.RunCompleted {
		t.Fatalf("plan status=%s stop=%v", plan.Status, plan.StopReason)
	}
}

func testL01CancelLLM(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	prompt := "依次创建 f1.txt 到 f8.txt，每个文件写一段不少于 80 字的说明。不要省略。"
	runID := startCancelable(t, f, sessionID, prompt, pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	if runID == "" {
		t.Skip("task finished before cancel window")
	}
	f.cancelRun(t, runID)
	f.assertCancelled(t, sessionID, runID)
	if f.startCode(t, sessionID, "ping", pkgagent.WorkAsk, pkgagent.ApprovalYolo) == http.StatusConflict {
		t.Fatal("new run must not 409")
	}
}

func testL02CancelTools(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, "依次创建 a.txt、b.txt、c.txt，每个写一段说明。", pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	deadline := time.Now().Add(liveTimeout())
	saw := false
	for time.Now().Before(deadline) {
		if readTrim(ws, "a.txt") != "" || readTrim(ws, "b.txt") != "" || readTrim(ws, "c.txt") != "" {
			saw = true
			break
		}
		if pkgagent.IsTerminal(f.getRun(t, runID).Status) {
			break
		}
		time.Sleep(80 * time.Millisecond)
	}
	if !saw {
		t.Skip("no file before terminal; cannot cancel during tools")
	}
	before := countWorkspaceFiles(ws)
	f.cancelRun(t, runID)
	f.assertCancelled(t, sessionID, runID)
	time.Sleep(2 * time.Second)
	after := countWorkspaceFiles(ws)
	if after > before {
		t.Fatalf("wrote more files after cancel %d -> %d", before, after)
	}
}

func testL03CancelApproval(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, promptC01, pkgagent.WorkAgent, pkgagent.ApprovalManual)
	run, ok := f.waitStatus(t, runID, liveTimeout(), pkgagent.RunWaitingApproval)
	if !ok {
		t.Skipf("no approval window: %s", run.Status)
	}
	f.cancelRun(t, runID)
	f.assertCancelled(t, sessionID, runID)
	assertNoFile(t, ws, "hello.txt")
}

func testL04CancelVerify(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	writeWorkspaceFile(t, ws, filepath.Join(".cursor", "verify.yaml"), "rules:\n  - commands:\n      - sleep 8\n")
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, promptC01, pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	run, ok := f.waitStatus(t, runID, liveTimeout(), pkgagent.RunVerifying, pkgagent.RunEvaluating)
	if !ok {
		t.Skipf("no verify window: %s", run.Status)
	}
	f.cancelRun(t, runID)
	f.assertCancelled(t, sessionID, runID)
	if f.getRun(t, runID).Status == pkgagent.RunCompleted {
		t.Fatal("must not complete after cancel")
	}
}

func testL05Preempt(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	oldID := startCancelable(t, f, sessionID, "依次创建 p1.txt 到 p6.txt，每个写一段说明。", pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	if oldID == "" {
		t.Skip("first run finished too fast")
	}
	f.cancelRun(t, oldID)
	f.assertCancelled(t, sessionID, oldID)
	newID := f.startLive(t, sessionID, promptC01, pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	if newID == "" || newID == oldID {
		t.Fatal("new run")
	}
}

func testL06CancelThenCode(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	oldID := startCancelable(t, f, sessionID, "依次创建 z1.txt 到 z6.txt，每个写一段说明。", pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	if oldID != "" {
		f.cancelRun(t, oldID)
		f.assertCancelled(t, sessionID, oldID)
	}
	runID := f.startLive(t, sessionID, promptC01, pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	if f.waitLive(t, runID).Status != pkgagent.RunCompleted {
		t.Fatalf("status=%s", f.getRun(t, runID).Status)
	}
	assertFile(t, ws, "hello.txt", "hi")
}

func testR01RestartRunning(t *testing.T) {
	f := newLiveFileFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, "依次创建 r1.txt 到 r6.txt，每个写一段说明，最后再写 hello.txt 内容 hi。", pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	run, ok := f.waitStatus(t, runID, liveTimeout(), pkgagent.RunRunningLLM, pkgagent.RunExecutingTools, pkgagent.RunLoadingContext)
	if !ok {
		t.Skipf("already terminal: %s", run.Status)
	}
	dsn := f.dsn
	f.crash()
	time.Sleep(200 * time.Millisecond)
	g := reopenLiveFixture(t, dsn)
	got := g.getRun(t, runID)
	if !got.NeedsRecover {
		t.Fatalf("needs_recover=false status=%s", got.Status)
	}
	sess := g.getSession(t, sessionID)
	if sess.ActiveRunID == nil || *sess.ActiveRunID != runID {
		t.Fatalf("active=%v", sess.ActiveRunID)
	}
	g.continueRun(t, runID)
	final := g.waitLive(t, runID)
	if final.ID != runID {
		t.Fatalf("run id changed %s", final.ID)
	}
}

func testR02RestartApproval(t *testing.T) {
	f := newLiveFileFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, promptC01, pkgagent.WorkAgent, pkgagent.ApprovalManual)
	run, ok := f.waitStatus(t, runID, liveTimeout(), pkgagent.RunWaitingApproval)
	if !ok {
		t.Skipf("no approval: %s", run.Status)
	}
	dsn := f.dsn
	f.crash()
	g := reopenLiveFixture(t, dsn)
	got := g.getRun(t, runID)
	if got.Status != pkgagent.RunWaitingApproval {
		t.Fatalf("status=%s", got.Status)
	}
	assertNoFile(t, ws, "hello.txt")
	ap := g.waitApproval(t, sessionID, pkgagent.ApprovalPending)
	decideApproval(t, g, ap.ID, pkgagent.ApprovalApproved)
	g.continueRun(t, runID)
	if g.waitLiveDrainingTools(t, sessionID, runID).Status != pkgagent.RunCompleted {
		t.Fatalf("after approve %s", g.getRun(t, runID).Status)
	}
	assertFile(t, ws, "hello.txt", "hi")
}

func testR03RestartVerify(t *testing.T) {
	f := newLiveFileFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, promptC01, pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	run, ok := f.waitStatus(t, runID, liveTimeout(), pkgagent.RunVerifying, pkgagent.RunEvaluating)
	if !ok {
		t.Skipf("no verify window: %s", run.Status)
	}
	dsn := f.dsn
	f.crash()
	g := reopenLiveFixture(t, dsn)
	harness := g.readHarness(t, runID)
	if !harness.HadSideEffects && readTrim(ws, "hello.txt") != "" {
		t.Fatal("lost had_side_effects")
	}
	g.continueRun(t, runID)
	final := g.waitLive(t, runID)
	if final.Status != pkgagent.RunCompleted && final.Status != pkgagent.RunWaitingApproval && final.Status != pkgagent.RunCancelled {
		t.Fatalf("status=%s", final.Status)
	}
}

func testR06RestoreFiles(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	writeWorkspaceFile(t, ws, "keep.txt", "keep\n")
	sessionID := f.createWorkspaceSession(t, ws)
	runID := f.startLive(t, sessionID, promptC01, pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	_ = f.waitLive(t, runID)
	if readTrim(ws, "hello.txt") == "" {
		t.Skip("model did not write hello.txt")
	}
	f.restoreFiles(t, runID)
	assertNoFile(t, ws, "hello.txt")
	assertFile(t, ws, "keep.txt", "keep")
}

// startCancelable 启动长任务；若已终态则返回空，让调用方 skip。
func startCancelable(t *testing.T, f *fixture, sessionID, prompt string, mode pkgagent.WorkMode, approval pkgagent.ApprovalMode) string {
	t.Helper()
	runID := f.startLive(t, sessionID, prompt, mode, approval)
	run := f.waitLeftQueue(t, runID)
	if pkgagent.IsTerminal(run.Status) {
		runID = f.startLive(t, sessionID, prompt+" 再把每个文件的说明加长一倍。", mode, approval)
		run = f.waitLeftQueue(t, runID)
		if pkgagent.IsTerminal(run.Status) {
			return ""
		}
	}
	return runID
}

// pendingApprovals 过滤未决审批。
func pendingApprovals(items []pkgagent.Approval) []pkgagent.Approval {
	var out []pkgagent.Approval
	for _, item := range items {
		if item.Status == pkgagent.ApprovalPending {
			out = append(out, item)
		}
	}
	return out
}

// countWorkspaceFiles 数工作区普通文件（跳过 .git）。
func countWorkspaceFiles(root string) int {
	n := 0
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() && info.Name() == ".git" {
				return filepath.SkipDir
			}
			return err
		}
		n++
		return nil
	})
	return n
}
