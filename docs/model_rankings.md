# 模型榜单与 AIGC 创作信息

前端侧栏的「模型榜单」位于 `/models`，无需登录即可查看，支持综合热度、作品数量、获赞数量排序、分页、刷新、空态和失败重试。

## 统计口径

- 只统计 `status=2`、存在发布时间且模型名称非空的公开视频；删除、下架和草稿作品不计入。
- 模型名称去除首尾空格，忽略大小写后分组；版本号属于模型名称的一部分。无互动统计记录的作品按零互动计算。
- 累计热度 = 点赞数 × 3 + 评论数 × 5 + 收藏数 × 4，与现有视频热度权重一致。
- 热度排序同分时依次比较作品数、模型名称；作品数排序同分时比较热度、模型名称；获赞排序同分时比较热度、作品数、模型名称。
- 数据来自数据库中的已落库统计，异步互动需等待 worker 消费完成。榜单表示本站创作和互动情况，不是模型能力评测。

## 榜单 API

`GET /api/model-rankings?sort=hot&limit=20&offset=0`

| 参数 | 含义 |
| --- | --- |
| `sort` | `hot`（默认）、`works`、`likes` |
| `limit` | 默认 20，正整数，最大 100，超过自动截断 |
| `offset` | 默认 0，取值 0～10000 |

```json
{
  "items": [
    {
      "rank": 1,
      "model_name": "Example Model v2",
      "video_count": 2,
      "like_count": 10,
      "comment_count": 3,
      "favorite_count": 2,
      "hot_score": 53
    }
  ],
  "sort": "hot",
  "limit": 20,
  "offset": 0,
  "has_more": false
}
```

`rank` 是当前排序下从 1 开始的名次。空榜返回 `items: []`。非法排序或分页参数返回 400，查询失败返回 500。榜单实时查询，各页之间可能随新发布和互动变动。

## 发布与展示

发布页增加以下选填字段，沿用 `POST /api/videos` 的现有字段名称：

| 字段 | 类型 | 规则 |
| --- | --- | --- |
| `model_name` | 字符串 | 模型名称及版本，最多 64 UTF-8 字节 |
| `model_params` | 字符串 | 生成参数或提示词，最多 1024 UTF-8 字节 |
| `ai_style_tag` | 字符串 | 风格标签，最多 32 UTF-8 字节 |
| `origin_video_id` | 整数 | 原视频 ID，填写时必须为正整数；作为来源元数据保存，不验证关联作品是否存在 |

这些字段相互独立，均可省略；空白字符串归一化为空值。前端在上传文件前校验字节长度和 ID，未填写的字段不提交。只有填写模型名称的作品才计入模型榜单。

视频详情、作者作品、最新流、推荐流、关注流和热门流均传递完整 AIGC 信息。前端在有任一字段时展示 AIGC 标签，可展开查看详情；普通视频不显示占位标签。发布预览及个人作品播放器也支持展示。视频卡片 Redis key 升级为 `video:card:v2:*`，避免旧缓存暂时缺少新增展示字段；API 和 worker 应一起重启。

## 验证

在 `apps/api` 下运行 `go test ./...`。可选 MySQL 集成测试会读取现有本地配置，并创建、自动清理独立的 `dreamreel_model_test_*` 测试库：

```powershell
$env:DREAMREEL_MODEL_LIVE = '1'
go test ./test -run TestModelRankingsLive -count=1 -v
```

在 `apps/web` 下运行 `npm test` 和 `npm run build`。
