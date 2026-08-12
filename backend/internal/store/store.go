// Package store 提供基于 SQLite 的持久化存储。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"trendscope/internal/model"
)

// Store 封装 SQLite 连接的读写操作。
type Store struct {
	db *sql.DB
}

// Open 打开(必要时创建)SQLite 数据库文件并迁移建表。
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite 单写者场景下减少锁竞争,同时允许多个读。
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS snapshots (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    window     TEXT NOT NULL DEFAULT 'day',
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS repos (
    id            INTEGER NOT NULL,
    full_name     TEXT NOT NULL,
    owner         TEXT NOT NULL,
    name          TEXT NOT NULL,
    stars         INTEGER NOT NULL,
    language      TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    html_url      TEXT NOT NULL,
    created_at    DATETIME NOT NULL,
    snapshot_time DATETIME NOT NULL,
    PRIMARY KEY (id, snapshot_time)
);

CREATE INDEX IF NOT EXISTS idx_repos_snapshot ON repos (snapshot_time);
CREATE INDEX IF NOT EXISTS idx_repos_language  ON repos (language);
CREATE INDEX IF NOT EXISTS idx_repos_stars     ON repos (stars);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// 兼容早期版本:若 repos 表仍为旧结构(单列主键),重建为复合主键。
	var tbl string
	if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='repos'`).Scan(&tbl); err != nil {
		return fmt.Errorf("inspect repos table: %w", err)
	}
	if !strings.Contains(tbl, "PRIMARY KEY (id, snapshot_time)") {
		if _, err := s.db.Exec(`DROP TABLE IF EXISTS repos`); err != nil {
			return fmt.Errorf("drop legacy repos: %w", err)
		}
		if _, err := s.db.Exec(schema); err != nil {
			return fmt.Errorf("recreate repos: %w", err)
		}
	}
	return nil
}

// Close 关闭数据库连接。
func (s *Store) Close() error { return s.db.Close() }

// InsertSnapshot 开启一个事务,插入快照索引及其关联仓库记录。
func (s *Store) InsertSnapshot(ctx context.Context, w model.Window, repos []model.Repo) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `INSERT INTO snapshots (window, created_at) VALUES (?, ?)`, string(w), now)
	if err != nil {
		return 0, fmt.Errorf("insert snapshot: %w", err)
	}
	snapshotID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("snapshot id: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO repos (id, full_name, owner, name, stars, language, description, html_url, created_at, snapshot_time)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare repo insert: %w", err)
	}
	defer stmt.Close()

	for _, r := range repos {
		if _, err := stmt.ExecContext(ctx,
			r.ID, r.FullName, r.Owner, r.Name, r.Stars, r.Language, r.Description, r.HTMLURL, r.CreatedAt, now,
		); err != nil {
			// 同一快照内重复抓取(语言交叉)时跳过。
			if isUniqueViolation(err) {
				continue
			}
			return 0, fmt.Errorf("insert repo %s: %w", r.FullName, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return snapshotID, nil
}

// LatestSnapshotTime 返回指定窗口最近一次快照的时间,无数据时返回零值。
func (s *Store) LatestSnapshotTime(ctx context.Context, w model.Window) (time.Time, error) {
	var t time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT created_at FROM snapshots WHERE window = ? ORDER BY created_at DESC LIMIT 1`, string(w)).Scan(&t)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("latest snapshot: %w", err)
	}
	return t, nil
}

// ReposInWindow 返回指定窗口最近一次快照中的仓库列表。
func (s *Store) ReposInWindow(ctx context.Context, w model.Window) ([]model.Repo, error) {
	latest, err := s.LatestSnapshotTime(ctx, w)
	if err != nil {
		return nil, err
	}
	if latest.IsZero() {
		return []model.Repo{}, nil
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, full_name, owner, name, stars, language, description, html_url, created_at, snapshot_time
FROM repos
WHERE snapshot_time = ?
  AND stars > 0
ORDER BY stars DESC
LIMIT 100`, latest)
	if err != nil {
		return nil, fmt.Errorf("query repos: %w", err)
	}
	defer rows.Close()

	var out []model.Repo
	for rows.Next() {
		var r model.Repo
		if err := rows.Scan(&r.ID, &r.FullName, &r.Owner, &r.Name, &r.Stars,
			&r.Language, &r.Description, &r.HTMLURL, &r.CreatedAt, &r.SnapshotTime); err != nil {
			return nil, fmt.Errorf("scan repo: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LanguageScores 返回指定窗口内各语言的活跃度分数,按分数降序排列。
func (s *Store) LanguageScores(ctx context.Context, w model.Window) ([]model.LanguageScore, error) {
	latest, err := s.LatestSnapshotTime(ctx, w)
	if err != nil {
		return nil, err
	}
	if latest.IsZero() {
		return []model.LanguageScore{}, nil
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT language,
       SUM(stars)   AS score,
       COUNT(*)     AS cnt,
       AVG(stars)   AS avg_stars
FROM repos
WHERE snapshot_time = ?
  AND language != ''
  AND stars > 0
GROUP BY language
ORDER BY score DESC
LIMIT 12`, latest)
	if err != nil {
		return nil, fmt.Errorf("query language scores: %w", err)
	}
	defer rows.Close()

	var out []model.LanguageScore
	for rows.Next() {
		var ls model.LanguageScore
		if err := rows.Scan(&ls.Language, &ls.Score, &ls.Count, &ls.AvgStars); err != nil {
			return nil, fmt.Errorf("scan language score: %w", err)
		}
		out = append(out, ls)
	}
	return out, rows.Err()
}

// Languages 返回数据中出现的所有语言列表(去重、按字母排序)。
func (s *Store) Languages(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT language FROM repos WHERE language != '' ORDER BY language`)
	if err != nil {
		return nil, fmt.Errorf("query languages: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return nil, fmt.Errorf("scan language: %w", err)
		}
		out = append(out, l)
	}
	if out == nil {
		out = []string{}
	}
	return out, rows.Err()
}

// isUniqueViolation 判断 SQLite 错误是否为 UNIQUE/主键约束冲突。
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
