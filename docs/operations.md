# 运维与验收

## 配置

首次安装按 [README](../README.md) 生成独立密钥。不要覆盖已存在的 .env 或删除生产数据卷。默认宿主机地址 127.0.0.1:8080，单应用实例。

- TGB_PORT：宿主机监听端口（默认 8080，仅回环地址）。
- TGB_DB_PASSWORD：数据库密码（推荐随机十六进制）。
- TGB_ADMIN_TOKEN：管理令牌。
- TGB_ENCRYPTION_KEY：32 字节随机密钥的十六进制表示，务必安全备份。
- TGB_ENABLE_WORKERS：是否启动备份任务。
- TGB_CACHE_GIB / TGB_MIN_FREE_GIB：缓存上限与磁盘保留空间。
- TGB_GOOGLE_REDIRECT_URI：可选 Google Web OAuth 完整回调 URL。

应用默认数据目录 /var/lib/telegram-backup，Compose 项目 telegram-backup，持久卷 postgres_data、app_data，数据库 telegram_backup。管理接口统一为 /api/，健康检查 /api/health、/api/ready。

## Google Drive

创建专用云盘目录并启用 Drive API。可通过可信桌面执行 rclone authorize drive 后导入 OAuth Token JSON（需包含 refresh_token），或配置 Web OAuth client，回调填写 https://YOUR-HOST/api/oauth/google/callback，设置 TGB_GOOGLE_REDIRECT_URI 后重建应用。仪表盘与回调必须使用同一 origin。反向代理不得记录授权回调查询字符串。不要公开密钥、会话、授权令牌或 QR。

服务端加密存储 OAuth 配置，使用私有 tmpfs 配置执行 rclone，并持久化刷新后的令牌。注册时检查列表访问，不保证写权限；必须实际上传测试。使用专属归档目录，避免其它程序并发修改同名对象。

## 验收

自动化验证覆盖前端构建、Go 编译/Vet/Race、依赖安全扫描、数据库迁移、模拟备份、真实本地 rclone 防覆盖回归及 Docker Compose 空库启动。参见 [Actions](https://github.com/hctty27/telegram-backup/actions)。

真实服务联调由有权限的操作员完成：独立测试账号及群组，完成 QR/2FA，配置专用 Drive 文件夹，测试历史扫描、新图片/视频、断线恢复、下载中断重启、文件内容校验、本地自动清理、磁盘压力暂停和失败任务重试。真实 Telegram/Drive 联调尚未完成。

未知大小或超过缓存上限的文件标为 blocked。缓存不足时暂停下载并继续上传和清理；非活跃未验证部分文件可能被回收。未支持的媒体格式、消息编辑和超出差量窗口后已删除的消息无法保证备份。前端分页、筛选、配置热更新仍可继续改善。

## 数据保护

重命名 Compose 项目和卷名会使用不同的容器/卷命名空间；不会自动迁移旧部署数据。若已有部署，先停止旧服务、备份数据库及应用数据并保留原密钥，再单独制定迁移方案；不要运行 docker compose down -v 删除生产数据。
