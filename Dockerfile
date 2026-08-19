# 图片转像素画 - 多阶段构建
# 阶段1：编译 Go 后端
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY main.go ./
COPY static/ static/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o pixel-art-server .

# 阶段2：运行环境（Python + Pillow）
FROM python:3.11-slim
WORKDIR /app

# 复制 Go 二进制和 Python 脚本
COPY --from=builder /app/pixel-art-server .
COPY pixelate.py .
COPY requirements.txt .

# 安装 Pillow
RUN pip install --no-cache-dir -r requirements.txt

# 创建非 root 用户运行
RUN useradd -m appuser && chown -R appuser:appuser /app
USER appuser

EXPOSE 8080
CMD ["./pixel-art-server"]
