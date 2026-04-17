FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o soulmask-control ./cmd/server/main.go

FROM alpine:latest

WORKDIR /app
COPY --from=builder /app/soulmask-control .
COPY static/ static/

RUN mkdir -p /app/data && chmod 777 /app/data

EXPOSE 8080

CMD ["./soulmask-control"]
