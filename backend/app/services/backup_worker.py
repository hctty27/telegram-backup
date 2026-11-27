import asyncio
import logging
import re
from datetime import datetime, timedelta
from pathlib import Path

from sqlalchemy import func, select

from app.config import settings
from app.database import SessionLocal
from app.models import BackupTask, Chat, MediaFile
from app.services.storage import RcloneStorage
from app.services.telegram_client import TelegramService

logger = logging.getLogger(__name__)


def safe_filename(value: str) -> str:
    name = Path(value).name
    name = re.sub(r"[^A-Za-z0-9._-]+", "_", name).strip("._")
    return name[:180] or "telegram-media.bin"


class BackupWorker:
    def __init__(self, telegram: TelegramService, storage: RcloneStorage) -> None:
        self.telegram = telegram
        self.storage = storage
        self._stop = asyncio.Event()

    def stop(self) -> None:
        self._stop.set()

    async def run(self) -> None:
        await self._recover_running_tasks()
        while not self._stop.is_set():
            try:
                if not self.telegram.connected:
                    await self.telegram.connect()
                if self.telegram.connected:
                    await self._enqueue_due_chats()
                    await self._process_next_task()
            except asyncio.CancelledError:
                raise
            except Exception:
                logger.exception("Backup worker iteration failed")

            try:
                await asyncio.wait_for(
                    self._stop.wait(),
                    timeout=max(1, settings.worker_poll_seconds),
                )
            except TimeoutError:
                pass

    async def _recover_running_tasks(self) -> None:
        async with SessionLocal() as db:
            result = await db.execute(
                select(BackupTask).where(BackupTask.status == "running")
            )
            for task in result.scalars():
                task.status = "queued"
                task.error = "Worker restarted; task returned to queue."
            await db.commit()

    async def _enqueue_due_chats(self) -> None:
        now = datetime.utcnow()
        cutoff = now - timedelta(seconds=max(60, settings.backup_scan_interval_seconds))
        async with SessionLocal() as db:
            result = await db.execute(
                select(Chat).where(Chat.backup_enabled.is_(True))
            )
            for chat in result.scalars():
                latest_attempt = await db.scalar(
                    select(func.max(BackupTask.finished_at)).where(
                        BackupTask.chat_id == chat.id
                    )
                )
                last_activity = max(
                    (value for value in (chat.last_synced_at, latest_attempt) if value),
                    default=None,
                )
                if last_activity and last_activity > cutoff:
                    continue

                active_task = await db.scalar(
                    select(BackupTask.id)
                    .where(
                        BackupTask.chat_id == chat.id,
                        BackupTask.status.in_(["queued", "running"]),
                    )
                    .limit(1)
                )
                if active_task is None:
                    db.add(BackupTask(chat_id=chat.id, status="queued"))
            await db.commit()

    async def _process_next_task(self) -> None:
        async with SessionLocal() as db:
            task = await db.scalar(
                select(BackupTask)
                .where(BackupTask.status == "queued")
                .order_by(BackupTask.created_at, BackupTask.id)
                .limit(1)
            )
            if task is None:
                return
            task.status = "running"
            task.started_at = datetime.utcnow()
            task.error = None
            task_id = task.id
            await db.commit()

        await self._run_task(task_id)

    async def _run_task(self, task_id: int) -> None:
        async with SessionLocal() as db:
            task = await db.get(BackupTask, task_id)
            if task is None:
                return
            chat = await db.get(Chat, task.chat_id)
            if chat is None:
                task.status = "failed"
                task.error = "Chat no longer exists."
                task.finished_at = datetime.utcnow()
                await db.commit()
                return

            try:
                if not self.telegram.connected:
                    raise RuntimeError("Telegram account is not authorized.")

                async for message in self.telegram.iter_messages(
                    chat.telegram_id,
                    min_id=chat.last_message_id,
                ):
                    task.scanned_messages += 1
                    if not message.media or not message.file:
                        chat.last_message_id = max(chat.last_message_id, message.id)
                        await db.commit()
                        continue

                    media = await db.scalar(
                        select(MediaFile).where(
                            MediaFile.chat_id == chat.id,
                            MediaFile.telegram_message_id == message.id,
                        )
                    )
                    if media and media.status == "uploaded":
                        chat.last_message_id = max(chat.last_message_id, message.id)
                        await db.commit()
                        continue

                    original_name = message.file.name or (
                        f"telegram-{message.id}{message.file.ext or '.bin'}"
                    )
                    filename = safe_filename(original_name)
                    local_target = (
                        settings.download_dir
                        / str(chat.telegram_id)
                        / f"{message.id}-{filename}"
                    )
                    media_type = getattr(message.file, "mime_type", None)
                    file_size = int(getattr(message.file, "size", 0) or 0)

                    if media is None:
                        media = MediaFile(
                            chat_id=chat.id,
                            telegram_message_id=message.id,
                            file_name=filename,
                            mime_type=media_type,
                            file_size=file_size,
                            status="downloading",
                        )
                        db.add(media)
                    else:
                        media.status = "downloading"
                        media.error = None
                        media.file_name = filename
                        media.mime_type = media_type
                        media.file_size = file_size
                    await db.commit()

                    local_path = (
                        Path(media.local_path)
                        if media.local_path and Path(media.local_path).is_file()
                        else None
                    )
                    if local_path is None:
                        local_path = await self.telegram.download_media(
                            message,
                            local_target,
                        )
                    if local_path is None:
                        raise RuntimeError(
                            f"Telegram returned no file for message {message.id}"
                        )

                    media.local_path = str(local_path)
                    media.status = "uploading"
                    await db.commit()

                    message_date = message.date
                    remote_path = (
                        f"{settings.rclone_remote.rstrip('/')}/"
                        f"{chat.telegram_id}/"
                        f"{message_date:%Y/%m}/"
                        f"{message.id}-{filename}"
                    )
                    await self.storage.upload(local_path, remote_path)

                    media.remote_path = remote_path
                    media.local_path = None
                    media.status = "uploaded"
                    media.error = None
                    media.uploaded_at = datetime.utcnow()
                    task.downloaded_files += 1
                    task.uploaded_files += 1
                    chat.last_message_id = max(chat.last_message_id, message.id)
                    await db.commit()

                    try:
                        local_path.unlink(missing_ok=True)
                    except OSError:
                        logger.warning("Could not remove temporary file %s", local_path)

                task.status = "completed"
                task.finished_at = datetime.utcnow()
                task.error = None
                chat.last_synced_at = datetime.utcnow()
                await db.commit()
                logger.info("Backup task %s completed for chat %s", task_id, chat.title)

            except Exception as exc:
                logger.exception("Backup task %s failed", task_id)
                task.status = "failed"
                task.error = str(exc)[:4000]
                task.finished_at = datetime.utcnow()
                await db.commit()
