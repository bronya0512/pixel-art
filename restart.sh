#!/bin/bash
# 一键更新并重启像素画服务
set -e

cd "$(dirname "$0")"

echo "==> [1/3] 拉取最新代码..."
git pull

echo "==> [2/3] 构建 Docker 镜像..."
docker build -t pixel-art .

echo "==> [3/3] 重启容器..."
docker rm -f pixel-art >/dev/null 2>&1 || true
docker run -d --name pixel-art --restart=always -p 8080:8080 pixel-art

echo ""
echo "==> 部署完成！"
IP=$(hostname -I 2>/dev/null | awk '{print $1}')
echo "访问地址: http://${IP}:8080"
