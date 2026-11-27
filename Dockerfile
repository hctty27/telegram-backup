# syntax=docker/dockerfile:1
# TelegramBackup Go application, with embedded frontend.
FROM node:22-bookworm-slim AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm,sharing=locked npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.27.4-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/assets.go ./web/assets.go
COPY --from=frontend /src/web/dist ./web/dist
ARG TGB_VERSION=local
ARG TGB_COMMIT=unknown
ARG TGB_BUILD_TIME=
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    go mod tidy -diff && go mod verify \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-X github.com/hctty27/telegram-backup/internal/buildinfo.Version=$TGB_VERSION -X github.com/hctty27/telegram-backup/internal/buildinfo.Commit=$TGB_COMMIT -X github.com/hctty27/telegram-backup/internal/buildinfo.BuildTime=$TGB_BUILD_TIME" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -o /out/migrate ./cmd/migrate \
 && CGO_ENABLED=0 go build -trimpath -o /out/telegram-probe ./cmd/telegram-probe

FROM rclone/rclone:1.76.0 AS rclone

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd --gid 10001 app && useradd --uid 10001 --gid 10001 --no-create-home app \
 && install -d -m 0700 -o app -g app /var/lib/telegram-backup
COPY --from=rclone /usr/local/bin/rclone /usr/local/bin/rclone
RUN rclone version | grep -Fx 'rclone v1.76.0'
WORKDIR /app
COPY --from=builder /out/ /usr/local/bin/
COPY migrations/ /app/migrations/
USER 10001:10001
ENV TGB_HTTP_ADDR=0.0.0.0:8080 TGB_MIGRATIONS_DIR=/app/migrations
EXPOSE 8080
CMD ["server"]
