# DreamReel

已支持公开的模型榜单（侧栏「模型榜单」，`/models`），按综合热度、作品数或获赞数查看。发布视频时可选填模型名称、生成参数、风格标签和原视频 ID；刷视频及查看作品时可展开查看 AIGC 信息。统计口径和接口说明见 [模型榜单与 AIGC 创作信息](docs/model_rankings.md)。

Go 后端位于 `apps/api`，React + Vite 前端位于 `apps/web`。前端迁自本机 `G:\GoCode\GCFeed\apps\web`，保留登录注册、视频流、点赞收藏、评论、关注、消息、个人主页和上传页面。

## 性能测试

实验室四轮 A/B 测试覆盖推荐分页、时间线与关注流、模型榜单、异步互动及客户端长刷，包含原始样本、执行计划和简历描述。[完整测试报告](docs/performance_lab_2026-11-13.md) · [复跑命令](benchmarks/lab/README.md)

## 服务器部署与一键启动

已提供项目根目录的 `start.ps1`（Windows 通过 SSH 操作 `target-wsl`）和 `start.sh`（目标 Linux 主机启动）。Windows 运行 `.\start.ps1` 启动已部署服务；运行 `.\start.ps1 -Action deploy` 同步当前源码、测试构建并重启。部署位置、首次配置和日志命令见 [部署说明](docs/deployment.md)。

已授权视频的下载、来源清单、初始社区数据导入和恢复下载方法见 [内容导入说明](docs/catalog-import.md)。

公网域名入口配置为 <http://www.dreamreel.me/>，当前等待备案审核，FRP 两端已停止并禁用自启，仅保留 <http://172.26.65.120/> 内网访问。普通启动和部署不会恢复已关闭的公网隧道；审核后恢复步骤见 [公网访问说明](docs/public-access.md)。

## 本地开发运行

需要 Go（版本见 `apps/api/go.mod`）、Node.js 22 或更新版本，以及 `apps/api/configs/config.yaml` 中配置的 MySQL、Redis、RabbitMQ 服务。MySQL 必须可用，目标数据库需要事先存在。后端会自动迁移表结构；Redis、RabbitMQ 连接失败时按现有逻辑降级。

在项目根目录打开 PowerShell，启动后端：

```powershell
cd G:\GoCode\DreamReel\apps\api
go run ./cmd/dreamreel -config ./configs/config.yaml
```

另开一个 PowerShell，启动前端：

```powershell
cd G:\GoCode\DreamReel\apps\web
npm ci
npm run dev
```

访问 <http://127.0.0.1:5173>。首次可直接浏览最新视频和热门榜单；注册或登录后使用推荐流、关注、互动、消息和上传。数据来自当前 DreamReel 数据库，不会迁入 GCFeed 的账号、数据库或上传文件。数据库没有作品时会显示空列表，可登录后上传视频和封面。

如果数据库中保留了 `cdn.example.com` 等占位视频地址，列表可以加载，但这些作品无法播放，需要使用真实的视频文件或可访问的媒体地址。

前端默认把 `/api`、`/uploads` 和 `/health` 代理到 `http://127.0.0.1:8080`，无需额外开启 CORS。可通过 <http://127.0.0.1:5173/health> 检查后端连接。如果后端使用其他地址，将 `apps/web/.env.example` 复制为同目录的 `.env.local`，修改 `BACKEND_PROXY_TARGET` 后重启前端。

上传视频需要后端所在环境的 `ffmpeg` 和 `ffprobe`。异步点赞、推荐画像、视频扇出等消费任务需要另开终端，在 `apps/api` 下执行 `go run ./cmd/worker -config ./configs/config.yaml`；详见后端现有配置。

本机 FFmpeg 与 ffprobe 已安装在 `G:\Tools\ffmpeg\bin`，并加入当前用户的 PATH。安装前已打开的终端或编辑器需要重新启动才能读取更新后的 PATH；也可在当前 PowerShell 中执行 `$env:Path = 'G:\Tools\ffmpeg\bin;' + $env:Path` 后再启动后端。

## 构建与预览

在 `apps/web` 下运行：

```powershell
npm run build
npm run preview
```

构建产物为 `apps/web/dist`，预览地址为 <http://127.0.0.1:4173>，仍会代理到真实后端。迁入的 Dockerfile 与 Nginx 配置保留原来的 `api:8080` 容器服务名约定；本地运行使用 Vite 即可。

前端已重构为浅色 AIGC 创作社区，提供点击暂停／继续、真实播放进度、声音／全屏控制、网格浏览、当前视频流搜索与模型筛选、作品分享、提示词复制和本地文字草稿。图标与默认图片均为本地资源。登录缓存使用 `dreamreel.*` 前缀，与 GCFeed 隔离。

功能保留清单、验证范围与结构说明见 [前端重构说明](docs/frontend_redesign.md)；原始前端源码归档和恢复方法见 [前端备份](backups/README.md)。
