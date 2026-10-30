FROM golang:1.27.3-alpine3.24@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o soulmask-control ./cmd/server/main.go

FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

# su-exec lets the entrypoint drop root after fixing volume ownership and Docker socket access
RUN apk add --no-cache su-exec \
    && addgroup -S dgsmgt \
    && adduser -S -D -H -G dgsmgt dgsmgt

WORKDIR /app
COPY --from=builder /app/soulmask-control .
COPY static/ static/
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod 0755 /usr/local/bin/docker-entrypoint.sh \
    && mkdir -p /app/data \
    && chown dgsmgt:dgsmgt /app/data \
    && chmod 0750 /app/data

EXPOSE 8080

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["./soulmask-control"]
