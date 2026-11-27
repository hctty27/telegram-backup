from pathlib import Path
from typing import Any, AsyncIterator

from telethon import TelegramClient

from app.config import settings


class TelegramService:
    def __init__(self) -> None:
        self.client: TelegramClient | None = None
        self.authorized = False

    @property
    def configured(self) -> bool:
        return bool(settings.telegram_api_id and settings.telegram_api_hash)

    @property
    def connected(self) -> bool:
        return bool(self.client and self.client.is_connected() and self.authorized)

    async def connect(self) -> bool:
        if not self.configured:
            return False

        if self.client is None:
            Path(settings.telegram_session).parent.mkdir(parents=True, exist_ok=True)
            self.client = TelegramClient(
                settings.telegram_session,
                settings.telegram_api_id,
                settings.telegram_api_hash,
            )

        if not self.client.is_connected():
            await self.client.connect()

        self.authorized = await self.client.is_user_authorized()
        return self.authorized

    async def disconnect(self) -> None:
        if self.client and self.client.is_connected():
            await self.client.disconnect()
        self.authorized = False

    async def list_dialogs(self) -> list[dict[str, Any]]:
        if not self.connected:
            raise RuntimeError("Telegram account is not authorized")

        items: list[dict[str, Any]] = []
        assert self.client is not None
        async for dialog in self.client.iter_dialogs():
            entity = dialog.entity
            entity_name = type(entity).__name__.lower()
            if "channel" in entity_name:
                chat_type = "channel" if getattr(entity, "broadcast", False) else "supergroup"
            elif "chat" in entity_name:
                chat_type = "group"
            else:
                chat_type = "private"

            items.append(
                {
                    "telegram_id": int(dialog.id),
                    "title": dialog.name or str(dialog.id),
                    "username": getattr(entity, "username", None),
                    "chat_type": chat_type,
                }
            )
        return items

    async def iter_messages(
        self,
        telegram_chat_id: int,
        min_id: int = 0,
    ) -> AsyncIterator[Any]:
        if not self.connected or self.client is None:
            raise RuntimeError("Telegram account is not authorized")

        async for message in self.client.iter_messages(
            telegram_chat_id,
            min_id=min_id,
            reverse=True,
        ):
            yield message

    async def download_media(self, message: Any, target: Path) -> Path | None:
        if not self.connected or self.client is None:
            raise RuntimeError("Telegram account is not authorized")

        target.parent.mkdir(parents=True, exist_ok=True)
        result = await self.client.download_media(message, file=str(target))
        return Path(result) if result else None
