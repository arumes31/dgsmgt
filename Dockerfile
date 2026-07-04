FROM golang:1.25-alpine AS builder

ARG VERSION=dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-X dgsmgt/internal/config.BuildVersion=${VERSION}" -o dgsmgt ./cmd/server/main.go

FROM alpine:latest

WORKDIR /app
COPY --from=builder /app/dgsmgt .
COPY static/ static/

RUN mkdir -p /app/backups /app/serverlogs /app/serverdata

EXPOSE 8080

CMD ["./dgsmgt"]
