# 授权视频与初始社区数据导入

本批次标识为 `bilibili-ai-20261108`，原作清单见 `data/bilibili-ai-20261108.json`。授权依据为操作者在聊天中确认已取得转载授权；工具没有独立核验授权文件，也不会绕过登录、付费或 DRM。清单仅保留公开可获取、源站播放量超过一万的作品。

视频下载到 `apps/api/uploads/catalog/video/`，封面从实际视频提取到 `apps/api/uploads/catalog/cover/`，头像为本地生成的原创 SVG。服务直接通过 `/uploads/catalog/` 提供媒体，前端使用原有 HTML5 播放器。下载格式优先采用公开可用的、不高于 720p 的 H.264/AAC；平台仅提供较低清晰度时保留可用版本，不虚构分辨率。

账号昵称没有“演示”前缀，使用独立的虚构人物、自然昵称及原创头像，不冒充来源作者。作品说明保留 B 站原作者和原作链接。初始点赞、收藏、评论、关注为模拟数据：每条互动均有对应账号及数据库记录，汇总计数与明细一致，时间在作品发布和本次导入之间分布。模拟账号不可登录，不存在公开或共享默认密码。

源站统计保存在来源清单和 `catalog_import_source.source_stats` 中，与站内模拟互动分开保存。内部表 `catalog_import_batch` 标注 `synthetic_engagement` 并记录总数、时间和清单 SHA-256；`catalog_import_account` 记录本批次创建的账号。后续真实用户产生的互动不会被写入导入批次。

## 在服务器重新运行

```bash
cd /home/accelerator/DreamReel

# 恢复下载会跳过已经完成的视频，并再次检查音轨、时长与文件可用性
python3 -u scripts/prepare_catalog.py download \
  --manifest data/bilibili-ai-20261108.json \
  --root "$PWD" --downloader .runtime/tools/yt-dlp
python3 scripts/catalog_avatars.py "$PWD"

cd apps/api
go build -o ../../.runtime/bin/catalog-import ./cmd/catalog-import

# 默认仅预检，不更改数据库
../../.runtime/bin/catalog-import \
  -root /home/accelerator/DreamReel \
  -manifest ../../.runtime/import/ready.json

# 首次导入会先做数据库快照，再在一个事务中提交数据
../../.runtime/bin/catalog-import \
  -root /home/accelerator/DreamReel \
  -config ../../.runtime/config.yaml \
  -manifest ../../.runtime/import/ready.json -apply
```

预检至少需要 30 部完整视频及封面；媒体缺失、来源不符或重复 BV 号会中止。数据库快照保存在 `.runtime/backups/`（权限 600），其中包含用户数据，应按私有备份保管。相同批次再次执行只输出已导入信息，不重复创建账号、视频或互动，不重置已有计数。导入器不覆盖原有账号或作品；账号名、BV 号冲突会使整个事务回滚。

初始热榜通过现有 Redis 热度接口预热，随后仍按网站原有时间窗口和真实互动更新。推荐所需的内容向量、关注流收件箱与互动明细一并创建。视频、来源清单、私有配置、数据库备份应一起保管。
