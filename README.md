# TelegramBackup

Self-hosted Telegram media backup with a web dashboard.

TelegramBackup uses a Telegram user account through Telethon to scan chats available to that account, download photos, videos and documents, then upload them to cloud storage through rclone.

## Stack

- Backend: Python 3.12, FastAPI, Telethon, SQLAlchemy async, SQLite
- Frontend: React, TypeScript, Vite, Ant Design
- Storage: rclone
- Deployment: Docker Compose

## Quick start

1. Clone the repository and copy `.env.example` to `.env`.
2. Generate an application encryption key:

   ```bash
   python3 -c "import base64,os; print(base64.urlsafe_b64encode(os.urandom(32)).decode())"
   ```

   Put the generated value in `APP_ENCRYPTION_KEY`. Keep it safe: without this key, encrypted Telegram and cloud-storage credentials cannot be read.
3. Set a strong `ADMIN_TOKEN` in `.env`.
4. Build and start the service:

   ```bash
   docker compose build
   docker compose up -d
   ```
5. Open `http://localhost:8080` and sign in with `ADMIN_TOKEN`.
6. In **Settings → Telegram**, enter the API ID and API Hash from https://my.telegram.org, save them, then complete phone-code and optional 2FA login in the dashboard.
7. In **Settings → Cloud storage**, add a Google Drive remote. Use the OAuth Client ID and Client Secret from Google Cloud and paste the complete JSON token output from `rclone authorize`. The token JSON must contain a `refresh_token`.
8. Test the connection, select the default destination and save.
9. In **Settings → Backup policy**, set the automatic scan interval.

## Existing rclone configuration

If `./rclone/rclone.conf` exists, TelegramBackup imports its remotes into the encrypted application database on the first Settings request when `APP_ENCRYPTION_KEY` is configured. The original file is not deleted. After verifying the imported remote works, keep the old file protected or remove it manually if no other service needs it.

## Security

- Never commit `.env`, Telegram session files, rclone config files, OAuth tokens, or downloaded media.
- The application encrypts Telegram API Hash and the complete rclone configuration in the database using Fernet and `APP_ENCRYPTION_KEY`.
- Telegram session files remain in the private persistent `./data` volume. Anyone who obtains a Telethon session can access the Telegram account, so protect the volume and backups.
- Do not change `APP_ENCRYPTION_KEY` after saving credentials unless you first decrypt and re-encrypt the stored settings.
- Put the dashboard behind HTTPS and restrict public access with an IP allowlist or VPN.

## Development

Backend: install `backend/requirements.txt` and run `uvicorn app.main:app --reload` from `backend/`.

Frontend: run `npm install` and `npm run dev` from `frontend/`.

## Project status

The dashboard supports Telegram API configuration and phone-code / 2FA login, Google Drive remote creation from OAuth token JSON, existing rclone remote import, storage connection testing, default destination selection, and persistent backup scan interval settings. Run an end-to-end test with a dedicated Telegram chat before enabling large archives.
