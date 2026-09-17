# Codex 对接 模块

把本机已安装的 Codex 接到看板可调用的后端。协议走 `codex app-server` JSONL，不把 Codex 当成本地模型的又一个供应商。

## 功能职责

- 发现本机是否装了 Codex、是否已取得授权、有哪些模型与模式
- 一个对话只走 Codex，`session_id` 即官方 `thread_id`
- 回合带上当前生效的模型与推理强度（本机配置或用户改过的）；Plan、权限仍只传用户改过的
- `/` 命令与官方扩展按钮共用同一套动作
- 给本条消息挂文件提及或图片
- 发送、排队、手动打断；正忙时不自动插话
- 分叉、归档、改标题、新开对话
- 回放正文、推理、命令、改文件、方案
- 回答跑命令、改文件、补一句、MCP 表单、额外权限
- 认不出的官方反问立刻按 `-32601` 回包，避免转圈
- 压缩按钮显示官方 `thread/tokenUsage/updated` 的剩余上下文（进程内存 + SSE，不入库）

## 边界

- 不走本地对话的工具、记忆和压缩
- 前端复用 Agent 对话页：新建会话时选 Agent 或 Codex，Codex 会话走 `/s/c/:id`，不单独开页面
- 绑了 Codex 的对话不能中途改成本地模型
- 一条对话同时只有一个进行中的回合；多条对话可以并行
- 共用本机 `~/.codex` 配置与授权，不另存密钥
- 凡 Codex CLI / app-server 能读到的都不进 CodeDock 数据库
- 排队、附件草稿、问票、SSE 环只在当前进程里；重启后不恢复、不重发
- `/mcp`、`/skills` 只提示去终端改

## 分层

- `pkg/codex`：领域类型与协议客户端，不 spawn CLI
- `internal/codex`：本机进程与内存编排
- `internal/handler/codex`：独立 `/codex/*` HTTP
- `packages/core/codex`：无头 `CodexClient` 与 SSE/reducer
- `packages/views/codex`：由 `ChatPage` 在 Codex 模式下组合（输入栏设置、时间线、问票、prompt）；归档在侧栏每条会话右侧
- `apps/web`：对话页 `/` 与 `/s/c/:id` 装配 `CodexClient`，不单独开 Codex 页
