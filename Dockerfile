# TrendScope 多阶段构建:后端编译 + 精简运行镜像
# 使用 golang:1.25-alpine 匹配 go.mod 声明的 Go 版本

# ---- 构建阶段 ----
FROM golang:1.25-alpine AS builder
WORKDIR /app

# 先复制依赖文件,利用 Docker 层缓存
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# 复制源码并编译两个二进制
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/trendscope-api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/trendscope-ingest ./cmd/ingest

# ---- 运行阶段 ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=builder /out/trendscope-api /usr/local/bin/trendscope-api
COPY --from=builder /out/trendscope-ingest /usr/local/bin/trendscope-ingest
COPY scripts/render-start.sh /usr/local/bin/render-start.sh
RUN chmod +x /usr/local/bin/render-start.sh && \
    # 防止 Windows 检出时 CRLF 导致 sh 无法执行
    sed -i 's/\r$//' /usr/local/bin/render-start.sh

# 数据目录(Render 免费档为临时盘,休眠后会清空)
RUN mkdir -p /data
ENV TRENDSCOPE_DB=/data/trendscope.db

EXPOSE 8080
CMD ["render-start.sh"]
