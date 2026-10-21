FROM golang:1.27.2-alpine3.24@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS builder

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
