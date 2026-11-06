# DreamReel

已支持公开的模型榜单（侧栏「模型榜单」，`/models`），按综合热度、作品数或获赞数查看。发布视频时可选填模型名称、生成参数、风格标签和原视频 ID；刷视频及查看作品时可展开查看 AIGC 信息。统计口径和接口说明见 [模型榜单与 AIGC 创作信息](docs/model_rankings.md)。

Go 后端位于 `apps/api`，React + Vite 前端位于 `apps/web`。前端迁自本机 `G:\GoCode\GCFeed\apps\web`，保留登录注册、视频流、点赞收藏、评论、关注、消息、个人主页和上传页面。

## 本地运行

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

前端保留了源项目的 Google Fonts、图标字体及默认示例图片，这些外部资源需要网络访问。登录缓存使用 `dreamreel.*` 前缀，与 GCFeed 隔离。
