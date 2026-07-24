# Steam Monitor

[English documentation](README.md)

一个独立运行的本地 Web 管理界面，用于监控公开的 Steam 用户资料。程序会定期调用 Steam Web API，将已完成的游戏会话保存到 SQLite，并按照配置的保留期限清理旧记录。项目不依赖 AstrBot、聊天平台或通知服务。

❤️ 本项目参考并改写自 [Maoer233/astrbot_plugin_steam_status_monitor](https://github.com/Maoer233/astrbot_plugin_steam_status_monitor)，原项目是 AstrBot Steam 状态监控插件。

## 运行

1. 从 [nep-0/steam-monitor Releases](https://github.com/nep-0/steam-monitor/releases) 下载对应操作系统的二进制文件。Linux/macOS 用户需要先执行 `chmod +x steam-monitor-*`。
2. 将 `config.example.json` 复制为 `config.json`，填写 `steam_api_key`。Steam Web API 密钥可以在 <https://steamcommunity.com/dev/apikey> 申请。
3. 启动程序：`./steam-monitor-linux-amd64 -config config.json`。其他系统请替换为实际下载的文件名。
4. 浏览器打开 <http://127.0.0.1:8080>，添加公开 Steam 资料并点击“Poll now”开始轮询。添加界面支持 SteamID64、Steam 个人资料链接、vanity URL、`s.team/p/` 链接，以及 8–10 位账号 ID；多个输入可以用逗号或换行分隔。

`config.json` 可以配置监听地址、数据库路径、轮询间隔、可选 HTTP 代理、请求超时和 `retention_days`。数据库及所有会话记录都会保存在 `database_path` 指定的位置。

示例 `config.json`：

```json
{
  "steam_api_key": "replace-with-your-Steam-Web-API-key",
  "listen_address": "127.0.0.1:8080",
  "database_path": "data/steam-monitor.db",
  "poll_interval_seconds": 60,
  "request_timeout_seconds": 15,
  "retention_days": 180,
  "proxy_url": "",
  "steam_api_base": "https://api.steampowered.com"
}
```

## 从源码构建

需要 Go 1.26 或更高版本：

```sh
go build -o steam-monitor .
cp config.example.json config.json
./steam-monitor
```

WebUI 源码位于 `frontend/`，使用 npm、Vite、React 和 TypeScript。修改前端后，可以重新生成嵌入 Go 二进制文件的静态资源：

```sh
cd frontend
npm install
npm run build
cd ..
go build -o steam-monitor .
```

生产环境静态资源会生成到 `web/dist/`，并嵌入最终的 Go 二进制文件。

WebUI 提供状态仪表盘、按名称/ID/当前游戏搜索玩家、编辑本地昵称、会话时间线、团队及单个玩家的日历热力图、游戏活动排行、每日活动统计、运行状态诊断，以及 CSV/JSON 会话导出。此前被监控的玩家从一次成功的轮询响应中消失时，程序会结束其正在进行的会话；旧记录会按照 `retention_days` 自动清理。

由于这是独立版本，项目不包含 AstrBot 集合、QQ/群组绑定、聊天命令、推送通知和权限管理等功能。
