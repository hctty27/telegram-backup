# Deployment

## Prepare the host

Install Docker Engine and the Docker Compose plugin. Clone this repository, then enter the project directory.

Create the runtime configuration:

```bash
cp .env.example .env
mkdir -p data rclone
```

Edit .env and set:

- Telegram API ID and API hash from https://my.telegram.org
- A unique ADMIN_TOKEN with at least 32 characters
- The rclone destination, for example backup:TelegramBackup

Create the rclone configuration file at rclone/rclone.conf. You can create it interactively on a trusted machine with:

```bash
rclone config --config ./rclone/rclone.conf
```

Copy the resulting config file to the server through a secure channel. Do not commit it to Git.

## Authorize Telegram

Build the backend image and run the one-time login command:

```bash
docker compose build backend
docker compose run --rm backend python scripts/login_telegram.py
```

The login command writes a Telethon session under ./data. Keep this directory private and persistent. Do not run the login command while the backend is already using the same session file.

## Start

```bash
docker compose up -d
docker compose ps
docker compose logs -f backend
```

The web dashboard listens on port 8080. For an internet-facing deployment, put it behind a reverse proxy with HTTPS and restrict access with an IP allowlist or VPN where practical.

## Upgrade

```bash
git pull
docker compose build
docker compose up -d
```

The ./data directory contains the SQLite database, Telegram session and temporary downloads. Back it up securely before upgrades or host migration.

## Backup safety

- Test with a small private test chat before enabling large channels.
- Monitor Telegram rate limits and storage-provider quotas.
- Keep enough local disk space for the largest media file being downloaded.
- Keep .env, rclone/rclone.conf, and data/ out of Git and out of public web roots.
- The first version uses one backend worker and SQLite; do not scale the backend service to multiple replicas.
