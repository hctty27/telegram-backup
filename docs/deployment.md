# Deployment

## Prepare the host

Install Docker Engine and the Docker Compose plugin. Clone the repository and enter the project directory.

```bash
cp .env.example .env
mkdir -p data rclone
```

Edit `.env` and set a strong, random `ADMIN_TOKEN`. Generate an application encryption key with Python's standard library:

```bash
python3 -c "import base64,os; print(base64.urlsafe_b64encode(os.urandom(32)).decode())"
```

Put the generated value in `APP_ENCRYPTION_KEY`. Keep it safe and do not change it after credentials have been saved. The application uses this key to encrypt Telegram API Hash and the complete rclone configuration in SQLite.

## Build and start

```bash
docker compose build
docker compose up -d
docker compose ps
docker compose logs -f backend
```

The dashboard listens on port 8080. For internet-facing deployment, put it behind HTTPS and restrict access with an IP allowlist or VPN.

## Configure from the dashboard

1. Open `http://SERVER_IP:8080` and sign in with `ADMIN_TOKEN`.
2. Open **Settings → Telegram account**. Enter the API ID and API Hash from https://my.telegram.org, save them, enter the phone number in international format, and complete the Telegram verification code and optional two-step password.
3. Open **Settings → Cloud storage**. Existing remotes in `./rclone/rclone.conf` are imported into the encrypted database when the encryption key is configured. You can also add a Google Drive remote by entering the OAuth Client ID, Client Secret and the full JSON token output from `rclone authorize` (the JSON must include `refresh_token`).
4. Test the storage connection and save the default destination, such as `gdrive:TelegramBackup`.
5. Open **Settings → Backup policy** and choose the automatic scan interval.
6. Synchronize Telegram groups, enable backup for the desired chats, and create a test backup.

## Existing rclone configuration

If `./rclone/rclone.conf` exists, the application imports its remotes into the encrypted application database on the first Settings request when `APP_ENCRYPTION_KEY` is configured. The original file is not deleted. After verifying the imported remote works, keep the old file protected or remove it manually if no other service needs it.

## Upgrade

```bash
git pull
docker compose build
docker compose up -d
```

The `./data` directory contains the SQLite database, encrypted application settings, Telegram session and temporary downloads. Back it up securely before upgrades or host migration. Back up `APP_ENCRYPTION_KEY` separately: losing it makes encrypted credentials unrecoverable.

## Backup safety

- Test with a small private test chat before enabling large channels.
- Monitor Telegram rate limits and cloud-storage quotas.
- Keep enough local disk space for the largest media file being downloaded.
- Keep `.env`, `rclone/rclone.conf`, and `data/` out of Git and public web roots.
- Keep the Telegram session file private; anyone who obtains it may be able to access the Telegram account.
- The first version uses one backend worker and SQLite; do not scale the backend service to multiple replicas.
