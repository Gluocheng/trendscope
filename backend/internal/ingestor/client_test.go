package ingestor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"trendscope/internal/model"
)

func TestToRepo(t *testing.T) {
	gr := githubRepo{
		ID:         42,
		FullName:   "owner/repo",
		Name:       "repo",
		Stars:      123,
		Language:   "Go",
		HTMLURL:    "https://github.com/owner/repo",
		CreatedAt:  "2026-08-01T10:00:00Z",
	}
	gr.Owner.Login = "owner"
	r := toRepo(gr)
	if r.ID != 42 || r.FullName != "owner/repo" || r.Stars != 123 || r.Language != "Go" {
		t.Errorf("unexpected repo: %+v", r)
	}
	if r.CreatedAt.IsZero() {
		t.Error("expected parsed created_at")
	}
}

func TestRateLimitInfo(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Add("X-RateLimit-Remaining", "5")
	resp.Header.Add("X-RateLimit-Reset", "1780000000")
	rem, reset, ok := rateLimitInfo(resp)
	if !ok || rem != 5 {
		t.Errorf("expected remaining=5, got %d (ok=%v)", rem, ok)
	}
	if reset.Unix() != 1780000000 {
		t.Errorf("unexpected reset time: %v", reset)
	}
}

func TestRateLimitInfoMissing(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	if _, _, ok := rateLimitInfo(resp); ok {
		t.Error("expected ok=false when headers missing")
	}
}

func TestSearchRepos(t *testing.T) {
	var gotQuery, gotSort, gotOrder string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/repositories" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query()
		gotQuery = q.Get("q")
		gotSort = q.Get("sort")
		gotOrder = q.Get("order")
		if gotSort != "stars" || gotOrder != "desc" {
			t.Errorf("expected sort=stars order=desc, got sort=%s order=%s", gotSort, gotOrder)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"total_count": 2,
			"items": [
				{"id": 1, "full_name": "a/go", "owner": {"login": "a"}, "name": "go",
				 "stargazers_count": 100, "language": "Go", "html_url": "https://github.com/a/go",
				 "created_at": "2026-08-11T00:00:00Z"},
				{"id": 2, "full_name": "b/py", "owner": {"login": "b"}, "name": "py",
				 "stargazers_count": 50, "language": "Python", "html_url": "https://github.com/b/py",
				 "created_at": "2026-08-11T00:00:00Z"}
			]
		}`))
	}))
	defer ts.Close()

	c := &Client{
		http:      ts.Client(),
		userAgent: "TrendScope/0.1",
		baseURL:   ts.URL,
	}
	repos, err := c.SearchRepos(context.Background(), model.WindowWeek, "Go", 5, 1)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	if repos[0].Stars != 100 || repos[0].Language != "Go" {
		t.Errorf("unexpected first repo: %+v", repos[0])
	}
	if !strings.Contains(gotQuery, "language:Go") {
		t.Errorf("query missing language filter: %q", gotQuery)
	}
	if !strings.Contains(gotQuery, "pushed:>") {
		t.Errorf("query missing pushed filter: %q", gotQuery)
	}
	if !strings.Contains(gotQuery, "stars:>50") {
		t.Errorf("query missing stars filter: %q", gotQuery)
	}
}

func TestSearchReposServerErrorRetries(t *testing.T) {
	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total_count": 1, "items": []}`))
	}))
	defer ts.Close()

	c := &Client{
		http:      ts.Client(),
		userAgent: "TrendScope/0.1",
		baseURL:   ts.URL,
	}
	// 窗口过滤不影响重试逻辑,重试发生在 fetchPage 内部。
	_, err := c.SearchRepos(context.Background(), model.WindowDay, "Go", 5, 1)
	if err != nil {
		t.Fatalf("search after retry: %v", err)
	}
	if calls < 2 {
		t.Errorf("expected retry on 500, got %d calls", calls)
	}
}
