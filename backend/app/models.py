from datetime import datetime

from sqlalchemy import (
    BigInteger,
    Boolean,
    DateTime,
    ForeignKey,
    Integer,
    String,
    Text,
    UniqueConstraint,
)
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column


class Base(DeclarativeBase):
    pass


class AppSetting(Base):
    __tablename__ = "app_settings"

    key: Mapped[str] = mapped_column(String(128), primary_key=True)
    value: Mapped[str] = mapped_column(Text, default="")
    encrypted: Mapped[bool] = mapped_column(Boolean, default=False)
    updated_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


class Chat(Base):
    __tablename__ = "chats"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    telegram_id: Mapped[int] = mapped_column(BigInteger, unique=True, index=True)
    title: Mapped[str] = mapped_column(String(512), default="")
    username: Mapped[str | None] = mapped_column(String(255), nullable=True)
    chat_type: Mapped[str] = mapped_column(String(64), default="unknown")
    backup_enabled: Mapped[bool] = mapped_column(Boolean, default=False)
    last_message_id: Mapped[int] = mapped_column(Integer, default=0)
    last_synced_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow)


class BackupTask(Base):
    __tablename__ = "backup_tasks"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    chat_id: Mapped[int] = mapped_column(ForeignKey("chats.id"), index=True)
    status: Mapped[str] = mapped_column(String(32), default="queued", index=True)
    scanned_messages: Mapped[int] = mapped_column(Integer, default=0)
    downloaded_files: Mapped[int] = mapped_column(Integer, default=0)
    uploaded_files: Mapped[int] = mapped_column(Integer, default=0)
    error: Mapped[str | None] = mapped_column(Text, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow)
    started_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)
    finished_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)


class MediaFile(Base):
    __tablename__ = "media_files"
    __table_args__ = (
        UniqueConstraint("chat_id", "telegram_message_id", name="uq_chat_message"),
    )

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    chat_id: Mapped[int] = mapped_column(ForeignKey("chats.id"), index=True)
    telegram_message_id: Mapped[int] = mapped_column(Integer)
    file_name: Mapped[str] = mapped_column(String(1024))
    mime_type: Mapped[str | None] = mapped_column(String(255), nullable=True)
    file_size: Mapped[int] = mapped_column(BigInteger, default=0)
    local_path: Mapped[str | None] = mapped_column(Text, nullable=True)
    remote_path: Mapped[str | None] = mapped_column(Text, nullable=True)
    status: Mapped[str] = mapped_column(String(32), default="pending", index=True)
    error: Mapped[str | None] = mapped_column(Text, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow)
    uploaded_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)
