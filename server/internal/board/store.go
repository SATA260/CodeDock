package board

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	cderr "codedock/internal/errors"
	"codedock/internal/util"
	"codedock/pkg/db/sqlite"
)

// CreateWork 建一张卡并挂空白 Work 级 Info。
func CreateWork(ctx context.Context, q *sqlite.Queries, tenantID, userID, title string) (Work, error) {
	if q == nil {
		return Work{}, cderr.Invalid("queries required")
	}
	if strings.TrimSpace(userID) == "" {
		return Work{}, cderr.Invalid("user_id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		tenantID = "default"
	}
	title, err := assignWorkTitle(ctx, q, tenantID, userID, title, "")
	if err != nil {
		return Work{}, err
	}
	now := util.FormatTime(util.Now())
	row, err := q.InsertWork(ctx, sqlite.InsertWorkParams{
		ID: util.NewID(), TenantID: tenantID, UserID: userID, Title: title, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return Work{}, err
	}
	if _, err := q.UpsertWorkInfo(ctx, sqlite.UpsertWorkInfoParams{WorkID: row.ID, Checkout: "", Body: "", UpdatedAt: now}); err != nil {
		return Work{}, err
	}
	return mapWork(row), nil
}

// GetWork 读取一张卡。
func GetWork(ctx context.Context, q *sqlite.Queries, workID string) (Work, error) {
	if q == nil {
		return Work{}, cderr.Invalid("queries required")
	}
	if strings.TrimSpace(workID) == "" {
		return Work{}, cderr.Invalid("work_id is required")
	}
	row, err := q.GetWork(ctx, workID)
	if err != nil {
		return Work{}, wrapDB(err)
	}
	return mapWork(row), nil
}

// ListWorks 列出用户的卡，按更新时间倒序。
func ListWorks(ctx context.Context, q *sqlite.Queries, tenantID, userID string) ([]Work, error) {
	if q == nil {
		return nil, cderr.Invalid("queries required")
	}
	if strings.TrimSpace(userID) == "" {
		return nil, cderr.Invalid("user_id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		tenantID = "default"
	}
	rows, err := q.ListWorksByUser(ctx, sqlite.ListWorksByUserParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return nil, err
	}
	out := make([]Work, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapWork(row))
	}
	return out, nil
}

// UpdateWork 改卡片标题。
func UpdateWork(ctx context.Context, q *sqlite.Queries, workID, title string) (Work, error) {
	current, err := GetWork(ctx, q, workID)
	if err != nil {
		return Work{}, err
	}
	title, err = assignWorkTitle(ctx, q, current.TenantID, current.UserID, title, workID)
	if err != nil {
		return Work{}, err
	}
	row, err := q.UpdateWork(ctx, sqlite.UpdateWorkParams{Title: title, UpdatedAt: util.FormatTime(util.Now()), ID: workID})
	if err != nil {
		return Work{}, err
	}
	return mapWork(row), nil
}

// assignWorkTitle 空白标题顺延为未命名(1)、未命名(2)；已有同名则拒绝。exceptID 是改名时自己的卡。
func assignWorkTitle(ctx context.Context, q *sqlite.Queries, tenantID, userID, title, exceptID string) (string, error) {
	title = strings.TrimSpace(title)
	works, err := ListWorks(ctx, q, tenantID, userID)
	if err != nil {
		return "", err
	}
	if title == "" {
		if exceptID != "" {
			return "", cderr.Invalid("title is required")
		}
		title = nextUntitled(works)
	}
	for _, work := range works {
		if work.ID == exceptID {
			continue
		}
		if work.Title == title {
			return "", cderr.Conflict("work title already exists")
		}
	}
	return title, nil
}

// nextUntitled 取最小的未占用序号，从 1 开始。
func nextUntitled(works []Work) string {
	used := map[int]struct{}{}
	for _, work := range works {
		if n, ok := untitledNumber(work.Title); ok {
			used[n] = struct{}{}
		}
	}
	for n := 1; ; n++ {
		if _, ok := used[n]; !ok {
			return fmt.Sprintf("未命名(%d)", n)
		}
	}
}

// untitledNumber 解析未命名(n)。不是这个格式则不算占号。
func untitledNumber(title string) (int, bool) {
	const prefix = "未命名("
	if !strings.HasPrefix(title, prefix) || !strings.HasSuffix(title, ")") {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSuffix(title[len(prefix):], ")"))
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// DeleteWork 删卡并断开归属；会话回未归组，不删会话、不删磁盘。
func DeleteWork(ctx context.Context, q *sqlite.Queries, workID string) error {
	if _, err := GetWork(ctx, q, workID); err != nil {
		return err
	}
	if err := q.DeletePlacementsByWork(ctx, workID); err != nil {
		return err
	}
	if err := q.DeleteWorkInfos(ctx, workID); err != nil {
		return err
	}
	if err := q.DeleteWorkCheckouts(ctx, workID); err != nil {
		return err
	}
	return q.DeleteWork(ctx, workID)
}

// AttachCheckout 把已存在的目录挂到卡上，并建空白目录级 Info。
func AttachCheckout(ctx context.Context, q *sqlite.Queries, workID, path string, kind CheckoutKind) (Checkout, error) {
	if _, err := GetWork(ctx, q, workID); err != nil {
		return Checkout{}, err
	}
	cleaned, err := requireExistingDir(path)
	if err != nil {
		return Checkout{}, err
	}
	if kind == "" {
		kind = CheckoutFolder
	}
	if _, err := ParseCheckoutKind(string(kind)); err != nil {
		return Checkout{}, err
	}
	if _, err := q.GetWorkCheckout(ctx, sqlite.GetWorkCheckoutParams{WorkID: workID, Path: cleaned}); err == nil {
		return Checkout{}, cderr.Conflict("checkout already attached")
	}
	row, err := q.InsertWorkCheckout(ctx, sqlite.InsertWorkCheckoutParams{WorkID: workID, Path: cleaned, Kind: string(kind)})
	if err != nil {
		return Checkout{}, err
	}
	if _, err := q.UpsertWorkInfo(ctx, sqlite.UpsertWorkInfoParams{WorkID: workID, Checkout: cleaned, Body: "", UpdatedAt: util.FormatTime(util.Now())}); err != nil {
		return Checkout{}, err
	}
	touchWork(ctx, q, workID)
	return mapCheckout(row), nil
}

// DetachCheckout 卸下目录：该路径上的会话变成同卡问答。
func DetachCheckout(ctx context.Context, q *sqlite.Queries, workID, path string) error {
	if _, err := GetWork(ctx, q, workID); err != nil {
		return err
	}
	cleaned, err := normalizeCheckoutPath(path)
	if err != nil {
		return err
	}
	if _, err := q.GetWorkCheckout(ctx, sqlite.GetWorkCheckoutParams{WorkID: workID, Path: cleaned}); err != nil {
		return wrapDB(err)
	}
	if err := q.ClearPlacementCheckout(ctx, sqlite.ClearPlacementCheckoutParams{WorkID: workID, Checkout: cleaned}); err != nil {
		return err
	}
	if err := q.DeleteWorkInfo(ctx, sqlite.DeleteWorkInfoParams{WorkID: workID, Checkout: cleaned}); err != nil {
		return err
	}
	if err := q.DeleteWorkCheckout(ctx, sqlite.DeleteWorkCheckoutParams{WorkID: workID, Path: cleaned}); err != nil {
		return err
	}
	touchWork(ctx, q, workID)
	return nil
}

// ListCheckouts 列出卡上已挂目录。
func ListCheckouts(ctx context.Context, q *sqlite.Queries, workID string) ([]Checkout, error) {
	if _, err := GetWork(ctx, q, workID); err != nil {
		return nil, err
	}
	rows, err := q.ListWorkCheckouts(ctx, workID)
	if err != nil {
		return nil, err
	}
	out := make([]Checkout, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapCheckout(row))
	}
	return out, nil
}

// HasCheckout 判断该路径是否仍挂在卡上。
func HasCheckout(ctx context.Context, q *sqlite.Queries, workID, path string) bool {
	cleaned, err := normalizeCheckoutPath(path)
	if err != nil {
		return false
	}
	_, err = q.GetWorkCheckout(ctx, sqlite.GetWorkCheckoutParams{WorkID: workID, Path: cleaned})
	return err == nil
}

// GetInfo 读 Work 级或目录级说明。
func GetInfo(ctx context.Context, q *sqlite.Queries, workID, checkout string) (Info, error) {
	if _, err := GetWork(ctx, q, workID); err != nil {
		return Info{}, err
	}
	key, err := infoCheckoutKey(checkout)
	if err != nil {
		return Info{}, err
	}
	row, err := q.GetWorkInfo(ctx, sqlite.GetWorkInfoParams{WorkID: workID, Checkout: key})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Info{WorkID: workID, Checkout: key}, nil
		}
		return Info{}, err
	}
	return mapInfo(row), nil
}

// PutInfo 覆盖写入说明。
func PutInfo(ctx context.Context, q *sqlite.Queries, workID, checkout, body string) (Info, error) {
	if _, err := GetWork(ctx, q, workID); err != nil {
		return Info{}, err
	}
	key, err := infoCheckoutKey(checkout)
	if err != nil {
		return Info{}, err
	}
	if key != "" && !HasCheckout(ctx, q, workID, key) {
		return Info{}, cderr.Invalid("checkout is not attached to this work")
	}
	row, err := q.UpsertWorkInfo(ctx, sqlite.UpsertWorkInfoParams{WorkID: workID, Checkout: key, Body: body, UpdatedAt: util.FormatTime(util.Now())})
	if err != nil {
		return Info{}, err
	}
	touchWork(ctx, q, workID)
	return mapInfo(row), nil
}

// Bind 把一路已建会话挂到卡上。目录属于会话：有路径就记到会话上，空路径沿用会话已有目录。
func Bind(ctx context.Context, q *sqlite.Queries, place Placement) (Placement, error) {
	if q == nil {
		return Placement{}, cderr.Invalid("queries required")
	}
	if _, err := GetWork(ctx, q, place.WorkID); err != nil {
		return Placement{}, err
	}
	if strings.TrimSpace(place.SessionID) == "" {
		return Placement{}, cderr.Invalid("session_id is required")
	}
	if place.Engine == "" {
		place.Engine = EngineNative
	}
	engine, err := ParseEngine(string(place.Engine))
	if err != nil {
		return Placement{}, err
	}
	place.Engine = engine
	checkout, err := sessionCheckout(ctx, q, engine, place.SessionID, place.Checkout)
	if err != nil {
		return Placement{}, err
	}
	if _, err := GetPlacement(ctx, q, engine, place.SessionID); err == nil {
		row, err := q.UpdateSessionPlacement(ctx, sqlite.UpdateSessionPlacementParams{
			WorkID: place.WorkID, Checkout: checkout, Engine: string(engine), SessionID: place.SessionID,
		})
		if err != nil {
			return Placement{}, err
		}
		touchWork(ctx, q, place.WorkID)
		return mapPlacement(row), nil
	}
	row, err := q.InsertSessionPlacement(ctx, sqlite.InsertSessionPlacementParams{
		Engine: string(engine), SessionID: place.SessionID, WorkID: place.WorkID, Checkout: checkout,
	})
	if err != nil {
		return Placement{}, err
	}
	touchWork(ctx, q, place.WorkID)
	return mapPlacement(row), nil
}

// SetSessionDirectory 在创建会话时记下目录。空路径不能解绑；已有目录不能换成另一个。已归组时同步 placement.checkout。
func SetSessionDirectory(ctx context.Context, q *sqlite.Queries, engine Engine, sessionID, path string) (string, error) {
	if q == nil {
		return "", cderr.Invalid("queries required")
	}
	engine, err := ParseEngine(string(engine))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(sessionID) == "" {
		return "", cderr.Invalid("session_id is required")
	}
	if strings.TrimSpace(path) == "" {
		return "", cderr.Invalid("directory cannot be unbound")
	}
	cleaned, err := requireExistingDir(path)
	if err != nil {
		return "", err
	}
	if current, getErr := GetSessionDirectory(ctx, q, engine, sessionID); getErr == nil && current != "" && current != cleaned {
		return "", cderr.Conflict("directory is bound when the session is created")
	} else if getErr != nil && !cderr.IsNotFound(getErr) {
		return "", getErr
	}
	if _, err := q.UpsertSessionDirectory(ctx, sqlite.UpsertSessionDirectoryParams{
		Engine: string(engine), SessionID: sessionID, Path: cleaned,
	}); err != nil {
		return "", err
	}
	if place, err := GetPlacement(ctx, q, engine, sessionID); err == nil {
		if _, err := q.UpdateSessionPlacement(ctx, sqlite.UpdateSessionPlacementParams{
			WorkID: place.WorkID, Checkout: cleaned, Engine: string(engine), SessionID: sessionID,
		}); err != nil {
			return "", err
		}
		touchWork(ctx, q, place.WorkID)
	}
	return cleaned, nil
}

// GetSessionDirectory 读会话自己的目录；没有行表示还没绑定。
func GetSessionDirectory(ctx context.Context, q *sqlite.Queries, engine Engine, sessionID string) (string, error) {
	if q == nil {
		return "", cderr.Invalid("queries required")
	}
	engine, err := ParseEngine(string(engine))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(sessionID) == "" {
		return "", cderr.Invalid("session_id is required")
	}
	row, err := q.GetSessionDirectory(ctx, sqlite.GetSessionDirectoryParams{Engine: string(engine), SessionID: sessionID})
	if err != nil {
		return "", wrapDB(err)
	}
	return row.Path, nil
}

// sessionCheckout 解析要写入归属的目录：显式路径必须存在，空则沿用会话已绑定的目录。
func sessionCheckout(ctx context.Context, q *sqlite.Queries, engine Engine, sessionID, checkout string) (string, error) {
	if strings.TrimSpace(checkout) != "" {
		cleaned, err := requireExistingDir(checkout)
		if err != nil {
			return "", err
		}
		if _, err := q.UpsertSessionDirectory(ctx, sqlite.UpsertSessionDirectoryParams{
			Engine: string(engine), SessionID: sessionID, Path: cleaned,
		}); err != nil {
			return "", err
		}
		return cleaned, nil
	}
	path, err := GetSessionDirectory(ctx, q, engine, sessionID)
	if err == nil {
		return path, nil
	}
	if cderr.IsNotFound(err) {
		return "", nil
	}
	return "", err
}

// requireExistingDir 把路径收成绝对目录，必须已经存在。
func requireExistingDir(path string) (string, error) {
	cleaned, err := normalizeCheckoutPath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", cderr.Invalid("checkout path must exist")
	}
	if !info.IsDir() {
		return "", cderr.Invalid("checkout path must be a directory")
	}
	return cleaned, nil
}

// AttachExisting 把未归组会话补挂到卡上，只能当问答。
func AttachExisting(ctx context.Context, q *sqlite.Queries, engine Engine, sessionID, workID string) (Placement, error) {
	if _, err := GetPlacement(ctx, q, engine, sessionID); err == nil {
		return Placement{}, cderr.Conflict("session already placed")
	}
	return Bind(ctx, q, Placement{Engine: engine, SessionID: sessionID, WorkID: workID})
}

// Unbind 断开归属，会话回未归组。
func Unbind(ctx context.Context, q *sqlite.Queries, engine Engine, sessionID string) error {
	if _, err := GetPlacement(ctx, q, engine, sessionID); err != nil {
		return err
	}
	return q.DeleteSessionPlacement(ctx, sqlite.DeleteSessionPlacementParams{Engine: string(engine), SessionID: sessionID})
}

// GetPlacement 读一路会话的归属；没有行表示未归组。
func GetPlacement(ctx context.Context, q *sqlite.Queries, engine Engine, sessionID string) (Placement, error) {
	if q == nil {
		return Placement{}, cderr.Invalid("queries required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return Placement{}, cderr.Invalid("session_id is required")
	}
	row, err := q.GetSessionPlacement(ctx, sqlite.GetSessionPlacementParams{Engine: string(engine), SessionID: sessionID})
	if err != nil {
		return Placement{}, wrapDB(err)
	}
	return mapPlacement(row), nil
}

// ListPlacements 列出一张卡下的全部归属。
func ListPlacements(ctx context.Context, q *sqlite.Queries, workID string) ([]Placement, error) {
	if _, err := GetWork(ctx, q, workID); err != nil {
		return nil, err
	}
	rows, err := q.ListPlacementsByWork(ctx, workID)
	if err != nil {
		return nil, err
	}
	return mapPlacements(rows), nil
}

// ListUserPlacements 列出用户所有卡上的归属，给侧栏分组。
func ListUserPlacements(ctx context.Context, q *sqlite.Queries, tenantID, userID string) ([]Placement, error) {
	if q == nil {
		return nil, cderr.Invalid("queries required")
	}
	if strings.TrimSpace(userID) == "" {
		return nil, cderr.Invalid("user_id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		tenantID = "default"
	}
	rows, err := q.ListPlacementsByUser(ctx, sqlite.ListPlacementsByUserParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return nil, err
	}
	return mapPlacements(rows), nil
}

// GetLinks 读会话头上的 Issue/PR 快照。
func GetLinks(ctx context.Context, q *sqlite.Queries, engine Engine, sessionID string) (SessionLinks, error) {
	if q == nil {
		return SessionLinks{}, cderr.Invalid("queries required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return SessionLinks{}, cderr.Invalid("session_id is required")
	}
	out := SessionLinks{Pulls: []PullSnap{}}
	if issue, err := q.GetSessionIssue(ctx, sqlite.GetSessionIssueParams{Engine: string(engine), SessionID: sessionID}); err == nil {
		snap := mapIssue(issue)
		out.Issue = &snap
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SessionLinks{}, err
	}
	rows, err := q.ListSessionPulls(ctx, sqlite.ListSessionPullsParams{Engine: string(engine), SessionID: sessionID})
	if err != nil {
		return SessionLinks{}, err
	}
	for _, row := range rows {
		out.Pulls = append(out.Pulls, mapPull(row))
	}
	return out, nil
}

// PutIssue 写入 Issue 快照。
func PutIssue(ctx context.Context, q *sqlite.Queries, snap IssueSnap) (IssueSnap, error) {
	if q == nil {
		return IssueSnap{}, cderr.Invalid("queries required")
	}
	if strings.TrimSpace(snap.SessionID) == "" {
		return IssueSnap{}, cderr.Invalid("session_id is required")
	}
	if snap.Number <= 0 {
		return IssueSnap{}, cderr.Invalid("issue number is required")
	}
	row, err := q.UpsertSessionIssue(ctx, sqlite.UpsertSessionIssueParams{
		Engine: string(snap.Engine), SessionID: snap.SessionID, Repo: strings.TrimSpace(snap.Repo),
		Number: int64(snap.Number), Title: snap.Title, Body: snap.Body, Url: snap.URL, State: snap.State,
		UpdatedAt: util.FormatTime(util.Now()),
	})
	if err != nil {
		return IssueSnap{}, err
	}
	return mapIssue(row), nil
}

// DeleteIssue 去掉会话上的 Issue。
func DeleteIssue(ctx context.Context, q *sqlite.Queries, engine Engine, sessionID string) error {
	if q == nil {
		return cderr.Invalid("queries required")
	}
	return q.DeleteSessionIssue(ctx, sqlite.DeleteSessionIssueParams{Engine: string(engine), SessionID: sessionID})
}

// PutPull 写入或覆盖一条 PR 快照。
func PutPull(ctx context.Context, q *sqlite.Queries, snap PullSnap) (PullSnap, error) {
	if q == nil {
		return PullSnap{}, cderr.Invalid("queries required")
	}
	if strings.TrimSpace(snap.SessionID) == "" {
		return PullSnap{}, cderr.Invalid("session_id is required")
	}
	if snap.Number <= 0 {
		return PullSnap{}, cderr.Invalid("pull number is required")
	}
	row, err := q.UpsertSessionPull(ctx, sqlite.UpsertSessionPullParams{
		Engine: string(snap.Engine), SessionID: snap.SessionID, Repo: strings.TrimSpace(snap.Repo),
		Number: int64(snap.Number), Title: snap.Title, Body: snap.Body, Url: snap.URL, State: snap.State,
		UpdatedAt: util.FormatTime(util.Now()),
	})
	if err != nil {
		return PullSnap{}, err
	}
	return mapPull(row), nil
}

// ReplaceLinks 用一批快照换掉会话上的全部 Issue/PR。
func ReplaceLinks(ctx context.Context, q *sqlite.Queries, engine Engine, sessionID string, issue *IssueSnap, pulls []PullSnap) (SessionLinks, error) {
	if q == nil {
		return SessionLinks{}, cderr.Invalid("queries required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return SessionLinks{}, cderr.Invalid("session_id is required")
	}
	if err := DeleteIssue(ctx, q, engine, sessionID); err != nil {
		return SessionLinks{}, err
	}
	if err := q.DeleteSessionPulls(ctx, sqlite.DeleteSessionPullsParams{Engine: string(engine), SessionID: sessionID}); err != nil {
		return SessionLinks{}, err
	}
	out := SessionLinks{Pulls: []PullSnap{}}
	if issue != nil {
		saved, err := PutIssue(ctx, q, *issue)
		if err != nil {
			return SessionLinks{}, err
		}
		out.Issue = &saved
	}
	for _, pull := range pulls {
		saved, err := PutPull(ctx, q, pull)
		if err != nil {
			return SessionLinks{}, err
		}
		out.Pulls = append(out.Pulls, saved)
	}
	return out, nil
}

// DeletePull 去掉会话上的一条 PR。
func DeletePull(ctx context.Context, q *sqlite.Queries, engine Engine, sessionID, repo string, number int) error {
	if q == nil {
		return cderr.Invalid("queries required")
	}
	return q.DeleteSessionPull(ctx, sqlite.DeleteSessionPullParams{
		Engine: string(engine), SessionID: sessionID, Repo: repo, Number: int64(number),
	})
}

// mapWork 把 sqlc 行收成领域对象。
func mapWork(row sqlite.Work) Work {
	return Work{ID: row.ID, TenantID: row.TenantID, UserID: row.UserID, Title: row.Title, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

// mapCheckout 把目录行收成 Checkout。
func mapCheckout(row sqlite.WorkCheckout) Checkout {
	return Checkout{WorkID: row.WorkID, Path: row.Path, Kind: CheckoutKind(row.Kind)}
}

// mapInfo 把说明行收成 Info。
func mapInfo(row sqlite.WorkInfo) Info {
	return Info{WorkID: row.WorkID, Checkout: row.Checkout, Body: row.Body, UpdatedAt: row.UpdatedAt}
}

// mapPlacement 把归属行收成 Placement。
func mapPlacement(row sqlite.SessionPlacement) Placement {
	return Placement{Engine: Engine(row.Engine), SessionID: row.SessionID, WorkID: row.WorkID, Checkout: row.Checkout}
}

// mapPlacements 批量映射归属。
func mapPlacements(rows []sqlite.SessionPlacement) []Placement {
	out := make([]Placement, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapPlacement(row))
	}
	return out
}

// mapIssue 把 Issue 行收成快照。
func mapIssue(row sqlite.SessionIssue) IssueSnap {
	return IssueSnap{Engine: Engine(row.Engine), SessionID: row.SessionID, Repo: row.Repo, Number: int(row.Number), Title: row.Title, Body: row.Body, URL: row.Url, State: row.State, UpdatedAt: row.UpdatedAt}
}

// mapPull 把 PR 行收成快照。
func mapPull(row sqlite.SessionPull) PullSnap {
	return PullSnap{Engine: Engine(row.Engine), SessionID: row.SessionID, Repo: row.Repo, Number: int(row.Number), Title: row.Title, Body: row.Body, URL: row.Url, State: row.State, UpdatedAt: row.UpdatedAt}
}

// wrapDB 把 sql.ErrNoRows 收成 NotFound。
func wrapDB(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return cderr.NotFound("%s", err.Error())
	}
	return err
}

// touchWork 刷新卡片 updated_at。
func touchWork(ctx context.Context, q *sqlite.Queries, workID string) {
	if q == nil || workID == "" {
		return
	}
	_ = q.TouchWork(ctx, sqlite.TouchWorkParams{UpdatedAt: util.FormatTime(util.Now()), ID: workID})
}

// normalizeCheckoutPath 收成绝对干净路径。
func normalizeCheckoutPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", cderr.Invalid("checkout path is required")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", cderr.Invalid("invalid checkout path")
	}
	return filepath.Clean(abs), nil
}

// infoCheckoutKey 空字符串表示 Work 级 Info。
func infoCheckoutKey(checkout string) (string, error) {
	trimmed := strings.TrimSpace(checkout)
	if trimmed == "" {
		return "", nil
	}
	return normalizeCheckoutPath(trimmed)
}
