# DreamReel 实验室性能基准

本套件在 Linux 上编译两个版本的生产路由，用真实 MySQL、Redis 和 RabbitMQ 比较推荐、Feed 读取、发布扇出和互动写入。另有 Node 长会话缓冲测试，以及已部署站点的静态资源条件请求测试。[测试报告](../../docs/performance_lab_2026-11-13.md)

## 从当前工作区打包

在仓库根目录运行。需要 Git 中存在推荐分页改造的父提交 `1cfc6fd`。打包器只收集指定的源码和测试文件，保留当前工作区未提交的 Go 改动。目标归档名需尚未使用。

```powershell
python benchmarks/lab/pack.py dreamreel-lab.tar.gz
$labDir = (ssh target-wsl 'mktemp -d ~/dreamreel-perf.XXXXXX').Trim()
scp dreamreel-lab.tar.gz "target-wsl:${labDir}/src.tar.gz"
ssh target-wsl "cd '$labDir' && tar xzf src.tar.gz && LAB_CONFIG=~/DreamReel/.runtime/config.yaml python3 -u run.py results"
```

实验机需提供 Go 及两版 go.mod 的依赖、Python 3 与 PyYAML、Node、MySQL 8 的服务端和命令行工具、Redis 服务端和命令行工具、Ubuntu 的 dpkg-query。默认实验环境的 RabbitMQ 管理 API 在回环地址 15672，`LAB_CONFIG` 指定配置中的 `rabbitmq.url` 需对应回环 broker，且账号可创建及回收测试 vhost。套件从现有账号建立随机命名 vhost，进程正常结束或 Python 异常退出时删除它。MySQL 与 Redis 仅在本次新建的私有目录监听 Unix socket，Go 进程也监听该目录下的 socket。

在目标机已部署 DreamReel 且回环 80 端口提供前端时，可追加静态 JS 的 ETag 实测并汇总，然后下载证据。

```powershell
ssh target-wsl "cd '$labDir' && python3 assets.py results/assets.jsonl && python3 analyze.py results"
scp -r "target-wsl:${labDir}/results" ./dreamreel-lab-results
```

再次测试应使用新的结果目录。若运行中任何响应、页长、期望数据、消费者计数或落库校验失败，运行器以非零状态退出。可以检查失败原因后新起一次完整实验。所有正式场景均使用同一包内的源码。

## 读取证据

- `source.json` 给出基线提交、当前工作区 HEAD 和打包文件的 SHA-256。`environment.json` 记录实测环境及视频、账号、关注关系、向量行数。`plans.txt` 保存深分页的 MySQL 实际执行计划。
- `components.jsonl`、`http.jsonl`、`buffer.jsonl`、`assets.jsonl` 中每行对应一个场景、一个版本的一轮；`sample_ms` 保留原始耗时。HTTP 文件另有客户端数和整轮墙钟时间。异步组记录实测消费数和队列排空后的数据库核对耗时。
- `rounds.csv` 可检查每轮波动。`summary.csv` 将四轮指标取中位数；样本数、重复视频和已消费消息数跨轮求和。统一列名 `ops_s` 对连续翻页表示会话/秒，对单请求表示请求/秒。各场景的工作量和比较条件见报告。
- 获取保存的原始文件后，离线运行 `python benchmarks/lab/analyze.py <结果目录>` 即可重算两张 CSV。
