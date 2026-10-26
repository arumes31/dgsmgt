FROM golang:1.26.10-alpine3.24@sha256:9ce231b28998b32e116927c61aa5c406183061d4f3f2d61b292c88e7f6bdf7ca AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o soulmask-control ./cmd/server/main.go

FROM alpine:3.24@sha256:71e9b7bae160ba167fa17414401b94cbcdaf3c32feeab9298bd794a2a0614c35

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
