# TelegramBackup V2 — Architecture and implementation contract

Status: accepted direction; implementation and end-to-end verification pending.

## 1. Scope and stack

Build a self-hosted, single-process modular Go application. Go 1.27.4, net/http + chi v5, gotd/td, PostgreSQL 18, pgx/v5, sqlc, Goose, open-source River (no Pro dependency), slog, OpenAPI, SSE. React 19, TypeScript, Vite, shadcn/ui, Tailwind CSS 4, TanStack Router/Query/Table and Lucide. Pin tested dependency versions and lockfiles. Use rclone CLI behind a Storage interface; no Redis, Kafka or Node runtime in production. Embed built SPA with Go embed. Docker Compose application + PostgreSQL, persistent private volumes. V2 is a clean install; no legacy migration or compatibility layer.

## 2. Components and ownership

- cmd/server: composition root, configuration, health/readiness, lifecycle and graceful shutdown.
- internal/telegram: one session owner per account, QR/phone/2FA auth, peer/access-hash persistence, historical pagination, update recovery, download by stable message identity. Treat session and credentials as secrets; never log them.
- internal/backup: policies, durable scan checkpoints, discovery and transactional idempotent job insertion.
- internal/worker: River queues sync, download, upload, verify, cleanup, maintenance. Separate queue limits; no workflow Pro dependency. Each stage commits business state and enqueues successor in one pgx transaction. External side effects are reconciled, not assumed exactly once.
- internal/disk: atomic capacity reservation, real free-space monitoring, staging files, startup reconciliation.
- internal/storage: rclone process adapter with context cancellation, bounded output, structured progress, upload and verification. No shell interpolation. Keep OAuth updates serialized per remote and protect temporary configuration.
- internal/database: SQL queries, sqlc-generated code; versioned schema migrations. River's own migration mechanism separately tracked.
- internal/api, auth, events: REST and SSE, authenticated dashboard, safe event retention and reconnect snapshots.
- web: feature modules, accessible light/dark/system themes, responsive dashboard.

## 3. Data and consistency

Core tables: telegram_accounts, telegram_sessions, telegram_chats, sync_checkpoints, messages, media_files, storage_remotes, media_replicas, backup_policies, sync_runs, disk_reservations, task_events, system_settings; River owns queue tables. Use UTC timestamps, bigint Telegram IDs, canonical peer kind and ID, explicit FK and indexes. Unique message key (account_id, peer_kind, peer_id, message_id); media unique by message and attachment identity; replica unique by media and storage target. Separate download state from per-replica upload state. Persist destination and deterministic remote key per replica; do not silently reroute an in-flight job if default storage changes.

History scans use bounded pages and durable checkpoints advanced only after successful transactional ingestion. Realtime events and scan overlap by design. Store update recovery state and peer access hashes; do not advance history checkpoint based only on latest live message. Periodic reconciliation repairs missed updates. Respect Telegram FloodWait, bounded retries, account-level backoff. Only back up chats the account is authorized to access; no access-control bypass.

## 4. Transfer lifecycle

DISCOVERED -> DOWNLOADING -> DOWNLOADED -> UPLOADING -> VERIFYING -> CLEANUP_PENDING -> COMPLETED. Transient errors enter retryable states with attempt/backoff metadata; permanent or resource-blocked errors are visible. Cancellation/pause is cooperative. Use River job identifiers for scheduling, business state for idempotency. On restart reconcile leases, files, jobs and remote effects. Never treat a .part file as complete. Check size and optionally hash before atomically renaming a staged file. Resume only with valid persisted offsets, file identity and supported gotd semantics; otherwise restart safely. Refresh expired Telegram file references from message identity.

Upload using deterministic object key, reconcile existing remote object and verify length and supported content hash, record verified replica/remote ID, then delete local file only after all required replicas are verified. Keep local path until cleanup is recorded; deletion must be idempotent. If upload succeeded before DB commit, reconcile the remote on retry. Google Drive permits duplicate names; detect conflicts and avoid treating path alone as an exactly-once guarantee. Persist remote IDs when available. Handle destination configuration changes explicitly. Encrypted OAuth config and refresh changes must be preserved; never overwrite concurrent updates.

## 5. Disk protection

Defaults (configurable): 2 download slots, 2 upload slots, 10 GiB staging quota, 5 GiB filesystem reserve. Not measured production tuning. Disk budget = min(quota minus staged bytes and outstanding reservations, free filesystem bytes minus reserve and outstanding unwritten reservations), clamped to zero. Reservations are atomic across active jobs; count partial files and failed upload backlog. Use a dedicated staging directory/volume; apply reserve to its actual filesystem. Unknown-length or oversized files must be explicitly blocked or handled by a separately tested bounded streaming strategy; do not spin on retries. Stop admitting downloads when full; keep upload and cleanup running. Reconcile orphan files without deleting unverified or active media. Throttle progress updates and enforce cancellation.

## 6. API and UI

Routes: overview, accounts, chats, tasks, transfers, archive, remotes, policies, logs, settings. UI style: restrained Vercel/Linear-inspired layout, collapsible sidebar, neutral zinc palette, blue accent, 8–12 px radius, clear dense tables, light/dark/system themes, responsive mobile layout, skeletons/toasts. Show real data and empty/error states, no fake metrics. SSE authenticated via secure same-origin session cookies, event IDs/reconnect plus snapshot fallback; never place credentials in URL. Admin authentication, CSRF protection for cookie-auth mutations, HTTPS via reverse proxy, least privilege, non-root runtime, no default credentials, redact logs. API and background workers share application services, not Telegram session files across processes.

## 7. Fresh deployment only

V2 starts with a new PostgreSQL database, a fresh Telegram authorization, independent rclone configuration and a private staging directory. There is no SQLite importer, Telethon session conversion, or compatibility requirement. Run one application instance. Generate independent database, admin and encryption secrets. Verify the entire backup workflow on a dedicated authorized chat before enabling large archives.

## 8. Delivery gates

Phase 0: branch and this contract.
Phase 1: compile and test Go scaffold, gotd login/history/download spike (real test account controlled by operator), dependencies locked.
Phase 2: PostgreSQL migrations, sqlc, River jobs and durable checkpoints; transaction rollback and duplicate enqueue tests.
Phase 3: full backup pipeline, bounded concurrency, restart and disk safety, rate-limit and remote verification tests.
Phase 4: dashboard rewrite, OpenAPI + generated API types, SSE, mobile and dark theme tests.
Phase 5: fresh-install documentation, reconciliation, and end-to-end tests with authorized Telegram account and Drive. CI runs formatting, lint, go test -race, frontend typecheck/build, PostgreSQL integration and container build. Test crash at every stage, disk exhaustion, network failure, failed deletion, auth expiration, expired file reference, concurrent discovery and duplicate cloud names.
Phase 6: release candidate and explicit cutover approval; no release before verification.

## References

- Go downloads: https://go.dev/dl/
- gotd: https://gotd.dev/docs/intro/
- gotd download helpers: https://gotd.dev/docs/helpers/downloading-files/
- gotd update recovery: https://gotd.dev/docs/helpers/updates-recovery/
- River transactional enqueue: https://riverqueue.com/docs/transactional-enqueueing
- River queues: https://riverqueue.com/docs/multiple-queues
- shadcn Vite: https://ui.shadcn.com/docs/installation/vite
- rclone: https://rclone.org/docs/
