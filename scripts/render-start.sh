#!/bin/sh
# Render 免费 Web Service 启动脚本:
# 单容器同时跑 API + 抓取器(免费档无 Background Worker / 持久盘)。
set -eu

mkdir -p /data
export TRENDSCOPE_DB="${TRENDSCOPE_DB:-/data/trendscope.db}"

# Render 注入 PORT;本地未注入时默认 8080
if [ -n "${PORT:-}" ]; then
  export TRENDSCOPE_ADDR=":${PORT}"
fi

# 后台启动抓取器;休眠唤醒后会重新灌数据(免费档磁盘是临时的)
trendscope-ingest &
INGEST_PID=$!

cleanup() {
  kill "$INGEST_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

exec trendscope-api
