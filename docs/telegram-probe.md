# Telegram integration probe

This standalone tool verifies an authorized Telegram account with a dedicated private directory. It does not use the application's database, sessions or storage configuration. Run from main with Go 1.27.4 and committed dependency locks. CI compiles and tests without credentials.

## Start the foundation health API

```sh
go mod tidy -diff
go test -race ./...
go run ./cmd/server
# in another terminal
curl http://127.0.0.1:8081/api/health
```

## Real Telegram verification (Linux)

Use a test account or authorized account, a fresh private directory and a private interactive terminal. Never share the application's session directory. Do not commit secrets, QR images or downloaded files.

```sh
umask 077
export TG_API_ID=YOUR_API_ID
read -rsp 'Telegram API hash: ' TG_API_HASH; echo
export TG_API_HASH
mkdir -p "$HOME/.telegram-backup-probe"
chmod 700 "$HOME/.telegram-backup-probe"
go run ./cmd/telegram-probe -state-dir "$HOME/.telegram-backup-probe"
```

Open the private QR PNG from the displayed path (on a headless host retrieve it through your private SSH/SFTP connection), scan it from Telegram Settings / Devices / Link Desktop Device, and enter the 2FA password in the terminal if prompted. The program lists up to 25 recent Saved Messages IDs without printing message text. Send a small test image or video to Saved Messages first. Run again with the same state directory to verify session reuse.

```sh
go run ./cmd/telegram-probe -state-dir "$HOME/.telegram-backup-probe" -message-id 123
unset TG_API_HASH TG_API_ID
```

Replace 123 with the selected media message ID. The download is capped at 32 MiB, saved via a temporary file, then published without overwriting an existing file. Verify the file locally. Remove test media manually once verified. QR images and sessions are sensitive; if the process is killed, inspect the private directory and remove stale QR files and probe.lock only after confirming no probe process is active.

Acceptance: compile/test pass, QR+optional 2FA works, session survives restart, history listing works, explicit media download completes and matches source, over-limit download leaves no completed file. Real Telegram and Drive tests must be run by an authorized operator; CI does not perform them. The standalone probe is not a full application integration test.
