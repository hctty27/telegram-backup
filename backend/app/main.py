import asyncio
import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI

from app.config import settings
from app.config_store import get_config_value
from app.database import SessionLocal, engine, init_database
from app.api import router
from app.services.backup_worker import BackupWorker
from app.services.storage import RcloneStorage
from app.services.telegram_client import TelegramService

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)


@asynccontextmanager
async def lifespan(app: FastAPI):
    await init_database()
    telegram = TelegramService()
    storage = RcloneStorage()

    async with SessionLocal() as db:
        api_id = await get_config_value(db, "telegram_api_id", str(settings.telegram_api_id) if settings.telegram_api_id else None)
        api_hash = await get_config_value(db, "telegram_api_hash", settings.telegram_api_hash)
    if api_id and api_hash:
        try:
            await telegram.configure(int(api_id), api_hash)
        except Exception:
            logging.getLogger(__name__).exception("Could not initialize Telegram session")

    worker = BackupWorker(telegram, storage)
    app.state.telegram = telegram
    app.state.storage = storage
    worker_task = asyncio.create_task(worker.run(), name="telegram-backup-worker")
    try:
        yield
    finally:
        worker.stop()
        worker_task.cancel()
        try:
            await worker_task
        except asyncio.CancelledError:
            pass
        await telegram.disconnect()
        await engine.dispose()


app = FastAPI(
    title="TelegramBackup API",
    version="0.2.0",
    description="Self-hosted Telegram media backup management API",
    lifespan=lifespan,
)
app.include_router(router)


@app.get("/api/health")
async def health():
    return {"status": "ok", "service": "telegram-backup"}
