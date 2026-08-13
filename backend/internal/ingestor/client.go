// Package ingestor 从 GitHub API 抓取热门仓库并写入 SQLite。
package ingestor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"trendscope/internal/model"
)

// 抓取的默认语言列表。可抓取多个语言,以便雷达图有足够维度。
var defaultLanguages = []string{
	"Go", "Python", "TypeScript", "JavaScript", "Rust", "Java",
	"Kotlin", "Swift", "C++", "C#", "Ruby", "PHP",
}
// githubRepo 对应 GitHub Search API 的仓库对象字段(仅取所需)。
type githubRepo struct {
	ID          int64  `json:"id"`
	FullName    string `json:"full_name"`
	Owner       struct {
		Login string `json:"login"`
	} `json:"owner"`
	Name        string `json:"name"`
	Stars       int    `json:"stargazers_count"`
	Language    string `json:"language"`
	Description string `json:"description"`
	HTMLURL     string `json:"html_url"`
	CreatedAt   string `json:"created_at"`
}

// searchResponse 对应 GitHub Search API 响应。
type searchResponse struct {
	TotalCount int          `json:"total_count"`
	Items      []githubRepo `json:"items"`
}

// Client 封装 GitHub Search API 调用,包含限流与重试。
type Client struct {
	http      *http.Client
	token     string
	userAgent string
	baseURL   string
}

// NewClient 创建 GitHub API 客户端。
func NewClient(token string) *Client {
	return &Client{
		http:      &http.Client{Timeout: 30 * time.Second},
		token:     token,
		userAgent: "TrendScope/0.1",
		baseURL:   "https://api.github.com",
	}
}

// SearchRepos 搜索给定窗口内有更新、且达到最低星标的热门仓库,按星标降序返回。
// perPage 为每页数量,最多拉取 maxPages 页。
func (c *Client) SearchRepos(ctx context.Context, w model.Window, language string, perPage, maxPages int) ([]model.Repo, error) {
	if perPage <= 0 {
		perPage = 50
	}
	if maxPages <= 0 {
		maxPages = 1
	}

	pushedSince := time.Now().UTC().Add(-w.Durations())
	query := fmt.Sprintf("pushed:>%s stars:>50 language:%s", pushedSince.Format("2006-01-02"), language)

	var repos []model.Repo
	for page := 1; page <= maxPages; page++ {
		pageRepos, err := c.fetchPage(ctx, query, page, perPage)
		if err != nil {
			return nil, fmt.Errorf("search %s page %d: %w", language, page, err)
		}
		repos = append(repos, pageRepos...)
		if len(pageRepos) < perPage {
			break
		}
	}
	return repos, nil
}

// fetchPage 拉取单页搜索结果,带限流等待与指数退避重试。
func (c *Client) fetchPage(ctx context.Context, query string, page, perPage int) ([]model.Repo, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	reqURL := &url.URL{
		Scheme: base.Scheme,
		Host:   base.Host,
		Path:   "/search/repositories",
		RawQuery: url.Values{
			"q":       {query},
			"sort":    {"stars"},
			"order":   {"desc"},
			"page":    {strconv.Itoa(page)},
			"per_page": {strconv.Itoa(perPage)},
		}.Encode(),
	}

	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<attempt) * 500 * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", c.userAgent)
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		// 自适应限流:读取剩余配额,不足时等待 reset。
		if remaining, resetAt, ok := rateLimitInfo(resp); ok && remaining <= 2 {
			wait := time.Until(resetAt)
			if wait > 0 {
				log.Printf("rate limit low (remaining=%d), waiting %s", remaining, wait.Round(time.Second))
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					resp.Body.Close()
					return nil, ctx.Err()
				}
			}
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			var sr searchResponse
			if err := json.Unmarshal(body, &sr); err != nil {
				return nil, fmt.Errorf("decode response: %w", err)
			}
			out := make([]model.Repo, 0, len(sr.Items))
			for _, gr := range sr.Items {
				out = append(out, toRepo(gr))
			}
			return out, nil

		case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
			// 限流:等待 reset 后重试
			if resetAt, ok := rateLimitReset(resp); ok {
				wait := time.Until(resetAt)
				if wait > 0 {
					log.Printf("rate limited, waiting %s", wait.Round(time.Second))
					select {
					case <-time.After(wait):
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
			}
			lastErr = fmt.Errorf("rate limited (status %d)", resp.StatusCode)
			continue

		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("server error (status %d): %s", resp.StatusCode, truncate(body, 300))
			continue

		default:
			return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, truncate(body, 300))
		}
	}
	return nil, fmt.Errorf("giving up after retries: %w", lastErr)
}

// decodeSearch 从响应中解析搜索结果的仓库列表。
func decodeSearch(resp *http.Response) ([]model.Repo, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, truncate(body, 300))
	}
	var sr searchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	out := make([]model.Repo, 0, len(sr.Items))
	for _, gr := range sr.Items {
		out = append(out, toRepo(gr))
	}
	return out, nil
}

// rateLimitInfo 从响应头解析剩余配额与重置时间。
func rateLimitInfo(resp *http.Response) (remaining int, resetAt time.Time, ok bool) {
	rem, err := strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining"))
	if err != nil {
		return 0, time.Time{}, false
	}
	reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64)
	if err != nil {
		return rem, time.Time{}, false
	}
	return rem, time.Unix(reset, 0), true
}

// rateLimitReset 仅解析重置时间。
func rateLimitReset(resp *http.Response) (time.Time, bool) {
	reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(reset, 0), true
}

// toRepo 将 GitHub API 仓库对象转换为内部模型。
func toRepo(g githubRepo) model.Repo {
	createdAt, _ := time.Parse(time.RFC3339, g.CreatedAt)
	return model.Repo{
		ID:          g.ID,
		FullName:    g.FullName,
		Owner:       g.Owner.Login,
		Name:        g.Name,
		Stars:       g.Stars,
		Language:    g.Language,
		Description: g.Description,
		HTMLURL:     g.HTMLURL,
		CreatedAt:   createdAt,
	}
}

// truncate 截断过长的错误信息。
func truncate(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
