# Architecture

## Components

- **React dashboard**: presents system status, imported chats, backup tasks, and connection settings.
- **FastAPI**: authenticated management API. The health endpoint is public for container health checks; all management endpoints require the admin bearer token.
- **Telethon service**: owns the Telegram user session, imports dialogs, iterates message history, and downloads media.
- **Backup worker**: persistent SQLite task queue; scans enabled chats incrementally and processes one task at a time.
- **rclone adapter**: uploads downloaded files to the configured remote. Storage provider details stay in rclone configuration.

## Data flow

1. The operator bootstraps a Telegram user session on the server.
2. The dashboard requests a dialog sync. Dialog metadata is stored in SQLite.
3. The operator enables selected chats or queues an immediate backup.
4. The worker scans messages after the last successfully processed message ID.
5. Media is deduplicated by local chat ID plus Telegram message ID.
6. The worker downloads the media, uploads it with rclone, then advances the chat cursor.
7. Failed uploads retain their local temporary file where possible and leave the cursor at the failed message so a retry can resume.

## Storage layout

The remote layout is:

```text
<RCLONE_REMOTE>/<telegram-chat-id>/<year>/<month>/<message-id>-<filename>
```

The local data volume contains the SQLite database, Telegram session, and temporary downloads. Back it up securely; it contains sensitive account access material.

## Operational boundaries

- This is a single-process, single-worker MVP. Do not run multiple backend replicas against the same SQLite database or Telegram session.
- Incremental scans are periodic; they are not a real-time event stream.
- Only chats visible to the authenticated Telegram user account can be scanned.
- rclone is invoked as a subprocess. The application does not expose rclone's remote-control API.
- Use HTTPS and a strong admin token before making the dashboard reachable from the public internet.
