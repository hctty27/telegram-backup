# TelegramBackup

Self-hosted Telegram media backup with a web dashboard.

TelegramBackup uses a Telegram user account through Telethon to scan chats available to that account, download photos, videos and documents, then upload them to any cloud storage supported by rclone.

## Stack

- Backend: Python 3.12, FastAPI, Telethon, SQLAlchemy async, SQLite
- Frontend: React, TypeScript, Vite, Ant Design
- Storage: rclone
- Deployment: Docker Compose

## Quick start

1. Create a Telegram application at https://my.telegram.org and obtain an API ID and API hash.
2. Copy .env.example to .env. Set TELEGRAM_API_ID, TELEGRAM_API_HASH and a strong ADMIN_TOKEN.
3. Configure an rclone remote in ./rclone/rclone.conf and set RCLONE_REMOTE.
4. Bootstrap the Telegram session before starting the API:

   docker compose build
   docker compose run --rm backend python scripts/login_telegram.py

5. Start the service:

   docker compose up -d

6. Open http://localhost:8080 and sign in with ADMIN_TOKEN.

## Security

Never commit .env, Telegram session files, rclone.conf, access tokens, or downloaded media. A Telegram session grants access to the chats visible to that account. Protect the server data volume and put the dashboard behind HTTPS before exposing it to the internet.

The public repository contains source code only. Credentials, session files, chat metadata, and backups must remain private.

## Development

Backend: install backend/requirements.txt and run uvicorn app.main:app --reload from backend/.

Frontend: run npm install and npm run dev from frontend/. Vite proxies /api to http://localhost:8000.

## Project status

Initial MVP scaffold: authenticated management API, dashboard, Telegram session bootstrap, dialog import, persistent backup tasks, incremental media scanning, deduplication, and rclone upload pipeline. Validate against a dedicated test chat before enabling large-scale backups.

## Roadmap

- [x] Repository and application scaffold
- [x] Authenticated API and management dashboard
- [x] Telegram session bootstrap and dialog import
- [x] Persistent tasks and media deduplication
- [x] Incremental media download and rclone upload
- [ ] Integration tests with a dedicated Telegram test chat
- [ ] Backup verification and restore workflow
- [ ] Retention policies and multi-user roles
