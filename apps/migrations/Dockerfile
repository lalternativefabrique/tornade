# Build migrate binary from source with up-to-date Go modules to avoid
# inheriting CVEs from the outdated upstream migrate/migrate:vX image.
FROM golang:1.26.6-alpine AS builder

ARG MIGRATE_VERSION=v4.19.1

RUN apk add --no-cache git ca-certificates

WORKDIR /src
RUN git clone --depth 1 --branch ${MIGRATE_VERSION} https://github.com/golang-migrate/migrate.git .

# Force-upgrade vulnerable transitive dependencies flagged by Trivy.
RUN go get \
        github.com/golang-jwt/jwt/v4@v4.5.2 \
        github.com/jackc/pgx/v5@v5.9.0 \
        golang.org/x/crypto@v0.46.0 \
        golang.org/x/oauth2@v0.34.0 \
        google.golang.org/grpc@v1.79.3 \
        github.com/go-jose/go-jose/v4@v4.1.4 \
    && go mod tidy

RUN CGO_ENABLED=0 GOOS=linux go build \
        -tags "postgres" \
        -ldflags="-s -w -X main.Version=${MIGRATE_VERSION}" \
        -o /out/migrate ./cmd/migrate

FROM alpine:3.21

RUN apk upgrade --no-cache && \
    apk add --no-cache ca-certificates && \
    addgroup -S migrate && adduser -S migrate -G migrate

COPY --from=builder /out/migrate /usr/local/bin/migrate
COPY . /migrations

USER migrate
ENTRYPOINT ["migrate", "-path", "/migrations", "-database"]
CMD ["--help"]
