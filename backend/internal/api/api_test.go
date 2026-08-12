package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"trendscope/internal/model"
	"trendscope/internal/store"
)

func newTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	srv := New(st)
	srv.now = func() time.Time { return time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC) }
	return srv, st
}

func doRequest(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := doRequest(t, srv.Handler(), http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", rec.Code)
	}
	var body envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, ok := body.Data.(map[string]any)
	if !ok || data["status"] != "ok" {
		t.Errorf("unexpected healthz body: %s", rec.Body.String())
	}
}

func TestReposEmpty(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := doRequest(t, srv.Handler(), http.MethodGet, "/api/repos?window=day")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Data []model.Repo `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data == nil {
		t.Fatal("expected empty array, got null")
	}
}

func TestReposWithData(t *testing.T) {
	srv, st := newTestServer(t)
	ctx := context.Background()
	_, err := st.InsertSnapshot(ctx, model.WindowDay, []model.Repo{{
		ID: 1, FullName: "owner/repo", Owner: "owner", Name: "repo",
		Stars: 100, Language: "Go", HTMLURL: "https://github.com/owner/repo",
		CreatedAt: time.Now().UTC(),
	}})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	rec := doRequest(t, srv.Handler(), http.MethodGet, "/api/repos?window=day")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Data []model.Repo `json:"data"`
		Meta *meta        `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].FullName != "owner/repo" {
		t.Errorf("unexpected repos: %+v", body.Data)
	}
	if body.Meta == nil || body.Meta.Window != "day" {
		t.Errorf("unexpected meta: %+v", body.Meta)
	}
}

func TestRadarWithData(t *testing.T) {
	srv, st := newTestServer(t)
	ctx := context.Background()
	_, err := st.InsertSnapshot(ctx, model.WindowWeek, []model.Repo{
		{ID: 1, FullName: "a", Owner: "a", Name: "a", Stars: 100, Language: "Go", HTMLURL: "u", CreatedAt: time.Now().UTC()},
		{ID: 2, FullName: "b", Owner: "b", Name: "b", Stars: 200, Language: "Python", HTMLURL: "u", CreatedAt: time.Now().UTC()},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	rec := doRequest(t, srv.Handler(), http.MethodGet, "/api/radar?window=week")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Data []model.LanguageScore `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("expected 2 languages, got %d", len(body.Data))
	}
	if body.Data[0].Language != "Python" {
		t.Errorf("expected Python on top, got %+v", body.Data[0])
	}
}

func TestInvalidWindow(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := doRequest(t, srv.Handler(), http.MethodGet, "/api/repos?window=year")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var body envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error == "" {
		t.Error("expected error message")
	}
}

func TestLanguagesEmpty(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := doRequest(t, srv.Handler(), http.MethodGet, "/api/languages")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data == nil {
		t.Fatal("expected empty array, got null")
	}
}
