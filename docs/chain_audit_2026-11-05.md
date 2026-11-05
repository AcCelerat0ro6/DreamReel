# DreamReel 逐链路检查报告

日期：2026-11-05（香港时间）  
范围：当前工作区代码，包含检查开始前已有的 6 个未提交修改文件；未修改业务实现。

## 结论与证据

已连接本机 WSL 的 MySQL、Redis、RabbitMQ，使用真实生产路由 `router.Register`、真实 GORM 仓储、真实 Redis 缓存与 RabbitMQ 消费者进行隔离集成测试。

- `go test ./...` 与 `go vet ./...` 均通过（默认跳过显式开启的真实审计）；原有 6 组内存仓储链路测试通过，正常流程的真实集成测试通过。
- 新增 14 个集成子用例：2 个正常流程用例通过，12 个异常行为用例失败，分别对应下文 12 项发现。失败是审计发现的证据，不代表正常构建失败。
- 原始执行日志：[chain_audit_2026-11-05_results.txt](./chain_audit_2026-11-05_results.txt)。
- 可复现测试：[chain_audit_test.go](../apps/api/test/chain_audit_test.go)。默认跳过；必须显式设置环境变量才连接基础设施。
- 测试新建并清理 `dreamreel_audit_<随机后缀>` 数据库、原先为空的 Redis DB 15 中的测试键、`dreamreel.audit.<随机后缀>` RabbitMQ 队列与交换机，没有复用业务数据库或业务队列。

## 各链路检查结果

| 链路 | 检查范围 | 结果 |
|---|---|---|
| 启动与依赖装配 | API/Worker 入口、配置、迁移、生产路由、三项基础设施 | 生产路由与迁移在独立库成功；未启动常驻 API/Worker 进程 |
| 注册、登录、资料 | 注册→bcrypt→MySQL→登录→JWT→读取/更新资料；原有测试中的公开资料脱敏与参数校验 | 正常流程通过；封禁旧令牌、退出令牌见 F03/F12 |
| 发布、详情、作品列表 | 发布→video/video_stat 事务→事件；详情；原有作者/本人列表测试 | 正常流程通过；事件失败见 F06，删除一致性见 F07 |
| 文件上传 | multipart→文件分类/扩展名/MIME→本地保存→静态访问 | HTML 风险见 F02；真实视频处理未验证，见边界说明 |
| Timeline Feed | SQL 轻量页→页/卡片/统计缓存→用户互动状态→返回 | 正常请求通过；删除后的缓存见 F07 |
| Following Feed | 关注回填→发布事件→Inbox→关注流 | 正常分发已验证；取关残留及故障回退见 F08/F10 |
| Hot Feed | 分钟桶、滑动窗口、MySQL 热度查询分支 | 正常接口返回通过；Redis 故障回退见 F10；未做大规模排序验证 |
| 推荐 Feed | 候选召回→向量/画像→排序→分页→卡片组装 | 正常请求通过；深分页见 F11 |
| 点赞、收藏 | HTTP→Redis 状态/分片计数→RabbitMQ→MySQL | 正常异步落库通过；冷缓存、乱序见 F04/F05 |
| 评论 | 创建/读取→MySQL 评论计数→通知→Redis 统计 | 正常流程通过；统计缓存过期见 F09；删除权限做了代码审查，未做完整真实库权限矩阵 |
| 关注/粉丝 | 关注/取关事务、计数、通知、列表分页 | 正常关注/取关通过；列表原有内存测试通过；关注流残留见 F08 |
| 消息中心 | 关注/点赞/评论触发通知；消息列表与未读数；已读用户条件 | 正常接口通过；原有消息幂等测试通过；消息写入失败补偿未做故障注入 |
| 曝光与观看 | 行为流水→曝光聚合；观看事件→画像 Worker | 真实观看→画像更新通过；内部接口鉴权见 F01 |
| 视频向量 | 发布事件→Embedding Worker→向量表 upsert | 真实消费和写入已验证；上游事件丢失见 F06 |
| 播放 | 默认配置回退、预加载、QoS 流水 | 正常接口返回通过；原有内部令牌及 QoS 幂等测试通过 |
| 内部接口 | recommendation-candidates、exposure-decisions、exposures、messages、playback-qos-reports | 前三个缺少鉴权，见 F01；后两个挂载内部令牌中间件 |

## 发现的问题（按修复优先级）

### F01 · P1 · 三个内部接口没有鉴权

位置：`apps/api/internal/interfaces/http/router/router.go:279`。

`/internal` 分组没有统一中间件，前三个 POST 也没有传入 `internalAuthMiddleware`。实际无 JWT、无内部令牌调用候选、曝光决策、曝光写入，分别返回 **200、200、201**。请求体可以指定其他用户 ID，可读取推荐上下文或污染曝光记录。

建议：在整个 `/internal` 分组挂载内部鉴权，补无令牌/错误令牌/正确令牌的生产路由测试。对应子用例 `03_internal_routes_require_auth`。

### F02 · P1 · 通用上传接受 HTML，并在应用同源下内联提供

位置：`interfaces/http/upload/handler.go:365`、`:429`、`:460`，以及 `interfaces/http/router/router.go:274`（均位于 `apps/api/internal/`）。

省略 `kind` 时归类为 `file`；该类别跳过扩展名/MIME 白名单。实际上传 `audit.html` 成功，静态 GET 返回 **200、text/html**，没有强制下载头。这形成同源主动内容入口，存在存储型脚本执行风险；测试没有执行攻击脚本。

建议：限制通用文件类型；主动内容用独立无凭证域提供，或强制附件下载，并设置合适的响应头。对应子用例 `10_html_upload_is_not_executable_on_app_origin`。

### F03 · P1 · 账号禁用后，已有 JWT 仍能发布视频

位置：`interfaces/http/middleware/jwtauth.go:16`、`application/account/service.go:59`、`application/video/service.go:37`。

登录时检查账号状态，后续鉴权仅验证 JWT，视频写入路径也不检查当前账号状态。实际把账号状态改为禁用后，旧 JWT 发布仍返回 **201**。

建议：在授权层校验账号状态/会话版本，必要时以短缓存降低查询成本，使封禁能阻断写操作。对应子用例 `09_disabled_account_cannot_publish`。

### F04 · P1 · 行为状态冷缓存时，取消点赞成功但没有落库

位置：`infra/cache/feedcache.go:578`、`:593`，`application/interaction/service.go:146`。

数据库已有点赞，但 Redis 用户行为键丢失时，缓存将未知状态当成未点赞；取消操作得到 `delta=0`，应用层因此不发送 MQ。真实测试清除该测试行为键后取消点赞，HTTP **200**，MySQL 仍为 **status=1、like_count=1**。收藏路径共用同一逻辑。

建议：冷缓存先回查真实用户行为；持久化目标状态不能仅由缓存计数 delta 决定，并处理缓存成功、投递失败的恢复。对应子用例 `05_cold_action_cache_unlike_reaches_mysql`。

### F05 · P1 · 乱序事件可以把较新的取消状态覆盖成点赞

位置：`application/interaction/worker.go:33`、`infra/persistence/interaction/gorm.go:109`。

事件带有时间，但消费者只传递目标状态和幂等键，没有版本比较。向真实仓储先交付较新的取消事件，再交付较旧的点赞事件，最终为 **status=1**。这是受控乱序注入，并非声称单消费者必然乱序；多实例消费、重投或并发发布会使这个条件成立。

建议：按用户/视频/行为维护单调版本，数据库有条件更新，或保证分区顺序并拒绝旧版本事件。对应子用例 `14_out_of_order_action_events_keep_latest_state`。

### F06 · P1 · 发布落库与事件投递之间没有恢复机制

位置：`application/video/service.go:37`、`:102`、`:110`。

视频事务先提交，消息发布错误被忽略；相同幂等键重试直接返回旧视频，也不会补发。真实 MySQL 配合已关闭的独立 MQ 客户端注入故障，两次业务调用均成功，但发布事件未送达，视频向量记录为 **0**。关注分发也依赖该事件。

建议：用事务 Outbox 与可重试投递闭合链路；RabbitMQ 使用 publisher confirms，并明确不可用时的降级/补偿行为。对应子用例 `12_publish_failure_is_repaired_on_idempotent_retry`。观看事件投递失败的补偿也需同类审查。

### F07 · P2 · 删除视频后，缓存 Feed 仍返回该视频

位置：`application/video/service.go:114`、`application/feed/utils.go:224`、`infra/cache/feedcache.go:87`。

删除仅更新数据库状态，未清理卡片/页/Inbox/热榜缓存，缓存命中时不重查发布状态。真实测试中视频详情已经 **404**，紧接着 Timeline 仍返回 **1 条已删除视频**。关注/热榜骨架与静态卡片的组合可能使残留持续更久。

建议：删除事件驱动缓存失效，并在组装阶段执行可见性过滤或版本/墓碑校验；过滤后应补页。对应子用例 `07_deleted_video_not_served_from_cache`。

### F08 · P2 · 取关后旧 Inbox 项没有移除或过滤

位置：`application/relation/service.go:88`、`application/feed/followingStrategy.go:90`、`infra/cache/feedcache.go:430`。

取关更新关系表，但不清理 Inbox；读取只用当前大 V 关注列表决定 Outbox，普通作者 Inbox 没有按实时关注关系过滤。真实取关成功后，关注流仍返回该作者的视频 **1 条**。

建议：取关清理对应作者项，并在读取阶段批量校验关注关系，覆盖并发分发场景。对应子用例 `06_unfollow_removes_old_inbox_items`。

### F09 · P2 · 评论短缓存过期后，计数回退到旧 Base

位置：`application/interaction/service.go:343`、`infra/cache/feedcache.go:117`、`:189`、`:674`、`:712`。

评论更新只刷新 JSON 统计，而点赞/收藏初始化的 Base 保留旧评论数。JSON 缓存失效后又从 Base 读取旧值。真实测试模拟 JSON TTL 到期，MySQL **comment_count=1**，Redis 重新组装结果却是 **0**。

建议：统一维护统计基准；更新评论数时同步 Base，避免短缓存失效重新写入旧快照。对应子用例 `04_comment_count_survives_json_ttl`。

### F10 · P2 · Redis 运行中故障时，关注流与热榜不会回退 MySQL

位置：`application/feed/followingStrategy.go:97`、`application/feed/hotStrategy.go:38`、`:117`。

仅启动时未配置缓存才走仓储；已有缓存对象返回连接错误时直接失败。用故障 Redis 客户端配合可用的真实 MySQL 仓储验证，两种场景都返回 `failed to load feed`。

建议：区分未命中和故障，故障回退数据库；热榜还需定义不同排序源之间游标的处理策略。对应子用例 `11_following_and_hot_fall_back_on_redis_failure`。

### F11 · P2 · 推荐深分页永远局限在第一批候选池

位置：`application/recommendation/service.go:23`、`:26`，`infra/persistence/recommendation/gorm.go:19`。

每页重新查询相同的 Top N 候选池，再在内存按游标过滤；仓储召回没有随游标向后推进。固定时间、固定数据的真实测试中，**102 条公开视频只返回 80 条**后就 `has_more=false`。

建议：使用能续读的召回游标/候选快照，或在候选耗尽时继续召回；动态评分需要稳定分页语义。对应子用例 `13_recommendation_pages_beyond_initial_pool`。

### F12 · P2 · 退出接口不会使旧访问令牌失效

位置：`interfaces/http/account/handler.go:94`、`interfaces/http/middleware/jwtauth.go:16`。

Logout 只返回 **204**，没有服务端会话撤销。随后用同一令牌读取 `/api/users/me` 仍返回 **200**。当前实现实际是“客户端丢弃令牌”；若产品明确只要求这种语义，可把本项定为契约说明问题，而不是撤销机制缺陷。

建议：明确登出契约；若要求服务端失效，增加会话版本或有效期内的 JTI 撤销。对应子用例 `08_logout_revokes_old_token`。

以上简写路径均相对 `apps/api/internal/`。

## 验证命令与边界

在 `apps/api` 下运行普通测试与静态检查：

```powershell
go test ./...
go vet ./...
```

显式运行真实基础设施审计（当前预期因已记录问题而失败）：

```powershell
$env:DREAMREEL_AUDIT_LIVE = '1'
go test ./test -run '^TestAuditLiveChains$' -count=1 -v
Remove-Item Env:DREAMREEL_AUDIT_LIVE
```

审计需要读取 `configs/config.yaml` 中的本机连接配置，以及创建独立测试库、测试队列的权限；不会输出连接密码或 JWT 密钥。

- HTTP 使用生产路由的 `httptest`，覆盖真实中间件、Handler、服务、仓储与 MQ；未经过 TCP HTTP 服务器、反向代理或浏览器。
- Windows PATH 中未发现 `ffmpeg`、`ffprobe`，WSL 查询也未找到相应可执行文件。本轮没有验证真实视频文件的探测、封装转换和播放；该依赖需要补齐后继续验证。
- 未进行并发压测、全量权限矩阵、浏览器跨域测试、RabbitMQ 重启恢复测试或媒体兼容性测试。
- 覆盖率采集遇到 Windows 沙箱临时目录重命名拒绝访问，未据此提供覆盖率数字。
- 代码审查还看到关注回填提交后失败缺少重试补偿、MQ 消费断连后缺少重连路径等风险；未在本轮故障测试中复现，未计入上述 12 项。

建议先处理 F01/F02/F03 的访问边界，再修复 F04/F05/F06 的数据一致性，随后处理 Feed 缓存、降级和分页。

