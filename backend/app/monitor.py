"""Single-worker, in-memory telemetry. Never store raw subprocess logs."""
from collections import deque
from datetime import datetime, timezone
from time import monotonic


class TransferMonitor:
    def __init__(self):
        self.active = None
        self.events = deque(maxlen=200)
        self.started = 0.0

    def event(self, task_id, action, file_name=None):
        self.events.appendleft({"time": datetime.now(timezone.utc).isoformat(),
                                "task_id": task_id, "action": action, "file_name": file_name})

    def begin(self, task_id, file_name, phase, total=0):
        self.started = monotonic()
        self.active = {"task_id": task_id, "file_name": file_name, "phase": phase,
                       "bytes": 0, "total_bytes": max(0, total), "speed": 0}
        self.event(task_id, phase, file_name)

    def update(self, done, total, speed=None):
        if self.active is None:
            return
        done = max(0, int(done or 0))
        total = max(0, int(total or self.active["total_bytes"]))
        rate = done / max(monotonic() - self.started, 0.01) if speed is None else max(0, float(speed))
        self.active.update(bytes=done, total_bytes=total, speed=rate)

    def finish(self, task_id, action):
        self.event(task_id, action, self.active["file_name"] if self.active else None)
        self.active = None

    def snapshot(self):
        return {"active": dict(self.active) if self.active else None, "events": list(self.events)}


monitor = TransferMonitor()
