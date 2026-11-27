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
      request<BackupTask>(`/tasks/${id}/retry`, { method: "POST" })
  };
}
