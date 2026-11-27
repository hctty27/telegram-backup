from datetime import datetime

from pydantic import BaseModel, ConfigDict


class ChatUpdate(BaseModel):
    backup_enabled: bool


class ChatOut(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    telegram_id: int
    title: str
    username: str | None
    chat_type: str
    backup_enabled: bool
    last_message_id: int
    last_synced_at: datetime | None


class TaskOut(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    chat_id: int
    status: str
    scanned_messages: int
    downloaded_files: int
    uploaded_files: int
    error: str | None
    created_at: datetime
    started_at: datetime | None
    finished_at: datetime | None
