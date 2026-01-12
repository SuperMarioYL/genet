FROM golang:1.21-alpine AS builder

WORKDIR /build

# 安装依赖
RUN apk add --no-cache git

# 复制 go mod 文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建 API 服务器
RUN CGO_ENABLED=0 GOOS=linux go build -o api-server ./cmd/api

# 构建控制器
RUN CGO_ENABLED=0 GOOS=linux go build -o controller ./cmd/controller

# 运行时镜像
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# 复制二进制文件
COPY --from=builder /build/api-server /app/
COPY --from=builder /build/controller /app/

EXPOSE 8080

# 默认运行 API 服务器
CMD ["/app/api-server"]
