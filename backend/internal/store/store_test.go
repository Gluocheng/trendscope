package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"trendscope/internal/model"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func testRepo(id int64, name, lang string, stars int) model.Repo {
	return model.Repo{
		ID:          id,
		FullName:    name,
		Owner:       "owner",
		Name:        name,
		Stars:       stars,
		Language:    lang,
		Description: "desc",
		HTMLURL:     "https://github.com/" + name,
		CreatedAt:   time.Now().UTC(),
	}
}

func TestInsertAndQueryRepos(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	repos := []model.Repo{
		testRepo(1, "a/go", "Go", 100),
		testRepo(2, "b/py", "Python", 200),
		testRepo(3, "c/rs", "Rust", 50),
	}
	id, err := st.InsertSnapshot(ctx, model.WindowDay, repos)
	if err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive snapshot id, got %d", id)
	}

	latest, err := st.LatestSnapshotTime(ctx, model.WindowDay)
	if err != nil {
		t.Fatalf("latest snapshot time: %v", err)
	}
	if latest.IsZero() {
		t.Fatal("expected non-zero snapshot time")
	}

	got, err := st.ReposInWindow(ctx, model.WindowDay)
	if err != nil {
		t.Fatalf("repos in window: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 repos, got %d", len(got))
	}
	// 按星标降序
	if got[0].FullName != "b/py" {
		t.Errorf("expected top repo b/py, got %s", got[0].FullName)
	}
}

func TestSnapshotIsolationPerWindow(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	_, err := st.InsertSnapshot(ctx, model.WindowDay, []model.Repo{testRepo(1, "a/go", "Go", 10)})
	if err != nil {
		t.Fatalf("insert day: %v", err)
	}
	_, err = st.InsertSnapshot(ctx, model.WindowMonth, []model.Repo{testRepo(2, "b/py", "Python", 20)})
	if err != nil {
		t.Fatalf("insert month: %v", err)
	}

	dayRepos, err := st.ReposInWindow(ctx, model.WindowDay)
	if err != nil {
		t.Fatal(err)
	}
	if len(dayRepos) != 1 || dayRepos[0].ID != 1 {
		t.Errorf("day window should only contain repo 1, got %+v", dayRepos)
	}

	monthRepos, err := st.ReposInWindow(ctx, model.WindowMonth)
	if err != nil {
		t.Fatal(err)
	}
	if len(monthRepos) != 1 || monthRepos[0].ID != 2 {
		t.Errorf("month window should only contain repo 2, got %+v", monthRepos)
	}

	// week 窗口无数据
	weekRepos, err := st.ReposInWindow(ctx, model.WindowWeek)
	if err != nil {
		t.Fatal(err)
	}
	if len(weekRepos) != 0 {
		t.Errorf("week window should be empty, got %d", len(weekRepos))
	}
}

func TestLanguageScores(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	repos := []model.Repo{
		testRepo(1, "a/go", "Go", 100),
		testRepo(2, "b/go2", "Go", 50),
		testRepo(3, "c/py", "Python", 30),
		testRepo(4, "d/nolang", "", 999), // 无语言,应被过滤
	}
	if _, err := st.InsertSnapshot(ctx, model.WindowWeek, repos); err != nil {
		t.Fatalf("insert: %v", err)
	}

	scores, err := st.LanguageScores(ctx, model.WindowWeek)
	if err != nil {
		t.Fatalf("language scores: %v", err)
	}
	if len(scores) != 2 {
		t.Fatalf("expected 2 languages, got %d", len(scores))
	}
	if scores[0].Language != "Go" || scores[0].Score != 150 || scores[0].Count != 2 {
		t.Errorf("unexpected Go score: %+v", scores[0])
	}
	if scores[1].Language != "Python" || scores[1].Score != 30 {
		t.Errorf("unexpected Python score: %+v", scores[1])
	}
}

func TestDuplicateRepoInSameSnapshotSkipped(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	// 同一仓库 ID 出现两次(语言交叉抓取),应只存一条。
	repos := []model.Repo{
		testRepo(1, "a/dup", "Go", 10),
		testRepo(1, "a/dup", "Go", 10),
	}
	if _, err := st.InsertSnapshot(ctx, model.WindowDay, repos); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := st.ReposInWindow(ctx, model.WindowDay)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 repo after dedup, got %d", len(got))
	}
}
