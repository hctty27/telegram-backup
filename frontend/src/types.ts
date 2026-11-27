export interface DashboardStats {
  total_chats: number;
  enabled_chats: number;
  uploaded_files: number;
  total_bytes: number;
  queued_tasks: number;
  running_tasks: number;
  failed_tasks: number;
}

export interface Chat {
  id: number;
  telegram_id: number;
  title: string;
  username: string | null;
  chat_type: string;
  backup_enabled: boolean;
  last_message_id: number;
  last_synced_at: string | null;
}

export interface BackupTask {
  id: number;
  chat_id: number;
  status: "queued" | "running" | "completed" | "failed" | string;
  scanned_messages: number;
  downloaded_files: number;
  uploaded_files: number;
  error: string | null;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
}

export interface AppSettings {
  telegram_configured: boolean;
  telegram_authorized: boolean;
  rclone_configured: boolean;
  rclone_remote: string;
  scan_interval_seconds: number;
}
