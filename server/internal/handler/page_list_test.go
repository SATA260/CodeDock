package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"codedock/internal/handler"
)

// TestListSessionsPagination 验证会话列表分页与排序。
func TestListSessionsPagination(t *testing.T) {
	f := newFixture(t)
	ids := make([]string, 5)
	for i := range ids {
		ids[i] = f.createSession(t)
	}

	page1 := listSessions(t, f, "?page=1&page_size=2&sort_by=created_at&sort_order=asc")
	if page1.Total != 5 || page1.Page != 1 || page1.PageSize != 2 {
		t.Fatalf("page1 meta = %+v", page1.PageInfo)
	}
	if len(page1.Sessions) != 2 {
		t.Fatalf("page1 len=%d", len(page1.Sessions))
	}

	page2 := listSessions(t, f, "?page=2&page_size=2&sort_by=created_at&sort_order=asc")
	if len(page2.Sessions) != 2 {
		t.Fatalf("page2 len=%d", len(page2.Sessions))
	}

	page3 := listSessions(t, f, "?page=3&page_size=2&sort_by=created_at&sort_order=asc")
	if len(page3.Sessions) != 1 {
		t.Fatalf("page3 len=%d", len(page3.Sessions))
	}

	overflow := listSessions(t, f, "?page=99&page_size=2")
	if overflow.Total != 5 || len(overflow.Sessions) != 0 {
		t.Fatalf("overflow total=%d len=%d", overflow.Total, len(overflow.Sessions))
	}

	seen := map[string]int{}
	for _, page := range []handler.ListSessionsResponse{page1, page2, page3} {
		for _, session := range page.Sessions {
			seen[session.ID]++
		}
	}
	if len(seen) != 5 {
		t.Fatalf("unique=%d want 5", len(seen))
	}
	for _, id := range ids {
		if seen[id] != 1 {
			t.Fatalf("id %s count=%d", id, seen[id])
		}
	}

	bad := f.do(t, http.MethodGet, "/sessions?sort_by=id", nil)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid sort status=%d %s", bad.Code, bad.Body.String())
	}
}

// TestListMessagesPagination 依赖完整 Run 循环生成消息，旧 Loop 已删除，骨架阶段跳过。
func TestListMessagesPagination(t *testing.T) {
	t.Skip("旧 Execute Loop 已删除，消息分页依赖完整 Run 实现")
}

// listSessions 发送 GET /sessions 并解析响应。
func listSessions(t *testing.T, f *fixture, query string) handler.ListSessionsResponse {
	t.Helper()
	rec := f.do(t, http.MethodGet, "/sessions"+query, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list sessions %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.ListSessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

// TestListEventsReplay 依赖完整 Run 循环生成事件，旧 Loop 已删除，骨架阶段跳过。
func TestListEventsReplay(t *testing.T) {
	t.Skip("旧 Execute Loop 已删除，事件回放依赖完整 Run 实现")
}

// listMessages 发送 GET /sessions/{id}/messages 并解析响应。
func listMessages(t *testing.T, f *fixture, sessionID, query string) handler.ListMessagesResponse {
	t.Helper()
	rec := f.do(t, http.MethodGet, "/sessions/"+sessionID+"/messages"+query, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list messages %d %s", rec.Code, rec.Body.String())
	}
	var resp handler.ListMessagesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}
