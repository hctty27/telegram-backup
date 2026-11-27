from fastapi import APIRouter, Depends, HTTPException, Query, Request, status
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.config import settings
from app.config_store import encryption_ready, get_config_value, set_config_value
from app.database import get_db
from app.models import BackupTask, Chat, MediaFile
from app.schemas import (
    BackupSettingsIn,
    ChatOut,
    ChatUpdate,
    GoogleDriveRemoteIn,
    StorageDestinationIn,
    StorageTestIn,
    TaskOut,
    TelegramCodeIn,
    TelegramCredentialsIn,
    TelegramPasswordIn,
    TelegramPhoneIn,
)
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
        await telegram.connect()
    if not telegram.connected:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Telegram is not authorized. Configure Telegram API credentials and complete login in Settings.",
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
async def get_settings(request: Request, db: AsyncSession = Depends(get_db)):
    storage = request.app.state.storage
    await storage.import_legacy_config(db)
    api_id = await get_config_value(db, "telegram_api_id", str(settings.telegram_api_id) if settings.telegram_api_id else None)
    api_hash = await get_config_value(db, "telegram_api_hash", settings.telegram_api_hash)
    destination = await get_config_value(db, "storage_destination", settings.rclone_remote)
    remotes = await storage.list_remotes()
    telegram = request.app.state.telegram
    return {
        "telegram_configured": bool(api_id and api_hash),
        "telegram_authorized": telegram.connected,
        "telegram_api_id": int(api_id) if api_id else None,
        "rclone_configured": bool(remotes),
        "rclone_remote": destination,
        "rclone_remotes": remotes,
        "scan_interval_seconds": int(
            await get_config_value(
                db,
                "backup_scan_interval_seconds",
                str(settings.backup_scan_interval_seconds),
            ) or settings.backup_scan_interval_seconds
        ),
        "encryption_ready": encryption_ready(),
    }


@router.put("/settings/telegram")
async def update_telegram_credentials(
    payload: TelegramCredentialsIn,
    request: Request,
    db: AsyncSession = Depends(get_db),
):
    if not encryption_ready():
        raise HTTPException(status_code=503, detail="APP_ENCRYPTION_KEY is not configured or invalid.")
    api_hash = (payload.api_hash or "").strip()
    if api_hash:
        await set_config_value(db, "telegram_api_hash", api_hash, secret=True)
    else:
        api_hash = await get_config_value(db, "telegram_api_hash", settings.telegram_api_hash) or ""
    if not api_hash:
        raise HTTPException(status_code=400, detail="Telegram API Hash is required.")
    await set_config_value(db, "telegram_api_id", str(payload.api_id))
    await db.commit()
    try:
        await request.app.state.telegram.configure(payload.api_id, api_hash)
    except Exception:
        # Credentials are persisted; the background worker will retry the Telegram connection.
        pass
    return {
        "configured": True,
        "authorized": request.app.state.telegram.connected,
        "message": "Telegram API credentials saved. Complete account login below.",
    }


@router.post("/settings/telegram/send-code")
async def send_telegram_code(payload: TelegramPhoneIn, request: Request):
    try:
        await request.app.state.telegram.send_login_code(payload.phone.strip())
    except RuntimeError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    except Exception as exc:
        raise HTTPException(status_code=400, detail="Telegram could not send a code. Check the phone number and try again.") from exc
    return {"sent": True}


@router.post("/settings/telegram/verify-code")
async def verify_telegram_code(payload: TelegramCodeIn, request: Request):
    try:
        authorized = await request.app.state.telegram.verify_login_code(payload.code.strip())
    except RuntimeError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    except Exception as exc:
        raise HTTPException(status_code=400, detail="Telegram verification failed. Check the code and try again.") from exc
    return {"authorized": authorized, "needs_password": not authorized}


@router.post("/settings/telegram/verify-password")
async def verify_telegram_password(payload: TelegramPasswordIn, request: Request):
    try:
        authorized = await request.app.state.telegram.verify_login_password(payload.password)
    except Exception as exc:
        raise HTTPException(status_code=400, detail="Telegram two-step verification failed. Check the password and try again.") from exc
    return {"authorized": authorized}


@router.post("/settings/storage/google-drive")
async def add_google_drive_remote(
    payload: GoogleDriveRemoteIn,
    request: Request,
):
    try:
        destination = await request.app.state.storage.create_google_drive_remote(
            name=payload.name.strip(),
            client_id=payload.client_id,
            client_secret=payload.client_secret,
            token_json=payload.token_json,
            root_folder_id=payload.root_folder_id,
        )
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    except RuntimeError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    return {"created": True, "destination": destination}


@router.post("/settings/storage/test")
async def test_storage(
    payload: StorageTestIn,
    request: Request,
):
    try:
        await request.app.state.storage.test_destination(payload.destination)
    except (ValueError, RuntimeError) as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    return {"success": True, "message": "Cloud storage connection succeeded."}


@router.put("/settings/storage/default")
async def set_default_storage(
    payload: StorageDestinationIn,
    request: Request,
):
    try:
        destination = await request.app.state.storage.set_destination(payload.destination)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    return {"destination": destination}


@router.put("/settings/backup")
async def update_backup_settings(
    payload: BackupSettingsIn,
    db: AsyncSession = Depends(get_db),
):
    await set_config_value(
        db,
        "backup_scan_interval_seconds",
        str(payload.scan_interval_seconds),
    )
    await db.commit()
    return {"scan_interval_seconds": payload.scan_interval_seconds}


@router.get("/monitor")
async def transfer_monitor():
    from app.monitor import monitor
    return monitor.snapshot()


@router.get("/archive")
async def archive_files(
    q: str = Query(default="", max_length=200),
    chat_id: int | None = None,
    kind: str = Query(default="all", pattern="^(all|image|video|other)$"),
    page: int = Query(default=1, ge=1, le=100000),
    page_size: int = Query(default=30, ge=1, le=100),
    db: AsyncSession = Depends(get_db),
):
    filters = [MediaFile.status == "uploaded"]
    if q:
        filters.append(MediaFile.file_name.contains(q, autoescape=True))
    if chat_id is not None:
        filters.append(MediaFile.chat_id == chat_id)
    if kind in ("image", "video"):
        filters.append(MediaFile.mime_type.like(kind + "/%"))
    elif kind == "other":
        mime = func.coalesce(MediaFile.mime_type, "")
        filters.extend([~mime.like("image/%"), ~mime.like("video/%")])
    total = await db.scalar(select(func.count(MediaFile.id)).where(*filters)) or 0
    rows = await db.execute(select(MediaFile, Chat.title).join(Chat, Chat.id == MediaFile.chat_id)
        .where(*filters).order_by(MediaFile.id.desc()).offset((page - 1) * page_size).limit(page_size))
    return {"total": total, "items": [
        {"id": file.id, "chat_title": title, "file_name": file.file_name,
         "mime_type": file.mime_type, "file_size": file.file_size,
         "remote_path": file.remote_path, "uploaded_at": file.uploaded_at}
        for file, title in rows
    ]}
