# 推荐会话队列 k6 A/B 压测

对比两个 DreamReel 版本的推荐分页性能。k6 通过两个版本各自的生产路由请求 `POST /api/feed-queries`。测试使用 MySQL 和 Redis，API 仅监听本机回环地址，RabbitMQ 关闭。

## 执行

需要 Windows PowerShell 5.1 或 PowerShell 7、Python、Go 和 k6，本机配置指向可访问的 MySQL、Redis。运行脚本需要访问本机端口、启动后台子进程，以及创建和清理独立测试库的权限。

从 DreamReel 仓库根目录执行。将 `$baselineApi` 指向基线检出的 `apps/api` 目录。两个版本都需要提供 `router.Register(g, cfg, db, rdb, rabbitMQ)` 和推荐查询路由。

```powershell
$baselineApi = '../DreamReel-baseline/apps/api'
./benchmarks/recommendation/run.ps1 -BaselineRoot $baselineApi -Videos 10000 -Rounds 3 -Duration 20s
```

`run.ps1` 使用 UTF-8 BOM 编码，以兼容 Windows PowerShell 5.1。编辑后请保留 BOM，目录中的 `.editorconfig` 已声明该约定。

脚本在 `.work` 内独立构建基线和当前版本。连接配置默认使用当前版本的 `apps/api/configs/config.yaml`，也可传入 `-Config`。测试使用新建的数据库和两个原先为空的 Redis 逻辑库。脚本退出时停止测试进程并按资源清单清理。如果进程被强制结束，可从仓库根目录使用同一配置和对应的 `fixture.json` 清理。

```powershell
./benchmarks/recommendation/.work/current.exe -action cleanup `
  -config ./apps/api/configs/config.yaml `
  -fixture './benchmarks/recommendation/results/<run>/fixture.json'
```

## 场景

| 场景 | 调度 | 工作量 |
|---|---|---|
| cached | 固定到达率 100 次/秒 | 每个 VU 复用预热完成的第二页游标 |
| first | 固定到达率 50 次/秒 | 每次创建新的推荐请求/会话 |
| scroll8 | 20 VU 闭环 | 每次迭代从首屏连续请求 8 页，共 80 个视频 |
| long | 新方案 10 VU 闭环 | 每次迭代连续请求 25 页，共 250 个视频，覆盖一次补池 |

每种场景执行三轮，每轮 20 秒，A/B 先后顺序交替。cached 场景将首屏建池和第二页卡片预热放在测量区间之外。scroll8 则包含建池成本，用于观察用户连续刷流时的整体开销。long 用来测量补池边界，不与旧方案超过其候选池范围的空响应比较。

每个响应校验状态码和视频数量，刷流场景同时校验会话内是否出现重复视频。旧方案阈值失败会保留退出码和数据，新方案阈值失败会中止运行。

## 数据与测量口径

- 默认 10,000 个已发布视频、64 个账号和用户兴趣画像、128 维 HashNgram 向量。
- 两份数据使用相同的种子时间、标题、向量、热度和作者分布，开始时没有曝光记录。
- 每个 API 的 `GOMAXPROCS=4`，数据库连接池最大 50、空闲 10。k6 与数据库及 Redis 运行在同一台机器，环境详情记录于 `environment.json`。
- 原有库和依赖版本分别保留。对比反映两个项目版本在该测试环境的表现，依赖与外围实现差异也可能影响测量。
- HTTP 延迟采用 k6 的 `response.timings.duration`，包含发送、等待和接收时间。
- 查询计数通过 database/sql 驱动包装器完成。驱动返回 `ErrSkip` 的回退调用不重复计数，预编译语句实际执行计数一次。普通翻页中的可见性检查和互动状态读取也计入 SQL。
- 测量起点在 k6 setup 的预热之后，终点在 VU 完成后的 teardown。观测吞吐量使用请求数除以这个区间。该口径排除了预热，仍包含闭环末尾正在执行的完整刷流迭代。
- 汇总的 P95 是“三轮各自 P95 的中位数”。原始逐轮 P95、P99、吞吐量、丢弃迭代、完整性检查、SQL 次数都保留。
- Redis 键数和 Go 堆内存是结束时快照，不能作为峰值内存或总资源消耗的结论。

## 文件

- `recommendation.js` 为 k6 负载脚本。
- `harness.go.tmpl` 为共用启动与计数代码模板，两个版本使用相同计数方式。
- `prepare.py` 为源码复制和构建脚本。每次运行的 `source-manifest.json` 保存两份源码的 SHA-256。
- `run.ps1` 为执行与清理入口。
- `analyze.py <results-directory>` 重新生成 `analysis.json` 与 `rounds.csv`。

k6 指标和调度模型的官方说明可参阅 [内置指标](https://grafana.com/docs/k6/latest/using-k6/metrics/reference/) 与 [开放和闭环负载模型](https://grafana.com/docs/k6/latest/using-k6/scenarios/concepts/open-vs-closed/)。
