// TrendScope API 服务入口:提供 REST API 并托管前端静态文件。
package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"trendscope/internal/api"
	"trendscope/internal/store"
)

//go:embed all:public
var publicFS embed.FS

func main() {
	addr := listenAddr()
	dbPath := envOr("TRENDSCOPE_DB", "trendscope.db")

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	apiServer := api.New(st)

	mux := http.NewServeMux()
	mux.Handle("/", staticHandler())
	mux.Handle("/api/", apiServer.Handler())
	mux.Handle("/healthz", apiServer.Handler())

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		log.Printf("TrendScope API listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func staticHandler() http.Handler {
	sub, err := fs.Sub(publicFS, "public")
	if err != nil {
		log.Fatalf("embed public: %v", err)
	}
	return http.FileServer(http.FS(sub))
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// listenAddr 优先使用 TRENDSCOPE_ADDR;否则跟随 Render/云平台注入的 PORT。
func listenAddr() string {
	if v := os.Getenv("TRENDSCOPE_ADDR"); v != "" {
		return v
	}
	if p := os.Getenv("PORT"); p != "" {
		if p[0] == ':' {
			return p
		}
		return ":" + p
	}
	return ":8080"
}
