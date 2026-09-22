package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"codedock/internal/board"
	cderr "codedock/internal/errors"
	"codedock/pkg/github"
)

type CreateWorkRequest struct {
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
	Title    string `json:"title"`
}

type UpdateWorkRequest struct {
	Title string `json:"title"`
}

type WorkResponse struct {
	Work board.Work `json:"work"`
}

type ListWorksResponse struct {
	Works []board.Work `json:"works"`
}

type AttachCheckoutRequest struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type CheckoutResponse struct {
	Checkout board.Checkout `json:"checkout"`
}

type ListCheckoutsResponse struct {
	Checkouts []board.Checkout `json:"checkouts"`
}

type PutInfoRequest struct {
	Checkout string `json:"checkout"`
	Body     string `json:"body"`
}

type InfoResponse struct {
	Info board.Info `json:"info"`
}

type StartWorkSessionRequest struct {
	Engine   string `json:"engine"`
	Kind     string `json:"kind"` // talk | dir
	Checkout string `json:"checkout"`
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
	AgentID  string `json:"agent_id"`
}

type PlacementResponse struct {
	Placement board.Placement `json:"placement"`
	SessionID string          `json:"session_id"`
	Engine    string          `json:"engine"`
}

type AttachPlacementRequest struct {
	Engine    string `json:"engine"`
	SessionID string `json:"session_id"`
}

type ListPlacementsResponse struct {
	Placements []board.Placement `json:"placements"`
}

type BoardResponse struct {
	Board board.BoardView `json:"board"`
}

type CardResponse struct {
	Card board.Card `json:"card"`
}

type InboxResponse struct {
	Items []board.InboxItem `json:"items"`
}

type InboxDecideRequest struct {
	Engine    string               `json:"engine"`
	TicketID  string               `json:"ticket_id"`
	Decisions []board.ToolDecision `json:"decisions"`
	Status    string               `json:"status"`
	Scope     string               `json:"scope"`
	ActorID   string               `json:"actor_id"`
	Reason    string               `json:"reason"`
	Override  string               `json:"override,omitempty"`
	Approved  bool                 `json:"approved"`
	Choice    string               `json:"choice"`
	Values    []string             `json:"values"`
	Answers   map[string]string    `json:"answers,omitempty"`
}

type PacketResponse struct {
	Packet board.Packet `json:"packet"`
}

type LinksResponse struct {
	Links board.SessionLinks `json:"links"`
}

type PutLinkRequest struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Ref    string `json:"ref"`
}

type ReplaceLinksRequest struct {
	Links []string `json:"links"`
}

// CreateWork 建一张卡并挂空白 Work 级 Info。
func (a *API) CreateWork(w http.ResponseWriter, r *http.Request) {
	var req CreateWorkRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	work, err := board.CreateWork(r.Context(), a.q(r.Context()), req.TenantID, req.UserID, req.Title)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, WorkResponse{Work: work})
}

// ListWorks 列出用户的卡。
func (a *API) ListWorks(w http.ResponseWriter, r *http.Request) {
	works, err := board.ListWorks(r.Context(), a.q(r.Context()), r.URL.Query().Get("tenant_id"), r.URL.Query().Get("user_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ListWorksResponse{Works: works})
}

// GetWork 读取一张卡。
func (a *API) GetWork(w http.ResponseWriter, r *http.Request) {
	work, err := board.GetWork(r.Context(), a.q(r.Context()), chi.URLParam(r, "work_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, WorkResponse{Work: work})
}

// UpdateWork 改卡片标题。
func (a *API) UpdateWork(w http.ResponseWriter, r *http.Request) {
	var req UpdateWorkRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	work, err := board.UpdateWork(r.Context(), a.q(r.Context()), chi.URLParam(r, "work_id"), req.Title)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, WorkResponse{Work: work})
}

// DeleteWork 删卡并断开归属。
func (a *API) DeleteWork(w http.ResponseWriter, r *http.Request) {
	if err := board.DeleteWork(r.Context(), a.q(r.Context()), chi.URLParam(r, "work_id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// AttachWorkCheckout 把已存在目录挂到卡上。
func (a *API) AttachWorkCheckout(w http.ResponseWriter, r *http.Request) {
	var req AttachCheckoutRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	kind, err := board.ParseCheckoutKind(req.Kind)
	if err != nil {
		writeError(w, err)
		return
	}
	co, err := board.AttachCheckout(r.Context(), a.q(r.Context()), chi.URLParam(r, "work_id"), req.Path, kind)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, CheckoutResponse{Checkout: co})
}

// ListWorkCheckouts 列出卡上目录。
func (a *API) ListWorkCheckouts(w http.ResponseWriter, r *http.Request) {
	items, err := board.ListCheckouts(r.Context(), a.q(r.Context()), chi.URLParam(r, "work_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ListCheckoutsResponse{Checkouts: items})
}

// DetachWorkCheckout 卸下目录，该路径上的会话变成问答。
func (a *API) DetachWorkCheckout(w http.ResponseWriter, r *http.Request) {
	var req AttachCheckoutRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Path == "" {
		req.Path = r.URL.Query().Get("path")
	}
	if err := board.DetachCheckout(r.Context(), a.q(r.Context()), chi.URLParam(r, "work_id"), req.Path); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GetWorkInfo 读 Work 级或目录级说明。
func (a *API) GetWorkInfo(w http.ResponseWriter, r *http.Request) {
	info, err := board.GetInfo(r.Context(), a.q(r.Context()), chi.URLParam(r, "work_id"), r.URL.Query().Get("checkout"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, InfoResponse{Info: info})
}

// PutWorkInfo 覆盖写入说明。
func (a *API) PutWorkInfo(w http.ResponseWriter, r *http.Request) {
	var req PutInfoRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	info, err := board.PutInfo(r.Context(), a.q(r.Context()), chi.URLParam(r, "work_id"), req.Checkout, req.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, InfoResponse{Info: info})
}

// StartWorkSession 从一张卡开问答或目录会话。
func (a *API) StartWorkSession(w http.ResponseWriter, r *http.Request) {
	var req StartWorkSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	engine, err := board.ParseEngine(req.Engine)
	if err != nil {
		writeError(w, err)
		return
	}
	spec := board.StartSpec{Engine: engine, UserID: req.UserID, TenantID: req.TenantID, AgentID: req.AgentID}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	var place board.Placement
	var started board.Started
	if kind == "dir" {
		place, started, err = a.board.StartInDir(r.Context(), chi.URLParam(r, "work_id"), req.Checkout, spec)
	} else {
		place, started, err = a.board.StartTalk(r.Context(), chi.URLParam(r, "work_id"), spec)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, PlacementResponse{
		Placement: publicPlacement(place),
		SessionID: started.ID,
		Engine:    board.PublicEngine(place.Engine),
	})
}

// AttachWorkPlacement 把未归组会话补挂到卡上。
func (a *API) AttachWorkPlacement(w http.ResponseWriter, r *http.Request) {
	var req AttachPlacementRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	engine, err := board.ParseEngine(req.Engine)
	if err != nil {
		writeError(w, err)
		return
	}
	place, err := board.AttachExisting(r.Context(), a.q(r.Context()), engine, req.SessionID, chi.URLParam(r, "work_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, PlacementResponse{
		Placement: publicPlacement(place),
		SessionID: place.SessionID,
		Engine:    board.PublicEngine(place.Engine),
	})
}

// ListPlacements 列出用户所有归属，给侧栏分组。
func (a *API) ListPlacements(w http.ResponseWriter, r *http.Request) {
	items, err := board.ListUserPlacements(r.Context(), a.q(r.Context()), r.URL.Query().Get("tenant_id"), r.URL.Query().Get("user_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ListPlacementsResponse{Placements: publicPlacements(items)})
}

// GetPlacement 读一路会话的归属。
func (a *API) GetPlacement(w http.ResponseWriter, r *http.Request) {
	engine, err := board.ParseEngine(chi.URLParam(r, "engine"))
	if err != nil {
		writeError(w, err)
		return
	}
	place, err := board.GetPlacement(r.Context(), a.q(r.Context()), engine, chi.URLParam(r, "session_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, PlacementResponse{
		Placement: publicPlacement(place),
		SessionID: place.SessionID,
		Engine:    board.PublicEngine(place.Engine),
	})
}

// DeletePlacement 断开归属。
func (a *API) DeletePlacement(w http.ResponseWriter, r *http.Request) {
	engine, err := board.ParseEngine(chi.URLParam(r, "engine"))
	if err != nil {
		writeError(w, err)
		return
	}
	if err := board.Unbind(r.Context(), a.q(r.Context()), engine, chi.URLParam(r, "session_id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GetBoard 聚合横向看板。
func (a *API) GetBoard(w http.ResponseWriter, r *http.Request) {
	view, err := a.board.Cards(r.Context(), r.URL.Query().Get("tenant_id"), r.URL.Query().Get("user_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, BoardResponse{Board: view})
}

// GetBoardCard 聚合一张卡。
func (a *API) GetBoardCard(w http.ResponseWriter, r *http.Request) {
	card, err := a.board.Card(r.Context(), chi.URLParam(r, "work_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, CardResponse{Card: card})
}

// ListWorkInbox 扫该卡下三引擎待批。
func (a *API) ListWorkInbox(w http.ResponseWriter, r *http.Request) {
	items, err := a.board.ListInbox(r.Context(), chi.URLParam(r, "work_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, InboxResponse{Items: items})
}

// DecideInbox 按引擎把裁决转给已有审批。
func (a *API) DecideInbox(w http.ResponseWriter, r *http.Request) {
	var req InboxDecideRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	engine, err := board.ParseEngine(req.Engine)
	if err != nil {
		writeError(w, err)
		return
	}
	answer := board.DecideAnswer{}
	switch engine {
	case board.EngineNative:
		answer.Native = &board.NativeDecision{
			Decisions: req.Decisions, Status: req.Status, Scope: req.Scope,
			ActorID: req.ActorID, Reason: req.Reason, Override: req.Override,
		}
	case board.EngineClaude:
		answer.Claude = &board.AskDecision{Approved: req.Approved, Scope: req.Scope, Choice: req.Choice, Values: req.Values, Answers: req.Answers}
	case board.EngineCodex:
		answer.Codex = &board.AskDecision{Approved: req.Approved, Scope: req.Scope, Choice: req.Choice, Values: req.Values, Answers: req.Answers}
	}
	if err := a.board.DecideInbox(r.Context(), engine, req.TicketID, answer); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GetPlacementPacket 读开回合用的只读 Packet。
func (a *API) GetPlacementPacket(w http.ResponseWriter, r *http.Request) {
	engine, err := board.ParseEngine(chi.URLParam(r, "engine"))
	if err != nil {
		writeError(w, err)
		return
	}
	pkt, err := board.BuildPacket(r.Context(), a.q(r.Context()), engine, chi.URLParam(r, "session_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, PacketResponse{Packet: pkt})
}

// GetSessionLinks 读会话头上的 Issue/PR。
func (a *API) GetSessionLinks(w http.ResponseWriter, r *http.Request) {
	engine, err := board.ParseEngine(chi.URLParam(r, "engine"))
	if err != nil {
		writeError(w, err)
		return
	}
	links, err := board.GetLinks(r.Context(), a.q(r.Context()), engine, chi.URLParam(r, "session_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, LinksResponse{Links: links})
}

// ReplaceSessionLinks 用一批链接收掉会话上的 Issue/PR。种类从链接路径判断，调用方不用分开填。
func (a *API) ReplaceSessionLinks(w http.ResponseWriter, r *http.Request) {
	engine, err := board.ParseEngine(chi.URLParam(r, "engine"))
	if err != nil {
		writeError(w, err)
		return
	}
	var req ReplaceLinksRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	sessionID := chi.URLParam(r, "session_id")
	var issue *board.IssueSnap
	pulls := []board.PullSnap{}
	seen := map[string]bool{}
	for _, raw := range req.Links {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		repo, number, kind, err := github.ParseLink(raw)
		if err != nil {
			writeError(w, cderr.Invalid("%s", err.Error()))
			return
		}
		key := kind + ":" + repo + "#" + strconv.Itoa(number)
		if seen[key] {
			continue
		}
		seen[key] = true
		switch kind {
		case "pull":
			snap, err := a.pullSnap(engine, sessionID, repo, number, raw)
			if err != nil {
				writeError(w, err)
				return
			}
			pulls = append(pulls, snap)
		case "issue":
			if issue != nil {
				writeError(w, cderr.Invalid("一个会话只能挂一条 Issue 链接"))
				return
			}
			snap, err := a.issueSnap(engine, sessionID, repo, number, raw)
			if err != nil {
				writeError(w, err)
				return
			}
			issue = &snap
		default:
			snap, pullErr := a.pullSnap(engine, sessionID, repo, number, raw)
			if pullErr == nil {
				pulls = append(pulls, snap)
				continue
			}
			if issue != nil {
				writeError(w, cderr.Invalid("一个会话只能挂一条 Issue 链接"))
				return
			}
			saved, issueErr := a.issueSnap(engine, sessionID, repo, number, raw)
			if issueErr != nil {
				writeError(w, pullErr)
				return
			}
			issue = &saved
		}
	}
	links, err := board.ReplaceLinks(r.Context(), a.q(r.Context()), engine, sessionID, issue, pulls)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, LinksResponse{Links: links})
}

// issueSnap 拉一条 Issue，缺链接时回填用户输入。
func (a *API) issueSnap(engine board.Engine, sessionID, repo string, number int, raw string) (board.IssueSnap, error) {
	snap, err := viewBoardIssue(repo, number)
	if err != nil {
		return board.IssueSnap{}, cderr.Unavailable("%s", err.Error())
	}
	if snap.URL == "" {
		snap.URL = raw
	}
	snap.Engine = engine
	snap.SessionID = sessionID
	return snap, nil
}

// pullSnap 拉一条 PR，缺链接时回填用户输入。
func (a *API) pullSnap(engine board.Engine, sessionID, repo string, number int, raw string) (board.PullSnap, error) {
	snap, err := viewBoardPull(repo, number)
	if err != nil {
		return board.PullSnap{}, cderr.Unavailable("%s", err.Error())
	}
	if snap.URL == "" {
		snap.URL = raw
	}
	snap.Engine = engine
	snap.SessionID = sessionID
	return snap, nil
}

// PutSessionIssue 用 gh 拉 Issue 快照并挂到会话头。
func (a *API) PutSessionIssue(w http.ResponseWriter, r *http.Request) {
	engine, sessionID, repo, number, err := a.parseLink(r)
	if err != nil {
		writeError(w, err)
		return
	}
	snap, err := viewBoardIssue(repo, number)
	if err != nil {
		writeError(w, cderr.Unavailable("%s", err.Error()))
		return
	}
	snap.Engine = engine
	snap.SessionID = sessionID
	saved, err := board.PutIssue(r.Context(), a.q(r.Context()), snap)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"issue": saved})
}

// DeleteSessionIssue 去掉会话上的 Issue。
func (a *API) DeleteSessionIssue(w http.ResponseWriter, r *http.Request) {
	engine, err := board.ParseEngine(chi.URLParam(r, "engine"))
	if err != nil {
		writeError(w, err)
		return
	}
	if err := board.DeleteIssue(r.Context(), a.q(r.Context()), engine, chi.URLParam(r, "session_id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// PutSessionPull 用 gh 拉 PR 快照并挂到会话头。
func (a *API) PutSessionPull(w http.ResponseWriter, r *http.Request) {
	engine, sessionID, repo, number, err := a.parseLink(r)
	if err != nil {
		writeError(w, err)
		return
	}
	snap, err := viewBoardPull(repo, number)
	if err != nil {
		writeError(w, cderr.Unavailable("%s", err.Error()))
		return
	}
	snap.Engine = engine
	snap.SessionID = sessionID
	saved, err := board.PutPull(r.Context(), a.q(r.Context()), snap)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pull": saved})
}

// DeleteSessionPull 去掉会话上的一条 PR。
func (a *API) DeleteSessionPull(w http.ResponseWriter, r *http.Request) {
	engine, err := board.ParseEngine(chi.URLParam(r, "engine"))
	if err != nil {
		writeError(w, err)
		return
	}
	number, err := strconv.Atoi(chi.URLParam(r, "number"))
	if err != nil {
		writeError(w, cderr.Invalid("invalid pull number"))
		return
	}
	if err := board.DeletePull(r.Context(), a.q(r.Context()), engine, chi.URLParam(r, "session_id"), r.URL.Query().Get("repo"), number); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// parseLink 从路径与 body 收出发动机、会话和 Issue/PR 编号。
func (a *API) parseLink(r *http.Request) (board.Engine, string, string, int, error) {
	engine, err := board.ParseEngine(chi.URLParam(r, "engine"))
	if err != nil {
		return "", "", "", 0, err
	}
	var req PutLinkRequest
	if err := decodeJSON(r, &req); err != nil {
		return "", "", "", 0, err
	}
	repo, number := req.Repo, req.Number
	if req.Ref != "" {
		parsedRepo, parsedN, refErr := github.FormatRef(req.Ref)
		if refErr != nil {
			return "", "", "", 0, cderr.Invalid("%s", refErr.Error())
		}
		if repo == "" {
			repo = parsedRepo
		}
		if number == 0 {
			number = parsedN
		}
	}
	if number <= 0 {
		return "", "", "", 0, cderr.Invalid("issue or pull number is required")
	}
	return engine, chi.URLParam(r, "session_id"), repo, number, nil
}

// publicPlacement 把库内 native 写成前端 agent。
func publicPlacement(place board.Placement) board.Placement {
	place.Engine = board.Engine(board.PublicEngine(place.Engine))
	return place
}

// publicPlacements 批量写成前端引擎名。
func publicPlacements(items []board.Placement) []board.Placement {
	out := make([]board.Placement, 0, len(items))
	for _, item := range items {
		out = append(out, publicPlacement(item))
	}
	return out
}
