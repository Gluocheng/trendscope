# TrendScope

展示 GitHub 技术趋势的 Web 应用。定时从 GitHub API 抓取热门仓库与语言数据,存储到 SQLite,通过 Web 界面以雷达图与榜单形式呈现技术热点,支持日 / 周 / 月时间窗口切换。

## 功能特性

- **语言趋势雷达图**: 按各语言仓库星标总量加权,直观对比语言活跃度
- **热门仓库榜**: 展示窗口内最热门的仓库(星标、语言标签、跳转链接)
- **日 / 周 / 月切换**: 三个时间窗口独立抓取与查询
- **并发抓取**: goroutine 并发池,自适应 GitHub 限流(`X-RateLimit-*`),指数退避重试
- **快照式存储**: 每次抓取生成独立快照,便于计算趋势
- **单二进制部署**: 前端静态文件通过 `go:embed` 内嵌进 API 服务

## 架构

```
GitHub API ──> Ingestor 抓取服务(并发池 + 限流 + 重试)──> SQLite
                                                          │
                                                          ▼
前端 SPA (ECharts 雷达图 + 榜单) <── REST JSON <── Go API 服务
```

三个独立模块:

| 模块 | 说明 |
|---|---|
| `cmd/ingest` | 定时抓取器,按语言并发搜索热门仓库写入 SQLite |
| `cmd/api` | REST API 服务,读取 SQLite 返回 JSON,托管前端静态文件 |
| `frontend/` | 原生 JS + ECharts 单页应用 |

## 快速开始

### 本地运行(需要 Go 1.25+)

```bash
# 1. 启动抓取器(首次抓取约几分钟,受 GitHub 限流影响)
cd backend
go run ./cmd/ingest

# 2. 另开终端启动 API 服务
go run ./cmd/api

# 3. 打开 http://localhost:8080
```

可选:设置 `GITHUB_TOKEN` 环境变量可将 GitHub API 配额从 60 次/小时提升到 5000 次/小时。

```bash
export GITHUB_TOKEN=ghp_xxx   # Windows: $env:GITHUB_TOKEN="ghp_xxx"
```

### Docker Compose 一键启动

```bash
docker compose up -d --build
```

会自动启动 API 服务(`:8080`)与定时抓取服务(默认每小时),数据持久化在 `trendscope-data` 卷。

## 配置环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `TRENDSCOPE_ADDR` | `:8080` | API 监听地址 |
| `TRENDSCOPE_DB` | `trendscope.db` | SQLite 数据库路径 |
| `GITHUB_TOKEN` | 空 | GitHub API 令牌,建议设置以提升配额 |
| `TRENDSCOPE_INTERVAL` | `1h` | 抓取间隔 |
| `TRENDSCOPE_WORKERS` | `5` | 并发 worker 数 |
| `TRENDSCOPE_PER_PAGE` | `50` | 每语言每页抓取数 |
| `TRENDSCOPE_MAX_PAGES` | `1` | 每语言抓取页数 |

## API 文档

统一响应格式:

```json
{
  "data": ...,
  "meta": { "window": "day", "snapshot_at": "...", "total": 100, "requested_at": "..." },
  "error": null
}
```

### `GET /healthz`

健康检查,返回 `{"data": {"status": "ok"}}`。

### `GET /api/repos?window=day|week|month`

返回指定窗口的热门仓库榜单,按星标降序。

```json
{
  "data": [
    {
      "id": 1331450399,
      "full_name": "owner/repo",
      "owner": "owner",
      "name": "repo",
      "stars": 100,
      "language": "Go",
      "description": "...",
      "html_url": "https://github.com/owner/repo",
      "created_at": "2026-08-12T00:00:22Z",
      "snapshot_time": "2026-08-12T02:00:00Z"
    }
  ]
}
```

### `GET /api/radar?window=day|week|month`

返回语言活跃度分数,按分数降序。

```json
{
  "data": [
    { "language": "TypeScript", "score": 85817, "count": 50, "avg_stars": 1716.3 }
  ]
}
```

### `GET /api/languages`

返回数据中出现过的所有语言列表。

## 开发

```bash
# 单元测试
cd backend && go test ./...

# 修改前端后同步到 embed 目录(CI 会检查一致性)
Copy-Item frontend/* backend/cmd/api/public/ -Recurse -Force
```

## 项目结构

```
trendscope/
├── backend/
│   ├── cmd/
│   │   ├── api/main.go          # API 服务入口(embed 托管前端)
│   │   └── ingest/main.go       # 抓取器入口
│   ├── internal/
│   │   ├── api/                 # HTTP handlers
│   │   ├── ingestor/            # GitHub 抓取、限流、重试
│   │   ├── model/               # 数据模型
│   │   └── store/               # SQLite 存储层
│   └── ...
├── frontend/                    # 前端源码(独立可维护)
│   ├── index.html
│   ├── css/style.css
│   └── js/app.js
├── Dockerfile
├── docker-compose.yml
└── .github/workflows/ci.yml
```

## 里程碑

- [x] M1 骨架: Go 工程 + SQLite 存储层 + 基本 API
- [x] M2 抓取: GitHub 抓取器 + 并发池 + 定时任务 + 快照存储
- [x] M3 前端: 雷达图 + 榜单 + 时间切换
- [x] M4 工程化: Docker + CI + 测试 + 本文档

## 范围外

用户系统 / 登录鉴权、多数据源聚合、分布式部署、数据库迁移系统。
