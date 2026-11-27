from datetime import datetime

from pydantic import BaseModel, ConfigDict, Field


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


class TelegramCredentialsIn(BaseModel):
    api_id: int = Field(gt=0)
    api_hash: str | None = None


class TelegramPhoneIn(BaseModel):
    phone: str = Field(min_length=5, max_length=32)


class TelegramCodeIn(BaseModel):
    code: str = Field(min_length=3, max_length=16)


class TelegramPasswordIn(BaseModel):
    password: str = Field(min_length=1, max_length=256)


class GoogleDriveRemoteIn(BaseModel):
    name: str = Field(min_length=1, max_length=64)
    client_id: str = Field(min_length=1, max_length=512)
    client_secret: str = Field(min_length=1, max_length=512)
    token_json: str = Field(min_length=2, max_length=20000)
    root_folder_id: str = Field(default="", max_length=512)


class StorageDestinationIn(BaseModel):
    destination: str = Field(min_length=2, max_length=1024)


class StorageTestIn(BaseModel):
    destination: str | None = Field(default=None, max_length=1024)


class BackupSettingsIn(BaseModel):
    scan_interval_seconds: int = Field(ge=60, le=86400)
