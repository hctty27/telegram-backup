import asyncio
import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI

from app.api import router
from app.database import engine, init_database
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
    worker = BackupWorker(telegram, RcloneStorage())
    app.state.telegram = telegram
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
    version="0.1.0",
    description="Self-hosted Telegram media backup management API",
    lifespan=lifespan,
)
app.include_router(router)


@app.get("/api/health")
async def health():
    return {"status": "ok", "service": "telegram-backup"}
