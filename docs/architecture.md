# TelegramBackup 架构

单进程模块化 Go 应用，内嵌 React 管理后台。PostgreSQL 持久化业务数据和 River 队列，gotd 管理 Telegram 客户端，rclone 提供 Google Drive 存储适配。生产环境无需 Python、Node.js 运行时或额外消息中间件。

## 模块

- cmd/server：应用组合、配置、健康检查及生命周期。
- cmd/migrate：Goose 和 River 数据库迁移。
- internal/telegram：账号授权、加密会话、历史分页、常驻连接和差量状态。
- internal/backup 与 internal/queue：历史与实时消息统一入库、幂等任务、下载、上传和清理。
- internal/disk 与 internal/transfer：磁盘配额、容量预留、持久化断点、原子文件发布。
- internal/storage：加密 Google Drive 配置、rclone 上传前冲突检查、上传后校验。
- internal/api 与 web：Bearer 管理认证、REST、SSE 监控和内嵌 SPA。

## 一致性与安全

历史游标只在消息事务提交成功后推进。实时消息复用同一去重逻辑，不推进历史游标；常驻连接及周期扫描补偿遗漏。超出 Telegram 差量恢复窗口或已删除的消息不保证能够恢复。

队列与状态转换使用 PostgreSQL 事务。下载前预留容量，写入临时文件，验证长度并原子发布；重启后按消息身份及摘要恢复断点。已发布文件不因缓存压力主动删除。仅在所有目标副本完整性验证记录提交后清理本地文件。失败保留源文件，重试前检查同名远端文件，拒绝不一致、歧义或目录冲突。远端列表与上传不是云端原子事务，应使用独占备份目录，避免外部并发写入。

加密使用 AES-256-GCM 与记录绑定的认证附加数据。密钥由环境变量提供。加密格式标识保持稳定；数据库迁移编号、第三方 API 版本及密码学格式版本不属于产品命名。非 root 容器运行；数据库不公开端口，管理接口通过可信 HTTPS 或 SSH 隧道访问。

## 部署与容量

单应用实例，默认下载缓存 10 GiB，最低剩余空间 5 GiB。队列并发受配置限制，同 Telegram 账号 RPC 串行，同云盘配置操作串行。禁止多个实例同时访问同一 staging 目录或数据库任务。使用独立数据库及专用云盘文件夹。

详见 [运维与验收](operations.md)。
