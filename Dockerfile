# K8s-Inspector Dockerfile

FROM golang:1.25-alpine AS builder

WORKDIR /app

# 复制项目文件
COPY . .

# 构建项目
RUN go mod tidy && go build -o k8s-inspector .

# 最终镜像
FROM alpine:3.18

WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /app/k8s-inspector .

# 暴露端口
EXPOSE 8080

# 运行应用
CMD ["/app/k8s-inspector"]
