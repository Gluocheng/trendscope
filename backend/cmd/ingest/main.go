// TrendScope 抓取服务入口:定时从 GitHub API 抓取热门仓库写入 SQLite。
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"trendscope/internal/ingestor"
	"trendscope/internal/store"
)

func main() {
	dbPath := envOr("TRENDSCOPE_DB", "trendscope.db")

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ing := ingestor.New(st)
	if err := ing.Run(ctx); err != nil {
		log.Fatalf("ingest: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
