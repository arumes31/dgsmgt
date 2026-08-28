FROM golang:1.27.0-alpine3.24@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o dgsmgt ./cmd/server/main.go && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o dgsmgt-docker-proxy ./cmd/docker-proxy/main.go

FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

WORKDIR /app
COPY --from=builder /app/dgsmgt /app/dgsmgt-docker-proxy ./
COPY static/ static/

RUN addgroup -S -g 65532 dgsmgt && \
    adduser -S -D -H -u 65532 -G dgsmgt dgsmgt && \
    mkdir -p /app/data /run/dgsmgt && \
    chown -R 65532:65532 /app/data /run/dgsmgt

EXPOSE 8080

USER 65532:65532

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/health || exit 1

CMD ["./dgsmgt"]
