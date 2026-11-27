from fastapi import APIRouter, Depends, HTTPException, Query, Request, status
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.config import settings
from app.database import get_db
from app.models import BackupTask, Chat, MediaFile
from app.schemas import ChatOut, ChatUpdate, TaskOut
from app.security import require_admin

router = APIRouter(
    prefix="/api",
    dependencies=[Depends(require_admin)],
)


@router.get("/dashboard")
async def dashboard(db: AsyncSession = Depends(get_db)):
    total_chats = await db.scalar(select(func.count(Chat.id))) or 0
    enabled_chats = await db.scalar(
        select(func.count(Chat.id)).where(Chat.backup_enabled.is_(True))
    ) or 0
    uploaded_files = await db.scalar(
        select(func.count(MediaFile.id)).where(MediaFile.status == "uploaded")
    ) or 0
    total_bytes = await db.scalar(
        select(func.coalesce(func.sum(MediaFile.file_size), 0)).where(
            MediaFile.status == "uploaded"
        )
    ) or 0
    queued_tasks = await db.scalar(
        select(func.count(BackupTask.id)).where(BackupTask.status == "queued")
    ) or 0
    running_tasks = await db.scalar(
        select(func.count(BackupTask.id)).where(BackupTask.status == "running")
    ) or 0
    failed_tasks = await db.scalar(
        select(func.count(BackupTask.id)).where(BackupTask.status == "failed")
    ) or 0

    return {
        "total_chats": total_chats,
        "enabled_chats": enabled_chats,
        "uploaded_files": uploaded_files,
        "total_bytes": int(total_bytes),
        "queued_tasks": queued_tasks,
        "running_tasks": running_tasks,
        "failed_tasks": failed_tasks,
    }


@router.get("/chats", response_model=list[ChatOut])
async def list_chats(db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(Chat).order_by(Chat.title))
    return result.scalars().all()


@router.post("/chats/sync")
async def sync_chats(request: Request, db: AsyncSession = Depends(get_db)):
    telegram = request.app.state.telegram
    if not telegram.connected:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Telegram is not authorized. Run the Telegram session login command first.",
        )

    dialogs = await telegram.list_dialogs()
    imported = 0
    for item in dialogs:
        chat = await db.scalar(
            select(Chat).where(Chat.telegram_id == item["telegram_id"])
        )
        if chat is None:
            db.add(Chat(**item))
            imported += 1
        else:
            chat.title = item["title"]
            chat.username = item["username"]
            chat.chat_type = item["chat_type"]
    await db.commit()
    return {"imported": imported, "scanned": len(dialogs)}


@router.patch("/chats/{chat_id}", response_model=ChatOut)
async def update_chat(
    chat_id: int,
    payload: ChatUpdate,
    db: AsyncSession = Depends(get_db),
):
    chat = await db.get(Chat, chat_id)
    if chat is None:
        raise HTTPException(status_code=404, detail="Chat not found")
    chat.backup_enabled = payload.backup_enabled
    await db.commit()
    await db.refresh(chat)
    return chat


@router.post("/chats/{chat_id}/backup", response_model=TaskOut)
async def start_backup(chat_id: int, db: AsyncSession = Depends(get_db)):
    chat = await db.get(Chat, chat_id)
    if chat is None:
        raise HTTPException(status_code=404, detail="Chat not found")

    active = await db.scalar(
        select(BackupTask)
        .where(
            BackupTask.chat_id == chat_id,
            BackupTask.status.in_(["queued", "running"]),
        )
        .limit(1)
    )
    if active:
        raise HTTPException(
            status_code=409,
            detail="A backup task for this chat is already queued or running.",
        )

    task = BackupTask(chat_id=chat_id, status="queued")
    db.add(task)
    await db.commit()
    await db.refresh(task)
    return task


@router.get("/tasks", response_model=list[TaskOut])
async def list_tasks(
    limit: int = Query(default=100, ge=1, le=500),
    db: AsyncSession = Depends(get_db),
):
    result = await db.execute(
        select(BackupTask).order_by(BackupTask.created_at.desc()).limit(limit)
    )
    return result.scalars().all()


@router.post("/tasks/{task_id}/retry", response_model=TaskOut)
async def retry_task(task_id: int, db: AsyncSession = Depends(get_db)):
    task = await db.get(BackupTask, task_id)
    if task is None:
        raise HTTPException(status_code=404, detail="Task not found")
    if task.status != "failed":
        raise HTTPException(status_code=409, detail="Only failed tasks can be retried")

    task.status = "queued"
    task.error = None
    task.finished_at = None
    task.started_at = None
    task.scanned_messages = 0
    task.downloaded_files = 0
    task.uploaded_files = 0
    await db.commit()
    await db.refresh(task)
    return task


@router.get("/settings")
async def get_settings(request: Request):
    telegram = request.app.state.telegram
    return {
        "telegram_configured": telegram.configured,
        "telegram_authorized": telegram.connected,
        "rclone_configured": settings.rclone_config.is_file(),
        "rclone_remote": settings.rclone_remote,
        "scan_interval_seconds": settings.backup_scan_interval_seconds,
    }
