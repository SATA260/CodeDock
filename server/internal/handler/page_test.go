package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	cderr "codedock/internal/errors"
)

func TestParsePageQueryDefaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	page, err := ParsePageQuery(req, sessionPageDefaults)
	if err != nil {
		t.Fatal(err)
	}
	if page.Page != defaultPage || page.PageSize != defaultPageSize {
		t.Fatalf("page=%d size=%d", page.Page, page.PageSize)
	}
	if page.SortBy != "updated_at" || page.SortOrder != sortDesc {
		t.Fatalf("sort=%s %s", page.SortBy, page.SortOrder)
	}
	if page.Offset() != 0 || page.Limit() != defaultPageSize {
		t.Fatalf("offset=%d limit=%d", page.Offset(), page.Limit())
	}
}

func TestParsePageQueryZeroFallsBack(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/sessions?page=0&page_size=0", nil)
	page, err := ParsePageQuery(req, sessionPageDefaults)
	if err != nil {
		t.Fatal(err)
	}
	if page.Page != defaultPage || page.PageSize != defaultPageSize {
		t.Fatalf("page=%d size=%d", page.Page, page.PageSize)
	}
}

func TestParsePageQueryClampAndOffset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/sessions?page=3&page_size=200&sort_by=created_at&sort_order=ASC", nil)
	page, err := ParsePageQuery(req, sessionPageDefaults)
	if err != nil {
		t.Fatal(err)
	}
	if page.Page != 3 || page.PageSize != maxPageSize {
		t.Fatalf("page=%d size=%d", page.Page, page.PageSize)
	}
	if page.SortBy != "created_at" || page.SortOrder != sortAsc {
		t.Fatalf("sort=%s %s", page.SortBy, page.SortOrder)
	}
	if page.Offset() != 2*maxPageSize {
		t.Fatalf("offset=%d", page.Offset())
	}
}

func TestParsePageQueryRejects(t *testing.T) {
	cases := []string{
		"/sessions?page=-1",
		"/sessions?page_size=-2",
		"/sessions?page=x",
		"/sessions?sort_by=password",
		"/sessions?sort_order=random",
	}
	for _, path := range cases {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		_, err := ParsePageQuery(req, sessionPageDefaults)
		if !cderr.IsInvalid(err) {
			t.Fatalf("%s: err=%v", path, err)
		}
	}
}
