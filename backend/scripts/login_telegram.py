import asyncio
from getpass import getpass
from pathlib import Path

from telethon import TelegramClient

from app.config import settings


async def main() -> None:
    if not settings.telegram_api_id or not settings.telegram_api_hash:
        raise SystemExit("Set TELEGRAM_API_ID and TELEGRAM_API_HASH in .env first.")

    Path(settings.telegram_session).parent.mkdir(parents=True, exist_ok=True)
    client = TelegramClient(
        settings.telegram_session,
        settings.telegram_api_id,
        settings.telegram_api_hash,
    )

    phone = input("Telegram phone number (with country code): ").strip()
    await client.start(
        phone=phone,
        code_callback=lambda: input("Telegram login code: ").strip(),
        password=lambda: getpass("Telegram 2FA password: "),
    )
    me = await client.get_me()
    print(f"Telegram session authorized for user id {me.id}.")
    await client.disconnect()


if __name__ == "__main__":
    asyncio.run(main())
