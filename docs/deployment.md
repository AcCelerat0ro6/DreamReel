# 部署到 target-wsl

部署目录为 `/home/accelerator/DreamReel`。Nginx 在 80 端口提供前端，并将 `/api/`、`/uploads/`、`/health` 转发到 8080 端口的 Go API。`dreamreel-api` 和 `dreamreel-worker` 由 systemd 管理，异常退出后自动重启，随目标 Linux 系统启动。首次部署使用空数据库，不迁移本机账号、视频或上传文件。

## 在 Windows 项目根目录操作

生产配置将 API 的 `host` 设为 `127.0.0.1`，固定使用 IPv4 回环地址与 Nginx 通信。源代码中的默认监听行为保持兼容，本地开发可继续使用原配置。

```powershell
# 一键启动已部署的服务（默认连接 target-wsl）
.\start.ps1

# 将当前工作目录中的最新源码同步到目标主机，测试、构建并重启
.\start.ps1 -Action deploy

.\start.ps1 -Action status
.\start.ps1 -Action restart
.\start.ps1 -Action stop
.\start.ps1 -Action logs
```

脚本通过 SSH 终端提示输入 sudo 密码，不保存密码。`deploy` 包含尚未提交的源码，排除 `.git`、本地依赖、构建产物、备份、日志、上传文件、`.env*` 和 `config.yaml`；目标主机的 `.runtime` 配置与上传文件保留。`start` 仅启动服务，代码更新使用 `deploy`。首次连接新目标、尚未找到启动脚本时，`start` 自动进入 `deploy`。

可用 `-Target` 和 `-RemotePath` 指定其他 SSH 别名与绝对路径。服务器路径不支持空格。

## 在目标主机操作

```bash
cd /home/accelerator/DreamReel
bash start.sh                 # 启动
bash start.sh deploy          # 测试、构建、更新服务
bash start.sh status
bash start.sh restart
bash start.sh stop
bash start.sh logs
```

停止操作只停止本项目 API 和 worker，保留共享的 MySQL、Redis、RabbitMQ 和 Nginx。

## 首次配置新主机

需要 Ubuntu/systemd、Go（不低于 `apps/api/go.mod` 指定版本）、Node.js 22 或更新版本、npm、Nginx、FFmpeg/ffprobe、Python 3/PyYAML、curl、flock，以及已启动的 MySQL、Redis 和 RabbitMQ。

```bash
sudo apt-get update
sudo apt-get install -y golang-go nodejs npm nginx ffmpeg python3-yaml curl util-linux
mkdir -p .runtime
chmod 750 .runtime
```

首次启动前，在目标主机的 `.runtime/bootstrap.yaml` 放置与 `apps/api/configs/config.yaml` 相同结构的配置，填写**目标主机**的 MySQL 管理员账号、Redis 和 RabbitMQ 连接信息，并执行 `chmod 600 .runtime/bootstrap.yaml`。MySQL、Redis、RabbitMQ 应运行在该目标主机上。配置不会由 `start.ps1` 自动上传。

`bash start.sh deploy` 会在 MySQL 中创建配置指定的数据库（若不存在）及仅有该库权限的 `dreamreel_app@127.0.0.1`，生成新的 JWT 密钥和内部 token，并将最终配置保存为 `.runtime/config.yaml`（权限 600）。创建成功后删除 bootstrap 文件。若数据库已存在则保留内容；若已存在同名应用账号却缺少最终配置，脚本会报错，需管理员核对账号，避免覆盖未知凭据。

后续部署保留 `.runtime/config.yaml`，不会重置数据库、密码或登录密钥。Redis 使用逻辑库 1；RabbitMQ 采用 bootstrap 配置中的连接与 `dreamreel.*` 队列。上传文件保存在 `apps/api/uploads/`，应与数据库、`.runtime/config.yaml` 一起备份。

## 日志与检查

```bash
curl -f http://127.0.0.1/health
systemctl status dreamreel-api dreamreel-worker nginx
journalctl -u dreamreel-api -u dreamreel-worker -n 100
```

应用文件日志位于 `.runtime/logs/`；前端静态文件位于 `/var/www/dreamreel`；Nginx 配置位于 `/etc/nginx/sites-available/dreamreel`。部署会禁用发行版默认欢迎页的软链接，保留原配置文件。

WSL 内的服务随该发行版启动；Windows 开机后若 WSL 尚未运行，需要先启动目标 WSL。访问地址使用目标主机当前 IP，WSL 重启后 IP 可能变化。此部署使用 HTTP，域名和 HTTPS 可在有域名后单独配置。

公网域名已配置独立的 FRP 隧道，当前等待 ICP 备案审核，FRP 两端已停止并禁用自启，仅保留内网访问。当前状态、恢复公网步骤与本地直连说明见 [公网访问说明](public-access.md)。
