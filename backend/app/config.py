from functools import lru_cache
from pathlib import Path

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    app_name: str = "TelegramBackup"
    admin_token: str = "replace-with-a-long-random-secret"
    app_encryption_key: str | None = None

    telegram_api_id: int | None = None
    telegram_api_hash: str | None = None
    telegram_session: str = "../data/telegram"

    data_dir: Path = Path("../data")
    download_dir: Path = Path("../data/downloads")
    database_url: str = "sqlite+aiosqlite:///../data/telegram_backup.db"

    rclone_config: Path = Path("../rclone/rclone.conf")
    rclone_remote: str = "backup:TelegramBackup"

    backup_scan_interval_seconds: int = 300
    worker_poll_seconds: int = 5

    model_config = SettingsConfigDict(
        env_file=Path(__file__).resolve().parents[2] / ".env",
        env_file_encoding="utf-8",
        env_ignore_empty=True,
        extra="ignore",
    )


@lru_cache
def get_settings() -> Settings:
    return Settings()


settings = get_settings()
