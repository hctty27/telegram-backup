import asyncio
import configparser
import json
import os
import re
import tempfile
from contextlib import contextmanager
from pathlib import Path
from typing import Iterator

from app.config import settings
from app.config_store import get_config_value, set_config_value
from app.database import SessionLocal


_REMOTE_NAME = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$")


class RcloneStorage:
    async def _read_config_text(self) -> str:
        async with SessionLocal() as db:
            stored = await get_config_value(db, "rclone_config_ini")
        if stored:
            return stored
        if settings.rclone_config.is_file():
            return settings.rclone_config.read_text(encoding="utf-8")
        return ""

    async def _persist_config_text(self, content: str) -> None:
        from app.config_store import encryption_ready
        if not encryption_ready():
            return
        async with SessionLocal() as db:
            current = await get_config_value(db, "rclone_config_ini")
            if current != content:
                await set_config_value(db, "rclone_config_ini", content, secret=True)
                await db.commit()

    async def import_legacy_config(self, db) -> bool:
        existing = await get_config_value(db, "rclone_config_ini")
        if existing:
            return True
        if not settings.rclone_config.is_file():
            return False
        # Keep the old file as a fallback when encryption has not been configured yet.
        from app.config_store import encryption_ready
        if not encryption_ready():
            return False

        content = settings.rclone_config.read_text(encoding="utf-8")
        await set_config_value(db, "rclone_config_ini", content, secret=True)
        parser = configparser.ConfigParser(interpolation=None)
        parser.read_string(content)
        remote_names = parser.sections()
        current = await get_config_value(db, "storage_destination")
        if not current and remote_names:
            legacy = settings.rclone_remote
            legacy_name = legacy.split(":", 1)[0]
            # root_folder_id configures the remote root itself; do not append it again to the destination.
            destination = f"{name}:"
            await set_config_value(db, "storage_destination", destination)
        await db.commit()
        return True

    async def list_remotes(self) -> list[str]:
        content = await self._read_config_text()
        if not content.strip():
            return []
        parser = configparser.ConfigParser(interpolation=None)
        try:
            parser.read_string(content)
        except configparser.Error:
            return []
        return parser.sections()

    async def get_destination(self) -> str:
        async with SessionLocal() as db:
            destination = await get_config_value(db, "storage_destination")
        return destination or settings.rclone_remote

    @contextmanager
    def _temporary_config(self, content: str) -> Iterator[Path]:
        settings.data_dir.mkdir(parents=True, exist_ok=True)
        handle = tempfile.NamedTemporaryFile(
            mode="w",
            encoding="utf-8",
            suffix=".conf",
            prefix="rclone-",
            dir=settings.data_dir,
            delete=False,
        )
        path = Path(handle.name)
        try:
            os.chmod(path, 0o600)
            with handle:
                handle.write(content)
            yield path
        finally:
            path.unlink(missing_ok=True)

    async def _rclone_environment(self, extra: dict[str, str] | None = None) -> dict[str, str]:
        environment = os.environ.copy()
        async with SessionLocal() as db:
            raw = await get_config_value(db, "rclone_remote_secrets", "{}")
        try:
            remote_secrets = json.loads(raw or "{}")
        except json.JSONDecodeError:
            remote_secrets = {}
        for remote_name, credentials in remote_secrets.items():
            env_name = remote_name.upper().replace("-", "_").replace(".", "_")
            if credentials.get("client_id"):
                environment[f"RCLONE_CONFIG_{env_name}_CLIENT_ID"] = credentials["client_id"]
            if credentials.get("client_secret"):
                environment[f"RCLONE_CONFIG_{env_name}_CLIENT_SECRET"] = credentials["client_secret"]
        if extra:
            environment.update(extra)
        return environment

    async def _run_rclone(
        self,
        args: list[str],
        *,
        timeout: int = 60,
        extra_env: dict[str, str] | None = None,
    ) -> tuple[int, str]:
        process = await asyncio.create_subprocess_exec(
            "rclone",
            *args,
            env=await self._rclone_environment(extra_env),
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        try:
            stdout, stderr = await asyncio.wait_for(process.communicate(), timeout=timeout)
        except TimeoutError:
            process.kill()
            await process.wait()
            raise RuntimeError("rclone operation timed out.")
        output = (stderr or stdout).decode("utf-8", errors="replace").strip()
        return process.returncode or 0, output

    async def create_google_drive_remote(
        self,
        *,
        name: str,
        client_id: str,
        client_secret: str,
        token_json: str,
        root_folder_id: str = "",
    ) -> str:
        from app.config_store import encryption_ready
        if not encryption_ready():
            raise RuntimeError("Set APP_ENCRYPTION_KEY before saving cloud credentials.")
        if not _REMOTE_NAME.fullmatch(name):
            raise ValueError("Remote name may contain letters, numbers, dot, underscore and hyphen.")
        remotes = await self.list_remotes()
        if name in remotes:
            raise ValueError(f"Remote '{name}' already exists.")

        if not client_id.strip() or not client_secret.strip():
            raise ValueError("Google OAuth Client ID and Client Secret are required.")
        try:
            token = json.loads(token_json)
        except json.JSONDecodeError as exc:
            raise ValueError("Token JSON is invalid.") from exc
        if not isinstance(token, dict) or not token.get("refresh_token"):
            raise ValueError("Token JSON must contain a refresh_token. Paste the full rclone authorize output.")

        config_text = await self._read_config_text()
        parser = configparser.ConfigParser(interpolation=None)
        if config_text.strip():
            try:
                parser.read_string(config_text)
            except configparser.Error as exc:
                raise RuntimeError("The current rclone configuration could not be parsed.") from exc
        parser[name] = {
            "type": "drive",
            "token": json.dumps(token, separators=(",", ":")),
            "scope": "drive",
        }
        if root_folder_id.strip():
            parser[name]["root_folder_id"] = root_folder_id.strip()
        from io import StringIO
        buffer = StringIO()
        parser.write(buffer)
        saved_config = buffer.getvalue()

        credentials_env = {
            f"RCLONE_CONFIG_{name.upper().replace('-', '_').replace('.', '_')}_CLIENT_ID": client_id.strip(),
            f"RCLONE_CONFIG_{name.upper().replace('-', '_').replace('.', '_')}_CLIENT_SECRET": client_secret.strip(),
        }
        with self._temporary_config(saved_config) as config_path:
            code, _ = await self._run_rclone(
                ["--config", str(config_path), "lsd", f"{name}:", "--max-depth", "1"],
                timeout=90,
                extra_env=credentials_env,
            )
            if code != 0:
                raise RuntimeError("Google Drive connection test failed. Check the OAuth credentials, token JSON and Drive permissions.")
            saved_config = config_path.read_text(encoding="utf-8")

        async with SessionLocal() as db:
            await set_config_value(db, "rclone_config_ini", saved_config, secret=True)
            raw_secrets = await get_config_value(db, "rclone_remote_secrets", "{}")
            try:
                secrets_map = json.loads(raw_secrets or "{}")
            except json.JSONDecodeError:
                secrets_map = {}
            secrets_map[name] = {
                "client_id": client_id.strip(),
                "client_secret": client_secret.strip(),
            }
            await set_config_value(
                db,
                "rclone_remote_secrets",
                json.dumps(secrets_map, separators=(",", ":")),
                secret=True,
            )
            destination = f"{name}:"
            await set_config_value(db, "storage_destination", destination)
            await db.commit()
        return destination

    async def set_destination(self, destination: str) -> str:
        destination = destination.strip()
        if ":" not in destination:
            raise ValueError("Destination must use the format remote:path.")
        remote_name, _ = destination.split(":", 1)
        if remote_name not in await self.list_remotes():
            raise ValueError(f"Remote '{remote_name}' does not exist.")
        if "\n" in destination or "\r" in destination:
            raise ValueError("Invalid destination.")
        async with SessionLocal() as db:
            await set_config_value(db, "storage_destination", destination)
            await db.commit()
        return destination

    async def test_destination(self, destination: str | None = None) -> None:
        config_text = await self._read_config_text()
        if not config_text.strip():
            raise RuntimeError("No rclone remote is configured.")
        target = (destination or await self.get_destination()).strip()
        if ":" not in target:
            raise ValueError("Destination must use the format remote:path.")
        remote_name = target.split(":", 1)[0]
        if remote_name not in await self.list_remotes():
            raise ValueError(f"Remote '{remote_name}' does not exist.")
        with self._temporary_config(config_text) as config_path:
            code, _ = await self._run_rclone(
                ["--config", str(config_path), "lsd", target, "--max-depth", "1"],
                timeout=90,
            )
            updated_config = config_path.read_text(encoding="utf-8")
        if code != 0:
            raise RuntimeError("Cloud storage connection test failed. Check the remote authorization and destination path.")
        await self._persist_config_text(updated_config)

    async def upload(self, source: Path, destination: str) -> None:
        if not source.is_file():
            raise FileNotFoundError(f"Local media file does not exist: {source}")
        config_text = await self._read_config_text()
        if not config_text.strip():
            raise RuntimeError("No rclone configuration found. Add a cloud storage remote in Settings.")
        with self._temporary_config(config_text) as config_path:
            code, detail = await self._run_rclone(
                [
                    "--config",
                    str(config_path),
                    "copyto",
                    str(source),
                    destination,
                    "--retries",
                    "3",
                    "--retries-sleep",
                    "10s",
                    "--low-level-retries",
                    "10",
                ],
                timeout=3600,
            )
            updated_config = config_path.read_text(encoding="utf-8")
        if code != 0:
            raise RuntimeError(f"rclone upload failed: {detail[-2000:]}")
        await self._persist_config_text(updated_config)
