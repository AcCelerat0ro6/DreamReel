# 公网域名与本地直连

已配置的公网入口：<http://www.dreamreel.me/>。本地入口：<http://172.26.65.120/>。

当前状态（2026-11-08）：按要求等待备案审核，云端 `dreamreel-frps` 与 WSL 端 `dreamreel-frpc` 均已停止并禁用开机自启；仅保留内网访问。普通启动、重启或重新部署应用会保留这个关闭状态，不会自动恢复公网隧道。

关闭前已验证 FRP 链路及云端本机的页面、API、视频正常，但外部 HTTP 请求被阿里云返回 `403 Forbidden`、`Server: Beaver`，页面标题为 `Non-compliance ICP Filing`。需要处理域名的 ICP 备案或接入状态后再恢复公网。[阿里云说明](https://help.aliyun.com/zh/icp-filing/what-scenarios-need-to-be-filed-in-aliyun)；[已备案域名接入问题](https://help.aliyun.com/zh/icp-filing/basic-icp-service/support/access-for-the-record-the-faq/)。

```text
公网浏览器 → 47.120.45.106:80（dreamreel-frps）
                         ↓ TLS 隧道，控制端口 7000
目标 WSL（dreamreel-frpc）→ 127.0.0.1:80（Nginx）→ 127.0.0.1:8080（API）

本地浏览器 → 172.26.65.120:80（Nginx）→ 127.0.0.1:8080（API）
```

前端的 API 和视频资源地址均使用同源相对路径。本地访问 IP 时，页面、API 和视频直接从目标 WSL 获取；不会重定向到公共域名。公网访问时，相同路径通过 FRP 获取完整站点。

FRP 使用官方 v0.72.0，二进制已通过 SHA-256 校验。云端服务为 `dreamreel-frps.service`；WSL 端服务为 `dreamreel-frpc.service`。配置仍保留，当前两端服务已停止并禁用自启，云端不再由 FRP 监听 TCP 80、7000。EasyTier 的服务、11010/15888 端口和网络配置不变。FRP 仪表盘未启用。

## 启动与本地调试

在 Windows 项目根目录：

```powershell
.\start.ps1                       # 启动应用，保留当前公网关闭状态
.\start.ps1 -Action public-off    # 停止客户端并禁用自启，本地应用继续运行
.\start.ps1 -Action public-on     # 云端恢复后，启用并启动客户端
.\start.ps1 -Action public-status # 查看隧道状态
.\start.ps1 -Action status        # 查看应用和隧道状态
```

在目标 WSL 项目目录中，使用 `bash start.sh public-off` / `public-on` / `public-status` 执行相同操作。`stop` 会停止隧道、API 与 worker；共享 Nginx、数据库等继续运行。

备案审核通过并准备恢复公网时，先登录云服务器执行 `systemctl enable --now dreamreel-frps`，再运行 `public-on` 启动 WSL 客户端。

本地页面流量不经过 ECS。隧道开启时，FRP 自身仍有少量连接心跳；如果整个调试期间不需要公网访问，可用 `public-off` 暂停隧道。公网访问页面、API、上传与视频会经过 ECS，按云服务商流量规则计费。

## 配置与管理

云服务器：

```bash
systemctl status dreamreel-frps
journalctl -u dreamreel-frps -n 100
```

目标 WSL：

```bash
systemctl status dreamreel-frpc
journalctl -u dreamreel-frpc -n 100
```

二进制位于两端各自的 `/opt/dreamreel-frp/`，配置与专用共享密钥位于 `/etc/dreamreel-frp/`。密钥通过独立文件加载，不在仓库中保存，也未复用 root 密码。客户端固定校验云端的隧道证书。证书有效期一年；到期前需在云端更新证书并同步公开证书到 WSL，再重启两端 FRP 服务。

安装脚本为 `deploy/frp/install-server.sh`（ECS 上以 root 运行）与 `deploy/frp/install-client.sh`（WSL 中通过 sudo 运行）。服务器脚本会先检查 80/7000 是否被其他服务占用，遇到冲突会停止安装；不会关闭占用端口的服务、重启网络或更改防火墙。

该入口目前提供 HTTP；FRP 的 TLS 用于保护 WSL 与 ECS 间的隧道，不是浏览器到域名的 HTTPS。

## 验证与 DNS

```bash
curl -f http://www.dreamreel.me/health
curl -f http://172.26.65.120/health
```

DNS 的 A 记录应为 `www.dreamreel.me → 47.120.45.106`。若公共 DNS 已正确解析，而当前电脑的默认 DNS 仍报域名不存在，可在 Windows 管理员终端运行 `ipconfig /flushdns` 后重试；若上游 DNS 仍缓存旧记录，需等待缓存更新或使用能正确解析的 DNS。可用 `Resolve-DnsName www.dreamreel.me -Server 223.5.5.5` 对照。

WSL 中的客户端随目标发行版启动；Windows 启动本身不代表 WSL 已经运行。
