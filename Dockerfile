# ===========================================
# Stage 1: 构建阶段
# ===========================================
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /app/bin/server \
    ./cmd/server/

# ===========================================
# Stage 2: 运行阶段
# ===========================================
FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata wget

ENV TZ=UTC

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=builder /app/bin/server /app/server

# 创建数据目录用于 SQLite
RUN mkdir -p /data && chown app:app /data

USER app

EXPOSE 5004

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:5004/healthz || exit 1

ENTRYPOINT ["/app/server"]
