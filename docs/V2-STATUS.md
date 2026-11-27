# TelegramBackup V2 — fresh-install development build

V2 is a clean implementation. No legacy database, Telethon session, or API migration is required or planned.

## Source implementation

- PostgreSQL schema and migrations, River queues and transactionally enqueued media stages.
- Fresh Telegram account creation; AES-256-GCM encrypted API hash and session records; browser QR + 2FA authorization, dialog discovery, bounded history pagination and incremental polling with durable checkpoints, plus resident per-account clients and direct new-message/media ingestion.
- Telegram account-level rate limit backoff; per-account exclusive resident client owner, reused for serialized account requests. QR authorization temporarily suspends the resident client. The supervisor reconciles enabled accounts every 15 seconds.
- Download admission with configurable staging quota / minimum free space (defaults 10 / 5 GiB), atomic staged file publication, fresh message/file reference retrieval, immutable rclone upload, size/hash/readback verification and guarded local cleanup.
- Dashboard with real database data, account/QR/2FA, chat import, registered rclone remote, backup policy, sync-now and failed job retry actions. Overview transfer and disk monitor uses authenticated SSE snapshots every two seconds; durable tables still refresh by polling. Download bytes are measured from successful writes; upload bytes are reported by rclone JSON stats when available. No simulated metrics.
- Compose application and PostgreSQL, temporary migration container; one app instance. CI includes frontend build, Go compile/vet/race tests, migrations, API smoke and PostgreSQL pipeline test with stub external services.

## Fresh installation (only after build and tests are green)

```sh
umask 077
printf 'V2_DB_PASSWORD=%s\nV2_ADMIN_TOKEN=%s\nV2_ENCRYPTION_KEY=%s\nV2_ENABLE_WORKERS=true\n' "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" > .env.v2
docker compose --env-file .env.v2 -f compose.v2.yaml up -d --build
docker compose --env-file .env.v2 -f compose.v2.yaml ps
```

Access host loopback 8081 through a trusted HTTPS reverse proxy or private SSH tunnel. Use the private V2_ADMIN_TOKEN to sign in. Keep the encryption key backed up securely. Set V2_CACHE_GIB and V2_MIN_FREE_GIB in .env.v2 to override defaults; restart the app to apply. Lowering the quota does not delete staged files. Existing blocked jobs are not automatically reactivated by a quota increase. SSE uses bearer-authenticated fetch; configure the reverse proxy to avoid buffering text/event-stream and allow long-lived responses.

1. Create a personal Google OAuth Desktop client following https://rclone.org/drive/#making-your-own-client-id. On a trusted computer with a browser, run rclone authorize drive with that client ID and secret, and copy the returned token JSON (including refresh_token). In the V2 cloud storage page, enter a unique remote name, client ID, client secret, token JSON, and optional root folder ID. The server tests listing access before creating the remote. This does not yet prove write permission; the real upload smoke test is still required. The app requests Drive scope for backing up files to a selected Drive folder. Do not disclose these secrets or paste them into chat.
   - Each remote is encrypted with AES-256-GCM in PostgreSQL. During operations a private mode-0600 config on /dev/shm (tmpfs) is used; the refreshed config is re-encrypted and committed even on an operation failure. Different remotes can upload independently, but each remote serializes operations. Do not configure existing remote names with a different account; choose a new unique name.
   - This replaces the preliminary operator-provisioned plaintext rclone config path. Fresh installs need no manual container configuration. Existing development-only unmanaged remotes are unsupported by the new adapter; configure a new remote and policy in a fresh test database. No legacy compatibility is promised.
   - OAuth authorization still occurs on the trusted computer; this is a token import form, not an integrated browser callback.
2. Create a Telegram account using your own API ID and API hash, scan the QR and complete optional 2FA.
3. Refresh chats. Select an authorized dedicated test chat and the registered storage remote, and save an enabled backup policy. The initial pass scans history; subsequent passes use incremental checkpoints.
4. Verify actual cloud content, task completion, local staging cleanup, and recovery after process restart before expanding the policy.

## Remaining verification and limitations

- Revision c3215fb passed the complete V2 GitHub verification workflow (46020585947), including mock pipeline tests and Docker build; the subsequent resume revision d9dcca3 compiled but failed its unit tests because the test directories were not mode 0700; the test setup was corrected without weakening the production guard. The current resident-client revision still needs confirmation; dependency lockfiles and generated sqlc code still need review. Do not treat source commits as a released binary.
- Real Telegram/Google Drive end-to-end authorization and media downloads, rate-limit recovery, cloud duplicate-name behavior, crash recovery, disk pressure and browser tests require execution.
- Live message and channel-message events are now dispatched into the same transactional ingestion path as polling. Live events never advance the history cursor; periodic scans compensate for missed events, including after reconnect. The gotd update-gap manager and durable pts/qts/seq state are not yet integrated, so messages deleted before a compensating scan cannot be guaranteed to be recovered. Each account serializes network operations; rclone configuration operations serialize per remote. Cross-account download concurrency is bounded by queue and disk quota. Downloads now retain aligned SHA-256 checkpoints synced to disk every 4 MiB when the writer reaches an aligned offset, and on aligned failures. A resumed request validates peer/message/media identity, expected length and prefix digest; gotd verifies transferred chunks and resumes with WithOffset. Invalid checkpoints restart from zero. Under quota pressure inactive unverified partials may be evicted to prevent queue deadlock; active or published files are not evicted. Real Telegram interruption/restart and file-hash compatibility still need verification.
- Google Drive credential import and encrypted configuration are implemented; direct OAuth callback, write-permission validation, credential rotation, other storage providers, per-chat filters, web-editable disk limits, expanded pagination and additional monitoring remain outstanding.
- Unknown-size or oversized media are recorded as blocked rather than exceeding disk quota. Only the selected media exposed by gotd Elem.File is handled; unsupported media formats and edits need more coverage.
- No legacy migration is a release blocker. No release has been published.
