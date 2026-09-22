package board

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	cderr "codedock/internal/errors"
	"codedock/pkg/agent"
	"codedock/pkg/db/sqlite"
)

const packetBanner = "CodeDock Work Packet — 只读，不要改写这段，也不要抄进记忆。"

// Ports 是看板编排要问的外部能力：建会话、现问 Git、gh 快照、三引擎问票。
type Ports struct {
	StartNative func(ctx context.Context, spec StartSpec) (Started, error)
	StartClaude func(ctx context.Context, spec StartSpec) (Started, error)
	StartCodex  func(ctx context.Context, spec StartSpec) (Started, error)
	Lookup      func(ctx context.Context, engine Engine, sessionID string) (SessionMeta, error)
	ListEngine  func(ctx context.Context, engine Engine) ([]SessionMeta, error)
	InspectDir  func(path string) DirView
	ViewIssue   func(repo string, number int) (IssueSnap, error)
	ViewPull    func(repo string, number int) (PullSnap, error)
	ListAsks    func(engine Engine, sessionID string) []InboxItem
	Decide      func(ctx context.Context, engine Engine, ticketID string, answer DecideAnswer) error
	ApplyDir    func(ctx context.Context, engine Engine, sessionID, path string) error
}

// Service 编排 Work / 看板聚合 / Inbox；不写记忆、不 spawn CLI、不建 worktree。
type Service struct {
	q     *sqlite.Queries
	ports Ports
}

// New 构造看板服务。
func New(q *sqlite.Queries, ports Ports) *Service {
	return &Service{q: q, ports: ports}
}

// StartTalk 先走现有建会话，再写成问答归属。
func (s *Service) StartTalk(ctx context.Context, workID string, spec StartSpec) (Placement, Started, error) {
	if s == nil || s.q == nil {
		return Placement{}, Started{}, cderr.Invalid("board service required")
	}
	spec.Talk = true
	spec.WorkspaceID = ""
	started, err := s.startSession(ctx, spec)
	if err != nil {
		return Placement{}, Started{}, err
	}
	place, err := Bind(ctx, s.q, Placement{Engine: spec.Engine, SessionID: started.ID, WorkID: workID})
	return place, started, err
}

// StartInDir 开会话，并把这个已存在的目录绑到新会话上。
func (s *Service) StartInDir(ctx context.Context, workID, path string, spec StartSpec) (Placement, Started, error) {
	if s == nil || s.q == nil {
		return Placement{}, Started{}, cderr.Invalid("board service required")
	}
	cleaned, err := requireExistingDir(path)
	if err != nil {
		return Placement{}, Started{}, err
	}
	spec.Talk = false
	spec.WorkspaceID = cleaned
	started, err := s.startSession(ctx, spec)
	if err != nil {
		return Placement{}, Started{}, err
	}
	place, err := Bind(ctx, s.q, Placement{Engine: spec.Engine, SessionID: started.ID, WorkID: workID, Checkout: cleaned})
	return place, started, err
}

// BindSessionDirectory 把目录绑到已有会话上。path 为空则解绑。非空时再交给引擎改工作目录。
func (s *Service) BindSessionDirectory(ctx context.Context, engine Engine, sessionID, path string) (string, error) {
	if s == nil || s.q == nil {
		return "", cderr.Invalid("board service required")
	}
	cleaned, err := SetSessionDirectory(ctx, s.q, engine, sessionID, path)
	if err != nil {
		return "", err
	}
	if cleaned != "" && s.ports.ApplyDir != nil {
		if err := s.ports.ApplyDir(ctx, engine, sessionID, cleaned); err != nil {
			return "", err
		}
	}
	return cleaned, nil
}

// startSession 按引擎走已注入的建会话口。
func (s *Service) startSession(ctx context.Context, spec StartSpec) (Started, error) {
	engine, err := ParseEngine(string(spec.Engine))
	if err != nil {
		return Started{}, err
	}
	spec.Engine = engine
	var fn func(context.Context, StartSpec) (Started, error)
	switch engine {
	case EngineNative:
		fn = s.ports.StartNative
	case EngineClaude:
		fn = s.ports.StartClaude
	case EngineCodex:
		fn = s.ports.StartCodex
	}
	if fn == nil {
		return Started{}, cderr.Unavailable("session starter for %s is not configured", PublicEngine(engine))
	}
	return fn(ctx, spec)
}

// Cards 聚合用户的横向看板，只带摘要不加载对话正文。
func (s *Service) Cards(ctx context.Context, tenantID, userID string) (BoardView, error) {
	if s == nil || s.q == nil {
		return BoardView{}, cderr.Invalid("board service required")
	}
	works, err := ListWorks(ctx, s.q, tenantID, userID)
	if err != nil {
		return BoardView{}, err
	}
	view := BoardView{Cards: make([]Card, 0, len(works)), Ungrouped: []SessionView{}}
	placed := map[string]struct{}{}
	for _, work := range works {
		card, err := s.Card(ctx, work.ID)
		if err != nil {
			return BoardView{}, err
		}
		view.Cards = append(view.Cards, card)
		for _, sess := range card.Sessions {
			placed[sess.Engine+":"+sess.SessionID] = struct{}{}
		}
	}
	ungrouped, err := s.listUngrouped(ctx, tenantID, userID, placed)
	if err != nil {
		return BoardView{}, err
	}
	view.Ungrouped = ungrouped
	return view, nil
}

// Card 聚合一张卡的说明与会话摘要。目录挂在会话上，不挂在卡上。
func (s *Service) Card(ctx context.Context, workID string) (Card, error) {
	if s == nil || s.q == nil {
		return Card{}, cderr.Invalid("board service required")
	}
	work, err := GetWork(ctx, s.q, workID)
	if err != nil {
		return Card{}, err
	}
	info, err := GetInfo(ctx, s.q, workID, "")
	if err != nil {
		return Card{}, err
	}
	card := Card{Work: work, Info: info, Dirs: []DirView{}, Sessions: []SessionView{}}
	places, err := ListPlacements(ctx, s.q, workID)
	if err != nil {
		return Card{}, err
	}
	for _, place := range places {
		view := s.sessionView(ctx, place)
		card.Sessions = append(card.Sessions, view)
		if view.Running {
			card.Running++
		}
		card.Pending += view.Pending
	}
	sort.SliceStable(card.Sessions, func(i, j int) bool { return card.Sessions[i].UpdatedAt > card.Sessions[j].UpdatedAt })
	return card, nil
}

// sessionView 读一路会话的看板摘要，目录取会话自己的绑定。
func (s *Service) sessionView(ctx context.Context, place Placement) SessionView {
	view := SessionView{Engine: PublicEngine(place.Engine), SessionID: place.SessionID, Checkout: place.Checkout}
	if s.ports.Lookup != nil {
		if meta, err := s.ports.Lookup(ctx, place.Engine, place.SessionID); err == nil {
			view.Summary = meta.Summary
			view.UpdatedAt = meta.UpdatedAt
			view.Running = meta.Running
			view.Pending = meta.Pending
			s.annotateDirectory(ctx, place.Engine, &view)
			return view
		}
	}
	if place.Engine == EngineNative {
		if row, err := s.q.GetSession(ctx, place.SessionID); err == nil {
			view.Summary = row.Summary
			view.UpdatedAt = row.UpdatedAt
			view.Running = row.ActiveRunID.Valid && row.ActiveRunID.String != ""
			if n, err := s.q.CountPendingApprovalsBySession(ctx, place.SessionID); err == nil {
				view.Pending = int(n)
			}
		}
	}
	s.annotateDirectory(ctx, place.Engine, &view)
	return view
}

// annotateDirectory 用会话目录盖过归属上的路径，并现问 Git 状态。
func (s *Service) annotateDirectory(ctx context.Context, engine Engine, view *SessionView) {
	if view == nil || s == nil || s.q == nil {
		return
	}
	if path, err := GetSessionDirectory(ctx, s.q, engine, view.SessionID); err == nil && path != "" {
		view.Checkout = path
	}
	if view.Checkout == "" || s.ports.InspectDir == nil {
		return
	}
	inspected := s.ports.InspectDir(view.Checkout)
	view.Branch = inspected.Branch
	view.Dirty = inspected.Dirty
}

// listUngrouped 列出没有 placement 的三引擎会话。
func (s *Service) listUngrouped(ctx context.Context, tenantID, userID string, placed map[string]struct{}) ([]SessionView, error) {
	var out []SessionView
	rows, err := s.q.ListUngroupedNativeSessions(ctx, sqlite.ListUngroupedNativeSessionsParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		view := SessionView{
			Engine: PublicEngine(EngineNative), SessionID: row.ID, Summary: row.Summary, UpdatedAt: row.UpdatedAt,
			Running: row.ActiveRunID.Valid && row.ActiveRunID.String != "",
		}
		if n, err := s.q.CountPendingApprovalsBySession(ctx, row.ID); err == nil {
			view.Pending = int(n)
		}
		s.annotateDirectory(ctx, EngineNative, &view)
		out = append(out, view)
	}
	for _, engine := range []Engine{EngineClaude, EngineCodex} {
		if s.ports.ListEngine == nil {
			continue
		}
		metas, err := s.ports.ListEngine(ctx, engine)
		if err != nil {
			continue
		}
		for _, meta := range metas {
			if meta.Archived {
				continue
			}
			if _, ok := placed[PublicEngine(engine)+":"+meta.ID]; ok {
				continue
			}
			view := SessionView{
				Engine: PublicEngine(engine), SessionID: meta.ID, Summary: meta.Summary,
				UpdatedAt: meta.UpdatedAt, Running: meta.Running, Pending: meta.Pending,
			}
			s.annotateDirectory(ctx, engine, &view)
			out = append(out, view)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

// ListInbox 扫该卡下三引擎待批。
func (s *Service) ListInbox(ctx context.Context, workID string) ([]InboxItem, error) {
	if s == nil || s.q == nil {
		return nil, cderr.Invalid("board service required")
	}
	places, err := ListPlacements(ctx, s.q, workID)
	if err != nil {
		return nil, err
	}
	var items []InboxItem
	for _, place := range places {
		if place.Engine == EngineNative {
			items = append(items, s.nativeInbox(ctx, place.SessionID)...)
			continue
		}
		if s.ports.ListAsks != nil {
			items = append(items, s.ports.ListAsks(place.Engine, place.SessionID)...)
		}
	}
	if items == nil {
		items = []InboxItem{}
	}
	return items, nil
}

// DecideInbox 按引擎把裁决转给已有审批入口。
func (s *Service) DecideInbox(ctx context.Context, engine Engine, ticketID string, answer DecideAnswer) error {
	if s == nil {
		return cderr.Invalid("board service required")
	}
	if ticketID == "" {
		return cderr.Invalid("ticket_id is required")
	}
	if s.ports.Decide == nil {
		return cderr.Unavailable("inbox decide is not configured")
	}
	return s.ports.Decide(ctx, engine, ticketID, answer)
}

// nativeInbox 扫自有 Agent 待批，payload 保持原审批。
func (s *Service) nativeInbox(ctx context.Context, sessionID string) []InboxItem {
	rows, err := s.q.ListPendingApprovalsBySession(ctx, sessionID)
	if err != nil {
		return nil
	}
	items := make([]InboxItem, 0, len(rows))
	for _, row := range rows {
		raw, _ := json.Marshal(map[string]any{
			"id": row.ID, "session_id": row.SessionID, "run_id": row.RunID,
			"status": row.Status, "kind": row.Kind, "tool_calls": json.RawMessage(row.ToolCalls),
		})
		items = append(items, InboxItem{
			Engine: PublicEngine(EngineNative), SessionID: sessionID, TicketID: row.ID,
			Summary: string(agent.ApprovalKind(row.Kind)), Payload: raw,
		})
	}
	return items
}

// BuildPacket 组装 Work Info、目录 Info、Issue/PR 快照。
func BuildPacket(ctx context.Context, q *sqlite.Queries, engine Engine, sessionID string) (Packet, error) {
	if q == nil {
		return Packet{}, nil
	}
	pkt := Packet{Pulls: []PullSnap{}}
	dirPath := ""
	if path, err := GetSessionDirectory(ctx, q, engine, sessionID); err == nil {
		dirPath = path
	}
	if place, err := GetPlacement(ctx, q, engine, sessionID); err == nil {
		if info, infoErr := GetInfo(ctx, q, place.WorkID, ""); infoErr == nil {
			pkt.WorkInfo = info
		}
		if dirPath == "" {
			dirPath = place.Checkout
		}
		if dirPath != "" {
			dir, dirErr := GetInfo(ctx, q, place.WorkID, dirPath)
			if dirErr != nil {
				dir = Info{WorkID: place.WorkID, Checkout: dirPath}
			}
			dir.Checkout = dirPath
			pkt.DirInfo = &dir
		}
	} else if dirPath != "" {
		pkt.DirInfo = &Info{Checkout: dirPath}
	}
	if links, err := GetLinks(ctx, q, engine, sessionID); err == nil {
		pkt.Issue = links.Issue
		pkt.Pulls = links.Pulls
	}
	pkt.Text = FormatPacket(pkt)
	return pkt, nil
}

// FormatPacket 把 Packet 收成独立 system / 只读前缀文本。
func FormatPacket(pkt Packet) string {
	var b strings.Builder
	b.WriteString(packetBanner)
	wrote := false
	if strings.TrimSpace(pkt.WorkInfo.Body) != "" {
		b.WriteString("\n\n## Work\n")
		b.WriteString(strings.TrimSpace(pkt.WorkInfo.Body))
		wrote = true
	}
	if pkt.DirInfo != nil && strings.TrimSpace(pkt.DirInfo.Checkout) != "" {
		b.WriteString("\n\n## 目录 ")
		b.WriteString(pkt.DirInfo.Checkout)
		if body := strings.TrimSpace(pkt.DirInfo.Body); body != "" {
			b.WriteByte('\n')
			b.WriteString(body)
		}
		wrote = true
	}
	if pkt.Issue != nil && (pkt.Issue.Number > 0 || pkt.Issue.Title != "") {
		b.WriteString("\n\n## Issue")
		if pkt.Issue.Repo != "" {
			b.WriteByte(' ')
			b.WriteString(pkt.Issue.Repo)
		}
		b.WriteString(fmt.Sprintf(" #%d %s", pkt.Issue.Number, pkt.Issue.Title))
		if pkt.Issue.State != "" {
			b.WriteString(" (" + pkt.Issue.State + ")")
		}
		if strings.TrimSpace(pkt.Issue.Body) != "" {
			b.WriteByte('\n')
			b.WriteString(strings.TrimSpace(pkt.Issue.Body))
		}
		wrote = true
	}
	for _, pull := range pkt.Pulls {
		b.WriteString("\n\n## Pull request")
		if pull.Repo != "" {
			b.WriteByte(' ')
			b.WriteString(pull.Repo)
		}
		b.WriteString(fmt.Sprintf(" #%d %s", pull.Number, pull.Title))
		if pull.State != "" {
			b.WriteString(" (" + pull.State + ")")
		}
		if strings.TrimSpace(pull.Body) != "" {
			b.WriteByte('\n')
			b.WriteString(strings.TrimSpace(pull.Body))
		}
		wrote = true
	}
	if !wrote {
		return ""
	}
	return strings.TrimSpace(b.String())
}

// PrefixContent 把 Packet 标成只读前缀再交给 Claude / Codex 现有 turn API。
func PrefixContent(packet, content string) string {
	packet = strings.TrimSpace(packet)
	content = strings.TrimSpace(content)
	if packet == "" {
		return content
	}
	if content == "" {
		return packet
	}
	return packet + "\n\n---\n\n" + content
}
