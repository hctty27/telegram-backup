import asyncio
import os
from datetime import timezone
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
        self._qr_login: Any | None = None
        self._qr_task: asyncio.Task | None = None
        self._qr_status = "idle"
        self._qr_error: str | None = None
        self._qr_expires_at: str | None = None

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
        qr_task = self._qr_task
        if qr_task and qr_task is not asyncio.current_task() and not qr_task.done():
            qr_task.cancel()
            try:
                await qr_task
            except asyncio.CancelledError:
                pass
        self._qr_task = None
        self._qr_login = None
        self._qr_status = "idle"
        if self.client and self.client.is_connected():
            await self.client.disconnect()
        self.authorized = False

    async def start_qr_login(self) -> dict[str, str]:
        if not self.configured:
            raise RuntimeError("Save Telegram API ID and API Hash first.")
        client = self._ensure_client()
        if not client.is_connected():
            await client.connect()
        if await client.is_user_authorized():
            self.authorized = True
            self._qr_status = "authorized"
            raise RuntimeError("This Telegram session is already authorized.")
        if self._qr_status == "waiting" and self._qr_task and not self._qr_task.done():
            raise RuntimeError("A QR login is already waiting. Scan it or wait for it to expire.")
        if self._qr_status == "needs_password":
            raise RuntimeError("The QR code was accepted. Enter your Telegram two-step verification password.")
        qr_login = await client.qr_login()
        self._qr_login = qr_login
        self._qr_status = "waiting"
        self._qr_error = None
        expires = qr_login.expires
        if expires.tzinfo is None:
            expires = expires.replace(tzinfo=timezone.utc)
        self._qr_expires_at = expires.isoformat()
        self._qr_task = asyncio.create_task(self._wait_for_qr_login(qr_login), name="telegram-qr-login")
        return {"url": qr_login.url, "expires_at": self._qr_expires_at}

    async def _wait_for_qr_login(self, qr_login: Any) -> None:
        try:
            await qr_login.wait()
            self.authorized = bool(self.client and await self.client.is_user_authorized())
            self._qr_status = "authorized" if self.authorized else "failed"
            self._qr_error = None if self.authorized else "Telegram QR confirmation completed but the session is not authorized."
            self._secure_session_file()
        except SessionPasswordNeededError:
            self._qr_status = "needs_password"
        except asyncio.TimeoutError:
            self._qr_status = "expired"
        except asyncio.CancelledError:
            raise
        except Exception as exc:
            self._qr_status = "failed"
            self._qr_error = type(exc).__name__

    async def qr_login_status(self) -> dict[str, Any]:
        if self.client and self.client.is_connected():
            self.authorized = await self.client.is_user_authorized()
            if self.authorized:
                self._qr_status = "authorized"
        return {
            "status": self._qr_status,
            "authorized": self.authorized,
            "needs_password": self._qr_status == "needs_password",
            "expires_at": self._qr_expires_at,
            "error": self._qr_error,
        }

    async def send_login_code(self, phone: str) -> dict[str, Any]:
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

        sent_type = getattr(sent, "type", None)
        next_type = getattr(sent, "next_type", None)
        type_name = type(sent_type).__name__ if sent_type is not None else "Unknown"
        next_type_name = type(next_type).__name__ if next_type is not None else None
        labels = {
            "SentCodeTypeApp": "Telegram 已登录设备内通知",
            "SentCodeTypeSms": "手机短信",
            "SentCodeTypeCall": "语音电话",
            "SentCodeTypeFlashCall": "闪呼验证",
            "SentCodeTypeMissedCall": "未接来电验证",
            "SentCodeTypeEmailCode": "登录邮箱",
            "SentCodeTypeFragmentSms": "Fragment 验证",
            "SentCodeTypeSetUpEmailRequired": "需要先设置登录邮箱",
            "SentCodeTypeFirebaseSms": "Firebase 短信（官方移动端流程）",
        }
        next_labels = {
            "CodeTypeSms": "手机短信",
            "CodeTypeCall": "语音电话",
            "CodeTypeFlashCall": "闪呼验证",
            "CodeTypeMissedCall": "未接来电验证",
            "CodeTypeFragmentSms": "Fragment 验证",
        }
        return {
            "delivery_type": type_name,
            "delivery_label": labels.get(type_name, f"Telegram 返回的验证方式（{type_name}）"),
            "next_type": next_type_name,
            "next_label": next_labels.get(next_type_name, next_type_name) if next_type_name else None,
            "timeout": getattr(sent, "timeout", None),
            "email_pattern": getattr(sent_type, "email_pattern", None),
            "fragment_url": getattr(sent_type, "url", None),
        }

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
        if self.authorized:
            self._qr_status = "authorized"
            self._qr_error = None
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
