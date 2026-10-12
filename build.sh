#!/usr/bin/env bash
# 构建 linux/amd64 c-shared 插件（在 linux/amd64 + gcc 环境执行）
set -euo pipefail
cd "$(dirname "$0")"
VERSION="${VERSION:-0.1.2}"
OUT="dist/linux/amd64/client-guard.so"
mkdir -p "$(dirname "$OUT")"
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -buildmode=c-shared \
  -ldflags "-s -w -X main.pluginVersion=${VERSION}" -o "$OUT" .
rm -f "${OUT%.so}.h"
echo "built $OUT"; sha256sum "$OUT"
echo "最高 GLIBC 依赖: $(objdump -T "$OUT" | grep -o 'GLIBC_[0-9.]*' | sort -uV | tail -1)"
