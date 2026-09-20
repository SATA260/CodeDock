package handler_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgagent "codedock/pkg/agent"
)

const notesAPIAddr = "127.0.0.1:18765"

const promptC18Plan = "写一份从零搭建笔记 CRUD 的计划。后端用 Go 标准库 HTTP，前端用 React。验收项字段必须是 id、description、verify_cmd。verify_cmd 必须能编译并跑通后端测试（例如 cd server && go test ./...）。"

const promptC18Agent = `按本会话刚才的计划，从零实现一个笔记 CRUD，不要只口头描述。

后端放在 server/：
- go.mod 的 module 名为 notes
- 只用 Go 标准库，执行 go mod tidy
- 写 *_test.go，先让测试失败再实现到通过；收工前必须 go test ./... 通过
- 监听 127.0.0.1:18765
- JSON API（字段 title、body；创建返回 id）：
  POST /notes → 201
  GET /notes → 200 数组
  GET /notes/{id} → 200
  PUT /notes/{id} → 200
  DELETE /notes/{id} → 204

前端放在 web/：
- package.json 依赖 react 与 react-dom
- 必须在 web/ 执行 npm install，让 node_modules/react 落地
- 页面能列出、新建、编辑、删除笔记，请求上述 /notes API

不要写到工作区外。测试失败必须修好再收工。`

type noteDTO struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// testC18CRUDFromScratch 验收同一会话先写计划，再从零搭 Go+React 笔记 CRUD。
func testC18CRUDFromScratch(t *testing.T) {
	f := newLiveFixture(t)
	ws := initCodingWorkspace(t)
	sessionID := f.createWorkspaceSession(t, ws)

	planID := f.startLive(t, sessionID, promptC18Plan, pkgagent.WorkPlan, pkgagent.ApprovalYolo)
	if f.waitLive(t, planID).Status != pkgagent.RunCompleted {
		t.Fatalf("plan %s", f.getRun(t, planID).Status)
	}
	if !workspaceHasPlanContract(ws) {
		t.Fatalf("no plan contract: %v", listCursorPlans(ws))
	}

	agentID := f.startLiveProject(t, sessionID, promptC18Agent, pkgagent.WorkAgent, pkgagent.ApprovalYolo)
	run := f.waitProjectDone(t, sessionID, agentID)
	if run.Status != pkgagent.RunCompleted {
		t.Fatalf("agent status=%s stop=%v", run.Status, run.StopReason)
	}
	if !hasEventType(t, f, sessionID, pkgagent.EventVerifyStarted) && !hasEventType(t, f, sessionID, pkgagent.EventVerifyResult) {
		t.Fatal("missing verify events")
	}
	assertCRUDProject(t, ws)
}

// workspaceHasPlanContract 判断工作区存在带必填验收字段的计划。
func workspaceHasPlanContract(ws string) bool {
	for _, name := range listCursorPlans(ws) {
		body, err := os.ReadFile(filepath.Join(ws, ".cursor", name))
		if err != nil {
			continue
		}
		if planHasContract(string(body)) {
			return true
		}
	}
	return false
}

// assertCRUDProject 检查计划落地后的 Go 测试、依赖安装和笔记 API。
func assertCRUDProject(t *testing.T, ws string) {
	t.Helper()
	serverDir := filepath.Join(ws, "server")
	webDir := filepath.Join(ws, "web")
	if _, err := os.Stat(filepath.Join(serverDir, "go.mod")); err != nil {
		t.Fatalf("server/go.mod: %v", err)
	}
	mod := readTrim(serverDir, "go.mod")
	if !strings.Contains(mod, "module notes") {
		t.Fatalf("go.mod=%q", mod)
	}
	if !hasGoTestFile(serverDir) {
		t.Fatal("server has no *_test.go")
	}
	runInDir(t, serverDir, "go", "test", "./...")

	pkg := readTrim(webDir, "package.json")
	if !strings.Contains(pkg, "react") || !strings.Contains(pkg, "react-dom") {
		t.Fatalf("web/package.json=%q", pkg)
	}
	if _, err := os.Stat(filepath.Join(webDir, "node_modules", "react")); err != nil {
		t.Fatalf("web/node_modules/react: %v", err)
	}
	if !webTalksToNotes(webDir) {
		t.Fatal("react sources do not call /notes")
	}

	stop := startNotesServer(t, serverDir)
	defer stop()
	smokeNotesAPI(t, "http://"+notesAPIAddr)
}

// hasGoTestFile 判断目录树里是否有 Go 测试文件。
func hasGoTestFile(root string) bool {
	found := false
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if strings.HasSuffix(info.Name(), "_test.go") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// webTalksToNotes 判断前端源码会请求 /notes。
func webTalksToNotes(root string) bool {
	found := false
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "node_modules" || info.Name() == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(info.Name()) {
		case ".js", ".jsx", ".ts", ".tsx":
		default:
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), "/notes") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// runInDir 在指定目录执行命令，失败则带输出报错。
func runInDir(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s in %s: %v\n%s", name, strings.Join(args, " "), dir, err, out)
	}
}

// startNotesServer 编译并拉起约定端口上的笔记服务。
func startNotesServer(t *testing.T, dir string) func() {
	t.Helper()
	target := notesMainTarget(dir)
	cmd := exec.Command("go", "run", target)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		t.Fatalf("go run %s: %v", target, err)
	}
	stop := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + notesAPIAddr + "/notes")
		if err == nil {
			_ = resp.Body.Close()
			return stop
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			t.Fatalf("notes server exited: %s", buf.String())
		}
		time.Sleep(200 * time.Millisecond)
	}
	stop()
	t.Fatalf("notes server did not listen on %s: %s", notesAPIAddr, buf.String())
	return stop
}

// notesMainTarget 选出可 go run 的包路径。
func notesMainTarget(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, "main.go")); err == nil {
		return "."
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "cmd", "*", "main.go"))
	if len(matches) == 1 {
		return "./cmd/" + filepath.Base(filepath.Dir(matches[0]))
	}
	return "."
}

// smokeNotesAPI 按约定走一遍创建、列表、更新、删除。
func smokeNotesAPI(t *testing.T, base string) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	created := requestJSON(t, client, http.MethodPost, base+"/notes", `{"title":"t1","body":"b1"}`)
	if created.status != http.StatusCreated && created.status != http.StatusOK {
		t.Fatalf("POST /notes %d %s", created.status, created.raw)
	}
	note := decodeNote(created.raw)
	if strings.TrimSpace(note.ID) == "" {
		t.Fatalf("create missing id: %s", created.raw)
	}
	listed := requestJSON(t, client, http.MethodGet, base+"/notes", "")
	if listed.status != http.StatusOK {
		t.Fatalf("GET /notes %d %s", listed.status, listed.raw)
	}
	if len(decodeNotes(listed.raw)) == 0 {
		t.Fatalf("GET /notes empty: %s", listed.raw)
	}
	updated := requestJSON(t, client, http.MethodPut, base+"/notes/"+note.ID, `{"title":"t2","body":"b2"}`)
	if updated.status != http.StatusOK {
		t.Fatalf("PUT /notes %d %s", updated.status, updated.raw)
	}
	deleted := requestJSON(t, client, http.MethodDelete, base+"/notes/"+note.ID, "")
	if deleted.status != http.StatusNoContent && deleted.status != http.StatusOK {
		t.Fatalf("DELETE /notes %d %s", deleted.status, deleted.raw)
	}
}

// jsonResp 是一次探测请求的状态码和正文。
type jsonResp struct {
	status int
	raw    string
}

// requestJSON 发一条 JSON HTTP 请求并读完整正文。
func requestJSON(t *testing.T, client *http.Client, method, url, body string) jsonResp {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return jsonResp{status: resp.StatusCode, raw: string(raw)}
}

// decodeNote 从创建/更新响应里抽出一条笔记。
func decodeNote(raw string) noteDTO {
	var note noteDTO
	if json.Unmarshal([]byte(raw), &note) == nil && strings.TrimSpace(note.ID) != "" {
		return note
	}
	var wrap struct {
		Note noteDTO `json:"note"`
	}
	if json.Unmarshal([]byte(raw), &wrap) == nil {
		return wrap.Note
	}
	return noteDTO{}
}

// decodeNotes 从列表响应里抽出笔记数组。
func decodeNotes(raw string) []noteDTO {
	var arr []noteDTO
	if json.Unmarshal([]byte(raw), &arr) == nil {
		return arr
	}
	var wrap struct {
		Notes []noteDTO `json:"notes"`
	}
	if json.Unmarshal([]byte(raw), &wrap) == nil {
		return wrap.Notes
	}
	return nil
}
