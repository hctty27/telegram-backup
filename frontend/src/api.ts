import type { AppSettings, BackupTask, Chat, DashboardStats } from "./types";

export function createApi(token: string) {
  async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const response = await fetch(`/api${path}`, {
      ...init,
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
        ...init.headers
      }
    });

    if (!response.ok) {
      const body = await response.json().catch(() => ({}));
      throw new Error(body.detail || `Request failed: ${response.status}`);
    }
    return response.json() as Promise<T>;
  }

  return {
    monitor: () => request<import("./OperationsPage").MonitorData>("/monitor"),
    archive: (params: URLSearchParams) => request<import("./OperationsPage").ArchiveData>("/archive?" + params.toString()),
    dashboard: () => request<DashboardStats>("/dashboard"),
    chats: () => request<Chat[]>("/chats"),
    tasks: () => request<BackupTask[]>("/tasks"),
    settings: () => request<AppSettings>("/settings"),
    syncChats: () => request<{ imported: number; scanned: number }>("/chats/sync", { method: "POST" }),
    updateChat: (id: number, backup_enabled: boolean) =>
      request<Chat>(`/chats/${id}`, {
        method: "PATCH",
        body: JSON.stringify({ backup_enabled })
      }),
    startBackup: (id: number) =>
      request<BackupTask>(`/chats/${id}/backup`, { method: "POST" }),
    retryTask: (id: number) =>
      request<BackupTask>(`/tasks/${id}/retry`, { method: "POST" }),
    saveTelegram: (payload: { api_id: number; api_hash?: string }) =>
      request<{ configured: boolean; authorized: boolean }>("/settings/telegram", {
        method: "PUT",
        body: JSON.stringify(payload)
      }),
    sendTelegramCode: (phone: string) =>
      request<{ sent: boolean }>("/settings/telegram/send-code", {
        method: "POST",
        body: JSON.stringify({ phone })
      }),
    verifyTelegramCode: (code: string) =>
      request<{ authorized: boolean; needs_password: boolean }>("/settings/telegram/verify-code", {
        method: "POST",
        body: JSON.stringify({ code })
      }),
    verifyTelegramPassword: (password: string) =>
      request<{ authorized: boolean }>("/settings/telegram/verify-password", {
        method: "POST",
        body: JSON.stringify({ password })
      }),
    addGoogleDrive: (payload: {
      name: string;
      client_id: string;
      client_secret: string;
      token_json: string;
      root_folder_id?: string;
    }) =>
      request<{ created: boolean; destination: string }>("/settings/storage/google-drive", {
        method: "POST",
        body: JSON.stringify(payload)
      }),
    testStorage: (destination?: string) =>
      request<{ success: boolean; message: string }>("/settings/storage/test", {
        method: "POST",
        body: JSON.stringify({ destination: destination || null })
      }),
    setStorageDestination: (destination: string) =>
      request<{ destination: string }>("/settings/storage/default", {
        method: "PUT",
        body: JSON.stringify({ destination })
      }),
    saveBackupSettings: (scan_interval_seconds: number) =>
      request<{ scan_interval_seconds: number }>("/settings/backup", {
        method: "PUT",
        body: JSON.stringify({ scan_interval_seconds })
      })
  };
}
