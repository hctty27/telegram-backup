import os
from pathlib import Path
from typing import Any, AsyncIterator

from telethon import TelegramClient
from telethon.errors import SessionPasswordNeededError

from app.config import settings


class TelegramService:
    def __init__(self) -> None:
        self.client: TelegramClient | None = None
        self.api_id: int | None = None
        self.api_hash: str | None = None
        self.authorized = False
        self._login_phone: str | None = None
        self._phone_code_hash: str | None = None

    @property
    def configured(self) -> bool:
        return bool(self.api_id and self.api_hash)

    @property
    def connected(self) -> bool:
        return bool(self.client and self.client.is_connected() and self.authorized)

    async def configure(self, api_id: int, api_hash: str) -> bool:
        changed = self.api_id != int(api_id) or self.api_hash != api_hash
        if changed and self.client is not None:
            await self.disconnect()
            self.client = None
        self.api_id = int(api_id)
        self.api_hash = api_hash
        self._login_phone = None
        self._phone_code_hash = None
        return await self.connect()

    def _secure_session_file(self) -> None:
        session_path = Path(settings.telegram_session)
        if not session_path.suffix:
            session_path = session_path.with_suffix(".session")
        if session_path.is_file():
            try:
                os.chmod(session_path, 0o600)
            except OSError:
                pass

    def _ensure_client(self) -> TelegramClient:
        if not self.configured:
            raise RuntimeError("Telegram API ID and API Hash have not been configured.")
        if self.client is None:
            Path(settings.telegram_session).parent.mkdir(parents=True, exist_ok=True)
            self.client = TelegramClient(
                settings.telegram_session,
                self.api_id,
                self.api_hash,
            )
        return self.client

    async def connect(self) -> bool:
        if not self.configured:
            return False
        client = self._ensure_client()
        if not client.is_connected():
            await client.connect()
        self.authorized = await client.is_user_authorized()
        self._secure_session_file()
        return self.authorized

    async def disconnect(self) -> None:
        if self.client and self.client.is_connected():
            await self.client.disconnect()
        self.authorized = False

    async def send_login_code(self, phone: str) -> None:
        if not self.configured:
            raise RuntimeError("Save Telegram API ID and API Hash first.")
        client = self._ensure_client()
        if not client.is_connected():
            await client.connect()
        if await client.is_user_authorized():
            self.authorized = True
            raise RuntimeError("This Telegram session is already authorized.")
        sent = await client.send_code_request(phone)
        self._login_phone = phone
        self._phone_code_hash = sent.phone_code_hash
        self._secure_session_file()

    async def verify_login_code(self, code: str) -> bool:
        if not self.client or not self.client.is_connected():
            raise RuntimeError("Login session expired. Request a new verification code.")
        if not self._login_phone or not self._phone_code_hash:
            raise RuntimeError("No pending Telegram login. Request a verification code first.")
        try:
            await self.client.sign_in(
                phone=self._login_phone,
                code=code,
                phone_code_hash=self._phone_code_hash,
            )
        except SessionPasswordNeededError:
            return False
        self.authorized = await self.client.is_user_authorized()
        self._secure_session_file()
        self._login_phone = None
        self._phone_code_hash = None
        return self.authorized

    async def verify_login_password(self, password: str) -> bool:
        if not self.client or not self.client.is_connected():
            raise RuntimeError("Login session expired. Request a new verification code.")
        await self.client.sign_in(password=password)
        self.authorized = await self.client.is_user_authorized()
        self._secure_session_file()
        self._login_phone = None
        self._phone_code_hash = None
        return self.authorized

    async def list_dialogs(self) -> list[dict[str, Any]]:
        if not self.connected:
            await self.connect()
        if not self.connected or self.client is None:
            raise RuntimeError("Telegram account is not authorized")

        items: list[dict[str, Any]] = []
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
            await self.connect()
        if not self.connected or self.client is None:
            raise RuntimeError("Telegram account is not authorized")

        async for message in self.client.iter_messages(
            telegram_chat_id,
            min_id=min_id,
            reverse=True,
        ):
            yield message

    async def download_media(self, message: Any, target: Path, progress_callback=None) -> Path | None:
        if not self.connected or self.client is None:
            raise RuntimeError("Telegram account is not authorized")

        target.parent.mkdir(parents=True, exist_ok=True)
        result = await self.client.download_media(message, file=str(target), progress_callback=progress_callback)
        return Path(result) if result else None
