# ---------- build ----------
FROM golang:1.24-alpine AS builder

WORKDIR /src

# 依赖先拷贝，最大化利用构建缓存
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# modernc.org/sqlite 为纯 Go 实现，无需 CGO，产出静态二进制
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server .

# ---------- runtime ----------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 -s /sbin/nologin app

WORKDIR /app
COPY --from=builder /out/server /app/server

RUN mkdir -p /app/data && chown -R app:app /app

USER app

ENV PORT=8080 \
    DATA_DIR=/app/data \
    TZ=Asia/Shanghai

EXPOSE 8080
VOLUME ["/app/data"]

HEALTHCHECK --interval=30s --timeout=4s --start-period=8s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/api/public/health >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/server"]
