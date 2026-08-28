package handler

import (
	"net/http"
	"strconv"
	"strings"

	cderr "codedock/internal/errors"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
	sortAsc         = "asc"
	sortDesc        = "desc"
)

// PageQuery 是列表接口的统一分页参数。
type PageQuery struct {
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// Offset 计算 SQL OFFSET。
func (p PageQuery) Offset() int {
	if p.Page < 1 {
		return 0
	}
	return (p.Page - 1) * p.PageSize
}

// Limit 返回 SQL LIMIT。
func (p PageQuery) Limit() int {
	return p.PageSize
}

// Info 把生效中的分页参数和总数写成响应字段。
func (p PageQuery) Info(total int64) PageInfo {
	return PageInfo{
		Page:      p.Page,
		PageSize:  p.PageSize,
		SortBy:    p.SortBy,
		SortOrder: p.SortOrder,
		Total:     total,
	}
}

// PageInfo 是列表响应中的分页元数据。
type PageInfo struct {
	Page      int    `json:"page"`
	PageSize  int    `json:"page_size"`
	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"`
	Total     int64  `json:"total"`
}

// PageDefaults 描述某一资源的默认排序和允许字段。
type PageDefaults struct {
	SortBy    string
	SortOrder string
	Allowed   []string
}

var (
	sessionPageDefaults = PageDefaults{
		SortBy:    "updated_at",
		SortOrder: sortDesc,
		Allowed:   []string{"updated_at", "created_at"},
	}
	messagePageDefaults = PageDefaults{
		SortBy:    "event_seq",
		SortOrder: sortAsc,
		Allowed:   []string{"event_seq", "created_at"},
	}
	approvalPageDefaults = PageDefaults{
		SortBy:    "id",
		SortOrder: sortAsc,
		Allowed:   []string{"id", "status", "expires_at"},
	}
	usagePageDefaults = PageDefaults{
		SortBy:    "created_at",
		SortOrder: sortAsc,
		Allowed:   []string{"created_at"},
	}
)

// ParsePageQuery 从 query 读取分页参数；缺省或 0 落到默认值。
func ParsePageQuery(r *http.Request, defaults PageDefaults) (PageQuery, error) {
	page := PageQuery{
		Page:      defaultPage,
		PageSize:  defaultPageSize,
		SortBy:    defaults.SortBy,
		SortOrder: defaults.SortOrder,
	}
	query := r.URL.Query()

	if raw := query.Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return PageQuery{}, cderr.Invalid("invalid page")
		}
		if n < 0 {
			return PageQuery{}, cderr.Invalid("page must be >= 0")
		}
		if n > 0 {
			page.Page = n
		}
	}
	if raw := query.Get("page_size"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return PageQuery{}, cderr.Invalid("invalid page_size")
		}
		if n < 0 {
			return PageQuery{}, cderr.Invalid("page_size must be >= 0")
		}
		if n > 0 {
			page.PageSize = n
		}
		if page.PageSize > maxPageSize {
			page.PageSize = maxPageSize
		}
	}
	if raw := query.Get("sort_by"); raw != "" {
		if !allowedSort(raw, defaults.Allowed) {
			return PageQuery{}, cderr.Invalid("invalid sort_by")
		}
		page.SortBy = raw
	}
	if raw := query.Get("sort_order"); raw != "" {
		order := strings.ToLower(raw)
		if order != sortAsc && order != sortDesc {
			return PageQuery{}, cderr.Invalid("invalid sort_order")
		}
		page.SortOrder = order
	}
	return page, nil
}

func allowedSort(field string, allowed []string) bool {
	for _, item := range allowed {
		if item == field {
			return true
		}
	}
	return false
}
