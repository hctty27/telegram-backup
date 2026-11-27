from cryptography.fernet import Fernet, InvalidToken
from sqlalchemy.ext.asyncio import AsyncSession

from app.config import settings
from app.models import AppSetting


def encryption_ready() -> bool:
    if not settings.app_encryption_key:
        return False
    try:
        Fernet(settings.app_encryption_key.encode("ascii"))
        return True
    except (ValueError, UnicodeEncodeError):
        return False


def _fernet() -> Fernet:
    if not settings.app_encryption_key:
        raise RuntimeError(
            "APP_ENCRYPTION_KEY is not configured. Generate a Fernet key and add it to .env."
        )
    try:
        return Fernet(settings.app_encryption_key.encode("ascii"))
    except (ValueError, UnicodeEncodeError) as exc:
        raise RuntimeError("APP_ENCRYPTION_KEY is invalid. It must be a Fernet key.") from exc


async def get_config_value(
    db: AsyncSession,
    key: str,
    default: str | None = None,
) -> str | None:
    row = await db.get(AppSetting, key)
    if row is None:
        return default
    if not row.encrypted:
        return row.value
    try:
        return _fernet().decrypt(row.value.encode("ascii")).decode("utf-8")
    except InvalidToken as exc:
        raise RuntimeError(f"Cannot decrypt stored setting: {key}") from exc


async def set_config_value(
    db: AsyncSession,
    key: str,
    value: str,
    *,
    secret: bool = False,
) -> None:
    stored_value = (
        _fernet().encrypt(value.encode("utf-8")).decode("ascii")
        if secret
        else value
    )
    row = await db.get(AppSetting, key)
    if row is None:
        row = AppSetting(key=key, value=stored_value, encrypted=secret)
        db.add(row)
    else:
        row.value = stored_value
        row.encrypted = secret


async def delete_config_value(db: AsyncSession, key: str) -> None:
    row = await db.get(AppSetting, key)
    if row is not None:
        await db.delete(row)
