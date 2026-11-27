# TelegramBackup V2

Go 自托管 Telegram 媒体备份服务。使用 gotd、PostgreSQL、River、React 19、Tailwind CSS 4 和 rclone。支持用户账号扫码授权、历史及增量扫描、实时消息监听、持久化差量恢复、断点续传、磁盘配额、Google Drive 加密配置及上传验证后清理。

**当前为发布候选开发分支，真实 Telegram / Google Drive 联调及最终 CI 尚未完成。不是正式发布版本。** 仅备份账号有权访问的会话。全新安装，不支持旧 Python 版本迁移。

## 部署（待 CI 通过后测试）

Linux Docker Engine + Docker Compose v2。单应用实例，数据库不对外暴露。

```sh
umask 077
printf 'V2_DB_PASSWORD=%s\nV2_ADMIN_TOKEN=%s\nV2_ENCRYPTION_KEY=%s\n' "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" > .env
docker compose up -d --build
docker compose ps
```

默认宿主机监听 127.0.0.1:8081。通过 SSH 隧道或可信 HTTPS 反向代理访问；不要直接公开 HTTP。使用 .env 中的管理令牌登录。保留并安全备份数据库、加密密钥和应用卷；不要运行 `docker compose down -v` 清理生产环境。

1. 添加 Telegram API ID / Hash，扫码并完成可选的 2FA。
2. 配置 Google Drive。支持桌面 OAuth Token 导入；直接网页授权需自己的 Web OAuth Client、Drive API 和精确的 HTTPS 回调 `V2_GOOGLE_REDIRECT_URI`。
3. 刷新群组，选择目标云存储，启用策略。首次扫描历史，之后实时监听及增量补偿。
4. 先用专门测试群验证文件内容、重启恢复和上传校验后本地清理，再扩大备份范围。

默认下载缓存 10 GiB，最小剩余磁盘空间 5 GiB；在 .env 中设置 V2_CACHE_GIB / V2_MIN_FREE_GIB 后重建应用容器生效。实际磁盘可能被其他服务消耗。当前同账号 RPC 串行，同云存储操作串行；不是无限并发系统。超出缓存额度或大小未知的媒体会标记受限。

## 开发及验收

- [V2 架构](docs/V2-ARCHITECTURE.md)
- [安装、限制和验证状态](docs/V2-STATUS.md)
- [Telegram 独立联调工具](docs/V2-PROBE.md)

Go 1.27.4 / Node 22；CI 执行前端构建、Go build/vet/race、数据库迁移、模拟备份集成、Docker 和全新 Compose 启动检查。依赖锁文件已入库；安全修复版本正在重新生成锁文件并等待 CI，最终真实服务验收尚未完成。

```sh
(cd web && npm ci && npm run build)
go mod tidy -diff
go mod verify
go build ./...
go vet ./...
go test -race ./...
```
