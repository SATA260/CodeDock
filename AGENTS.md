# CodeDock Agent 开发说明

修改 CodeDock 代码前，先阅读 [`docs/architecture.md`](docs/architecture.md)。该文档是当前目录归属和模块边界的依据。

Agent Loop 已闭环：用户发文本、装上下文、调模型、产出文字或 Tool、事件落库并由 SSE 消费。当前只注册示例 Tool `ping`（返回 `{"ok":true}`），不实现文件 / Shell / Git。

## 目录放置规则

- 服务启动、配置读取、Router 和依赖装配放在 `server/cmd/server`。
- 大部分 HTTP 逻辑放在 `server/internal/handler`：Session / Message / Usage / Approval 的 CRUD，SSE，Run 的 Start / Continue / Cancel，审批裁决，以及用户侧记忆查看/删除。
- Agent 运行时编排和 sqlc 持久化放在 `server/internal/agent`。
- Markdown 记忆（热层目录+专题）与 context message 索引（冷层按工作区 FTS）放在 `server/internal/agent/memory`；不放 `pkg/memory`。memory 不 import 父包 `internal/agent`。
- Agent 通用无状态逻辑放在 `server/pkg/agent`：类型、token 统计、提示词、上下文、Tool、Agent 配置、模型调用。
- 进程内事件总线放在 `server/internal/events`。
- 数据库入口和 sqlc 生成代码放在 `server/pkg/db`。
- 数据库结构演进放在 `server/migrations`。
- Web 路由和页面入口放在 `apps/web`。
- 不要创建 `server/pkg/ai`。大模型调用属于 `pkg/agent`。

## 边界规则

- `pkg/agent` 不持有包级状态，不查库，不依赖 handler 或 internal。
- 模型由 `pkg/agent` 在 `Stream` / `CompactIfNeeded` 内按 `ModelConfig` 创建：`provider=fake` 走脚本化假模型（测试用），`provider=openai` 走 OpenAI 兼容 HTTP。不由 Runtime 注入模型实例。
- Handler 直接使用 `*sqlite.Queries` 做 CRUD、SSE 回放、Run 的 Start / Continue / Cancel 和审批裁决；领取后的 Loop 才进入 `internal/agent`。
- `internal/agent` 负责把 pkg 的计算结果持久化为 Run、Turn、消息、用量和事件。`AgentEvent` 必须先同事务写入并递增 `sessions.last_event_seq`，提交后再 `events.Bus.Publish`。
- 示例 Tool 只有 `ping`。业务 Tool（文件 / Shell / Git）本阶段不实现。
- `internal/agent/memory` 负责 TextMemory 的 Get / Upsert / Delete / List、`SearchMessages` 和 `IndexMessage`；用户侧只看/删目录与专题。不负责 Prompt / Context Packet / 压缩。记忆工具不注册到默认 Registry。本阶段 Loop 不装载目录、不调用 `IndexMessage`。后期：新 Session / 压缩后装目录；写 message 时 `IndexMessage`。
- 不要使用 Store 接口包装 sqlc。
- Agent 契约不得依赖 React、UI 包或路由框架。
- 流式事件是通知，不是真实数据源；重连时应按 `event_seq` 回放。
- 不要预先创建 Issue、Task、Review、Workspace 或其他具体业务域目录。
- 不要把产品工作流放入 `server/pkg`。

当需求变更没有明显的代码归属时，先依据 `docs/architecture.md` 对其分类，再开始编写代码。
