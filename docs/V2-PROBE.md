# V2 foundation / Telegram integration probe

This is not a production backup service. It deliberately does not touch the V1 SQLite database, Telethon session, Docker services, or cloud credentials. Run from the v2-rewrite branch with Go 1.27.4. Direct dependencies are pinned in go.mod; run go mod tidy and commit the resolved go.mod/go.sum after reviewing the result. CI compiles and tests without credentials.

## Start the foundation health API

```sh
go mod tidy
go test -race ./...
go run ./cmd/server
# in another terminal
curl http://127.0.0.1:8081/api/health
```

## Real Telegram verification (Linux)

Use a test account or authorized account, a fresh private directory and a private interactive terminal. Never use the V1 Telethon session. Do not commit secrets, QR images or downloaded files.

```sh
umask 077
export TG_API_ID=YOUR_API_ID
read -rsp 'Telegram API hash: ' TG_API_HASH; echo
export TG_API_HASH
mkdir -p "$HOME/.telegram-backup-v2-probe"
chmod 700 "$HOME/.telegram-backup-v2-probe"
go run ./cmd/telegram-probe -state-dir "$HOME/.telegram-backup-v2-probe"
```

Open the private QR PNG from the displayed path (on a headless host retrieve it through your private SSH/SFTP connection), scan it from Telegram Settings / Devices / Link Desktop Device, and enter the 2FA password in the terminal if prompted. The program lists up to 25 recent Saved Messages IDs without printing message text. Send a small test image or video to Saved Messages first. Run again with the same state directory to verify session reuse.

```sh
go run ./cmd/telegram-probe -state-dir "$HOME/.telegram-backup-v2-probe" -message-id 123
unset TG_API_HASH TG_API_ID
```

Replace 123 with the selected media message ID. The download is capped at 32 MiB, saved via a temporary file, then published without overwriting an existing file. Verify the file locally. Remove test media manually once verified. QR images and sessions are sensitive; if the process is killed, inspect the private directory and remove stale QR files and probe.lock only after confirming no probe process is active.

Acceptance: compile/test pass, QR+optional 2FA works, session survives restart, history listing works, explicit media download completes and matches source, over-limit download leaves no completed file. Real Telegram and Drive tests must be run by an authorized operator; CI does not perform them. Production work still needs peer resolution, history checkpoints, robust resume, PostgreSQL/River, full storage integration and UI.
