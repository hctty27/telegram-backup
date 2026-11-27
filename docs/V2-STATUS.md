# V2 development preview — NOT production ready

## What is implemented

- Go foundation with health API and operator-run gotd QR/2FA/history/download probe.
- PostgreSQL 18 schema, Goose and River migration command, sqlc config and query definitions (generated code not committed yet).
- Typed River queue registration and transactional enqueue adapter, not activated in server.
- Conservative single-process disk admission, Linux filesystem measurement, bounded staging, rclone immutable upload and verification, exact-path cleanup primitives and unit tests. These are not yet connected to the production sync pipeline.
- React 19 + Vite + Tailwind 4 + shadcn-compatible component structure + TanStack Query dashboard preview, authenticated read-only real database data, light/dark themes. Polling; no SSE yet. No fake data.
- Go embedded web assets, isolated V2 Compose configuration with PostgreSQL 18 and non-root application. No V1 service changes.

## Preview deployment

Use a separate checkout of v2-rewrite. After GitHub verification passes, generate a private V2 environment file with independent values:

```sh
umask 077
printf 'V2_DB_PASSWORD=%s\nV2_ADMIN_TOKEN=%s\n' "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" > .env.v2
docker compose --env-file .env.v2 -f compose.v2.yaml up -d --build
docker compose --env-file .env.v2 -f compose.v2.yaml ps
```

The preview binds host loopback 8081 and does not use the V1 data volume or 8080 port. Access through a private SSH tunnel or trusted HTTPS reverse proxy. Use V2_ADMIN_TOKEN from your local private file. Never publish the token. The database and UI are empty until data is imported/discovered; the preview will not run backup workers or delete files.

## Outstanding release blockers

1. Confirm Go, gotd, River, pgx and all npm dependency versions resolve; review CI output and commit go.sum, npm lockfile and sqlc generated package. CI is configured but not yet verified.
2. Production Telegram session owner, encrypted session storage, browser-based QR/2FA, peer mapping, history and incremental checkpointing, realtime updates with gap recovery, Telegram FloodWait and fresh file references.
3. Wire real River handlers with transactional state transitions, job reconciliation, safe retries, quota reservations and verified multi-replica cleanup. Handle duplicate Drive names, remote ID reconciliation, cancellation and process crashes. The storage primitive does not itself delete files.
4. Full settings/authentication and storage OAuth configuration; encrypted secrets and persistent credential refresh management; SSE and live progress; archive previews, pagination/filtering, task controls, automated history sync.
5. Build and verify idempotent V1 dry-run importer; preserve source chat/message/remote identities and checkpoints, reconcile files, reauthorize or convert Telethon session offline, never expose credentials. No production migration has run.
6. Run real authorized Telegram and Drive end-to-end tests, concurrency/restart/failure injection and browser tests; only then consider a release candidate and explicit production cutover.

No code in the preview enables automatic download/upload/delete; do not merge or replace V1 based only on source commits.
