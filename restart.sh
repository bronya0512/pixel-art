#!/bin/bash
# 一键更新并重启像素画服务
set -e

cd "$(dirname "$0")"

# 读取密钥：优先从 .env 文件（不提交 git），其次用当前 shell 环境变量
if [ -f .env ]; then
  set -a
  . ./.env
  set +a
fi

echo "==> [1/3] 拉取最新代码..."
git pull

echo "==> [2/3] 构建 Docker 镜像..."
docker build -t pixel-art .

echo "==> [3/3] 重启容器..."
docker rm -f pixel-art >/dev/null 2>&1 || true
docker run -d --name pixel-art --restart=always -p 8080:8080 \
  -e CHAT_API_KEY="${CHAT_API_KEY}" \
  pixel-art

echo ""
echo "==> 部署完成！"
IP=$(hostname -I 2>/dev/null | awk '{print $1}')
echo "访问地址: http://${IP}:8080"
