import asyncio
from pathlib import Path

from app.config import settings


class RcloneStorage:
    async def upload(self, source: Path, destination: str) -> None:
        if not source.is_file():
            raise FileNotFoundError(f"Local media file does not exist: {source}")
        if not settings.rclone_config.is_file():
            raise RuntimeError(
                f"rclone config not found: {settings.rclone_config}. "
                "Create ./rclone/rclone.conf and configure a remote first."
            )

        command = [
            "rclone",
            "copyto",
            str(source),
            destination,
            "--config",
            str(settings.rclone_config),
            "--retries",
            "3",
            "--retries-sleep",
            "10s",
            "--low-level-retries",
            "10",
        ]
        process = await asyncio.create_subprocess_exec(
            *command,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        stdout, stderr = await process.communicate()
        if process.returncode != 0:
            detail = (stderr or stdout).decode("utf-8", errors="replace").strip()
            raise RuntimeError(f"rclone upload failed: {detail[-2000:]}")
