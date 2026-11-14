# 原始前端备份

- 文件：`dreamreel-web-original-20261106-201520.zip`
- 备份时间：2026-11-06 20:15（Asia/Hong_Kong）
- 范围：重构前 `apps/web` 的完整源码、测试、依赖锁文件、入口、环境示例、Vite / Docker / Nginx 配置，共 20 个文件。
- 排除：可重新生成的 `node_modules`、`dist`、`.npm-cache`。
- SHA256：`DA9C7215CBDE21A60DC5157D64EBB6B01D23610A45B50AF243377AFD6AA6D399`

恢复前先保存当前版本。在项目根目录运行以下命令，会用原始文件覆盖当前前端的同名文件；新增加但未被原始入口引用的组件无需删除：

```powershell
Expand-Archive -LiteralPath .\backups\dreamreel-web-original-20261106-201520.zip -DestinationPath .\apps\web -Force
cd .\apps\web
npm run build
```

如果依赖目录缺失或依赖版本有变化，构建前运行 `npm ci`。
