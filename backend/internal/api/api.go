// Package api 提供 TrendScope 的 REST API 处理器。
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"trendscope/internal/model"
	"trendscope/internal/store"
)

// Server 持有 API 所需依赖。
type Server struct {
	store *store.Store
	now   func() time.Time
}

// New 创建 API Server。
func New(st *store.Store) *Server {
	return &Server{store: st, now: time.Now}
}

// Handler 返回注册了所有路由的 http.Handler。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/languages", s.handleLanguages)
	mux.HandleFunc("GET /api/repos", s.handleRepos)
	mux.HandleFunc("GET /api/radar", s.handleRadar)
	return mux
}

// JSON 统一响应结构。
type envelope struct {
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
	Meta  *meta  `json:"meta,omitempty"`
}

type meta struct {
	Window      string `json:"window,omitempty"`
	SnapshotAt  string `json:"snapshot_at,omitempty"`
	Total       int    `json:"total,omitempty"`
	RequestedAt string `json:"requested_at,omitempty"`
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, body envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write json: %v", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, envelope{Error: msg})
}

// parseWindow 解析 window 查询参数,缺省为 day。
func (s *Server) parseWindow(r *http.Request) (model.Window, error) {
	raw := r.URL.Query().Get("window")
	if raw == "" {
		return model.WindowDay, nil
	}
	return model.ParseWindow(raw)
}

// GET /healthz
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, envelope{Data: map[string]string{"status": "ok"}})
}

// GET /api/languages
func (s *Server) handleLanguages(w http.ResponseWriter, r *http.Request) {
	langs, err := s.store.Languages(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "查询语言列表失败")
		return
	}
	s.writeJSON(w, http.StatusOK, envelope{Data: langs})
}

// GET /api/repos?window=day|week|month
func (s *Server) handleRepos(w http.ResponseWriter, r *http.Request) {
	win, err := s.parseWindow(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	latest, err := s.store.LatestSnapshotTime(r.Context(), win)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "查询快照失败")
		return
	}

	repos, err := s.store.ReposInWindow(r.Context(), win)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "查询仓库失败")
		return
	}

	s.writeJSON(w, http.StatusOK, envelope{
		Data: repos,
		Meta: &meta{
			Window:      string(win),
			SnapshotAt:  formatTime(latest),
			Total:       len(repos),
			RequestedAt: formatTime(s.now()),
		},
	})
}

// GET /api/radar?window=day|week|month
func (s *Server) handleRadar(w http.ResponseWriter, r *http.Request) {
	win, err := s.parseWindow(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	latest, err := s.store.LatestSnapshotTime(r.Context(), win)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "查询快照失败")
		return
	}

	scores, err := s.store.LanguageScores(r.Context(), win)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "计算雷达数据失败")
		return
	}

	s.writeJSON(w, http.StatusOK, envelope{
		Data: scores,
		Meta: &meta{
			Window:      string(win),
			SnapshotAt:  formatTime(latest),
			Total:       len(scores),
			RequestedAt: formatTime(s.now()),
		},
	})
}

// formatTime 将时间格式化为 ISO 8601 字符串,零值返回空串。
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
