# DreamReel

DreamReel 是一个 AIGC 短视频平台。用户可以从不同模型和风格的作品中寻找灵感，创作者可以自由分享自己的作品、提示词和创作思路。模型榜单汇集站内公开作品及其互动数据，帮助用户发现社区正在使用的模型。

## 平台功能

- **发现灵感**。浏览最新作品、热门作品、个性化推荐和关注动态，在沉浸播放与网格浏览之间切换。支持在已加载的作品中搜索，并按模型筛选。
- **分享创作**。在创作工作室上传视频和封面，可附上模型名称、提示词或生成参数、风格标签和原作品 ID。支持保存本地文字草稿、复制提示词及分享作品链接。
- **查看模型榜单**。访问 `/models`，按综合热度、作品数或获赞数排序。榜单统计标注了模型名称的站内公开视频。[统计口径与 API](docs/model_rankings.md)
- **参与社区**。登录后可以点赞、收藏、评论、关注创作者和查看消息。公开主页集中展示创作者的作品。

## 技术架构

| 目录 | 说明 |
| --- | --- |
| `apps/web` | React 和 Vite 前端，包含短视频播放、创作工作室和模型榜单 |
| `apps/api` | Go 和 Gin API，以及独立的后台 Worker |
| `deploy` | Nginx 与 systemd 部署配置 |
| `benchmarks` | 推荐、视频流、模型榜单和异步互动的性能验证 |

MySQL 保存业务数据，Redis 缓存视频卡片并维护推荐会话队列，RabbitMQ 将互动处理、视频发布和兴趣画像更新等任务交给后台 Worker。推荐会话复用已排序的候选，翻页时批量校验作品可见性，候选不足时补充队列。

## 本地运行

需要 Go 1.26.2 或更新版本、Node.js 22 或更新版本，以及可访问的 MySQL、Redis 和 RabbitMQ。上传视频还需要将 `ffmpeg` 和 `ffprobe` 加入 API 进程的 `PATH`。

1. 在 `apps/api/configs/config.yaml` 中配置服务连接和 JWT 密钥，并在 MySQL 中创建配置指定的数据库。API 启动时会迁移表结构。
2. 在仓库根目录打开终端，启动 API。

   ```sh
   cd apps/api
   go run ./cmd/dreamreel -config ./configs/config.yaml
   ```

3. 在另一个终端从仓库根目录启动前端。

   ```sh
   cd apps/web
   npm ci
   npm run dev
   ```

4. 需要处理异步任务时，从仓库根目录另开终端启动 Worker，使它和 API 使用同一份配置。

   ```sh
   cd apps/api
   go run ./cmd/worker -config ./configs/config.yaml
   ```

打开 <http://127.0.0.1:5173> 浏览作品，或访问 <http://127.0.0.1:5173/health> 检查 API 连接。Vite 默认将 `/api`、`/uploads` 和 `/health` 转发到 `http://127.0.0.1:8080`。若 API 地址不同，将 `apps/web/.env.example` 复制为 `apps/web/.env.local`，设置 `BACKEND_PROXY_TARGET` 后重启前端。

使用空数据库启动时，可注册账号并发布首部作品。前端测试和构建分别在 `apps/web` 下执行 `npm test` 和 `npm run build`，构建产物位于 `apps/web/dist`。后端测试在 `apps/api` 下执行 `go test ./...`。

## 项目文档

- [服务器部署与启动](docs/deployment.md)
- [API 链路说明](docs/api_chains_guide.md)
- [模型榜单与 AIGC 创作信息](docs/model_rankings.md)
- [性能测试报告](docs/performance_lab_2026-11-13.md) · [实验复跑方法](benchmarks/lab/README.md)
