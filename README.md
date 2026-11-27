# TelegramBackup

Go 自托管 Telegram 媒体备份服务。使用 gotd、PostgreSQL、River、React 19、Tailwind CSS 4 和 rclone。支持用户账号扫码授权、历史及增量扫描、实时消息监听、持久化差量恢复、断点续传、磁盘配额、Google Drive 加密配置及上传验证后清理。

自托管服务仅备份账号有权访问的会话。首次部署请使用独立数据库和授权，实际 Telegram / Google Drive 联调仍需执行。

## 部署

Linux Docker Engine + Docker Compose v2。单应用实例，数据库不对外暴露。

```sh
umask 077
(set -C; printf 'TGB_DB_PASSWORD=%s\nTGB_ADMIN_TOKEN=%s\nTGB_ENCRYPTION_KEY=%s\n' "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" > .env)
docker compose pull
docker compose up -d --no-build
docker compose ps
```

密钥初始化命令仅适用于首次安装；若 .env 已存在，不要覆盖，应保留原密钥。默认宿主机监听 0.0.0.0:8080，可通过服务器 IP 访问。通过 .env 中的 `TGB_BIND_ADDR` 和 `TGB_PORT` 自定义监听地址及宿主机端口；容器内部端口为 8080。公网访问请限制防火墙来源并使用可信 HTTPS 反向代理，避免明文传输管理令牌；仅代理访问时可将 TGB_BIND_ADDR 设为 127.0.0.1。使用 .env 中的管理令牌登录。保留并安全备份数据库、加密密钥和应用卷；不要运行 `docker compose down -v` 清理生产环境。

1. 添加 Telegram API ID / Hash，扫码并完成可选的 2FA。
2. 配置 Google Drive。支持桌面 OAuth Token 导入；直接网页授权需自己的 Web OAuth Client、Drive API 和精确的 HTTPS 回调 `TGB_GOOGLE_REDIRECT_URI`。
3. 刷新群组，选择目标云存储，启用策略。首次扫描历史，之后实时监听及增量补偿。
4. 先用专门测试群验证文件内容、重启恢复和上传校验后本地清理，再扩大备份范围。

默认下载缓存 10 GiB，最小剩余磁盘空间 5 GiB；在 .env 中设置 TGB_CACHE_GIB / TGB_MIN_FREE_GIB 后重建应用容器生效。实际磁盘可能被其他服务消耗。当前同账号 RPC 串行，同云存储操作串行；不是无限并发系统。超出缓存额度或大小未知的媒体会标记受限。

## 更新与源码构建

默认使用 `ghcr.io/hctty27/telegram-backup:latest`，无需在服务器编译。先确认 [Actions](https://github.com/hctty27/telegram-backup/actions) 发布任务通过。首次创建的 GHCR 包可能是私有的，仓库管理员需要在 Package settings 将其设为 Public 才能匿名拉取；私有部署可使用具备 read:packages 的凭据执行 docker login ghcr.io。不要提交令牌。

已有部署更新（保留 .env 与数据卷）：

```sh
git pull --ff-only origin main
docker compose pull
docker compose up -d --no-build
docker compose ps
```

CI 对 linux/amd64 和 linux/arm64 分别构建并验证实际镜像，安全扫描通过后才发布。main 推送更新 latest，稳定版本标签推送发布对应版本，均带完整 sha 标签。可设置 TGB_IMAGE 为指定标签或 digest 以固定版本。发布失败时不应假定最新镜像已可用。

开发者需要从源码构建时：

```sh
docker compose -f docker-compose.yml -f docker-compose.build.yml build app
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --no-build --pull never
```

上述源码启动要求 PostgreSQL 基础镜像已在本机（首次先执行 `docker compose pull postgres`）。BuildKit 缓存 npm、Go modules 和 Go 编译产物。

## 开发及验收

- [架构](docs/architecture.md)
- [安装、限制和验证状态](docs/operations.md)
- [Telegram 独立联调工具](docs/telegram-probe.md)

Go 1.27.4 / Node 22；CI 执行前端构建、Go build/vet/race、数据库迁移、模拟备份集成、Docker 和全新 Compose 启动检查。依赖锁文件已入库；完整自动化测试、安全扫描和容器部署检查由 CI 执行。实际服务验收需使用授权测试账号。

```sh
(cd web && npm ci && npm run build)
go mod tidy -diff
go mod verify
go build ./...
go vet ./...
go test -race ./...
```

### 更新依赖

更新 Go 依赖后执行 `go mod tidy`，更新前端依赖后在 `web` 中执行 `npm install --package-lock-only --ignore-scripts`。将依赖声明与对应锁文件放入同一个 PR，复核后再运行完整 CI。依赖锁生成工作流仅提供只读构建产物，不再自动写入分支。不要为了通过测试关闭漏洞扫描或手工伪造校验值。
