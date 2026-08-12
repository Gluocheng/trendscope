// Package ingestor 从 GitHub API 抓取热门仓库并写入 SQLite。
package ingestor

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"trendscope/internal/model"
	"trendscope/internal/store"
)

// Ingestor 协调抓取任务的执行与调度。
type Ingestor struct {
	store     *store.Store
	client    *Client
	languages []string
	interval  time.Duration
	perPage   int
	maxPages  int
	workers   int
}

// Config 用于构造 Ingestor。
type Config struct {
	Token     string
	Languages []string
	Interval  time.Duration
	PerPage   int
	MaxPages  int
	Workers   int
}

// New 创建 Ingestor,读取环境变量作为默认配置。
func New(st *store.Store) *Ingestor {
	return NewWithConfig(st, configFromEnv())
}

// NewWithConfig 使用显式配置创建 Ingestor。
func NewWithConfig(st *store.Store, cfg Config) *Ingestor {
	langs := cfg.Languages
	if len(langs) == 0 {
		langs = defaultLanguages
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = time.Hour
	}
	return &Ingestor{
		store:     st,
		client:    NewClient(cfg.Token),
		languages: langs,
		interval:  interval,
		perPage:   cfg.PerPage,
		maxPages:  cfg.MaxPages,
		workers:   cfg.Workers,
	}
}

func configFromEnv() Config {
	return Config{
		Token:    os.Getenv("GITHUB_TOKEN"),
		Interval: envDuration("TRENDSCOPE_INTERVAL", time.Hour),
		Workers:  envInt("TRENDSCOPE_WORKERS", 5),
		PerPage:  envInt("TRENDSCOPE_PER_PAGE", 50),
		MaxPages: envInt("TRENDSCOPE_MAX_PAGES", 1),
	}
}

// Run 执行一次抓取并进入定时循环,直到 ctx 被取消。
func (in *Ingestor) Run(ctx context.Context) error {
	ticker := time.NewTicker(in.interval)
	defer ticker.Stop()

	if err := in.IngestAll(ctx); err != nil {
		log.Printf("initial ingest failed: %v", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := in.IngestAll(ctx); err != nil {
				log.Printf("scheduled ingest failed: %v", err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// IngestAll 并发抓取所有窗口与语言,写入快照。
func (in *Ingestor) IngestAll(ctx context.Context) error {
	for _, w := range model.AllWindows() {
		if err := in.ingestWindow(ctx, w); err != nil {
			return fmt.Errorf("ingest window %s: %w", w, err)
		}
	}
	return nil
}

// ingestWindow 抓取单个窗口:按语言并发搜索并写入一个快照。
func (in *Ingestor) ingestWindow(ctx context.Context, w model.Window) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	results := make([][]model.Repo, len(in.languages))
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)
	if in.workers > 0 {
		g.SetLimit(in.workers)
	}

	for i, lang := range in.languages {
		lang, i := lang, i
		g.Go(func() error {
			repos, err := in.client.SearchRepos(gctx, w, lang, in.perPage, in.maxPages)
			if err != nil {
				return fmt.Errorf("search %s: %w", lang, err)
			}
			mu.Lock()
			results[i] = repos
			mu.Unlock()
			log.Printf("window=%s language=%-12s fetched=%d", w, lang, len(repos))
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	var all []model.Repo
	for _, rs := range results {
		all = append(all, rs...)
	}
	if len(all) == 0 {
		log.Printf("window=%s: no repos fetched, skipping snapshot", w)
		return nil
	}

	id, err := in.store.InsertSnapshot(ctx, w, all)
	if err != nil {
		return err
	}
	log.Printf("window=%s snapshot=%d repos=%d", w, id, len(all))
	return nil
}

// envDuration 解析环境变量为 time.Duration,失败返回默认值。
func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
}
