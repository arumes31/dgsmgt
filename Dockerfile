FROM golang:1.27.4-alpine3.24@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o dgsmgt ./cmd/server/main.go

FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

# su-exec lets the entrypoint drop root after fixing volume ownership and Docker socket access
RUN apk add --no-cache su-exec \
    && addgroup -S dgsmgt \
    && adduser -S -D -H -G dgsmgt dgsmgt

WORKDIR /app
COPY --from=builder /app/dgsmgt .
COPY static/ static/
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod 0755 /usr/local/bin/docker-entrypoint.sh \
    && mkdir -p /app/data \
    && chown dgsmgt:dgsmgt /app/data \
    && chmod 0750 /app/data

EXPOSE 8080

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["./dgsmgt"]
