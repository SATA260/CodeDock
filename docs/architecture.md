# CodeDock 架构与目录边界

本文档定义 CodeDock 当前的技术骨架。目录按能力拆分；Issue、Task、Review、Workspace 等业务目录不属于本项目的基础结构。

Agent Loop 已闭环：Handler 写用户消息与 Run，Worker 领取后由 Runtime 装上下文、调模型、执行 Tool，事件先落库再经 Bus 由 SSE 消费。当前只注册示例 Tool `ping`。

## 总体架构

```text
Web 客户端
    |
    | HTTP / SSE
    v
server/cmd/server
    |
    v
server/internal/handler
    |-- CRUD / SSE / Start / Continue / Cancel / 审批 --> pkg/db/sqlite
    |-- 领取 Run 后的 Loop --> internal/agent
    |-- 用户记忆查看 / 删除 --> pkg/db/sqlite
    |
    v
server/internal/agent
    |-- sqlc 读写 Run / Turn / 事件 / 用量 / checkpoint
    |-- 调用 pkg/agent 无状态方法
    |-- 先持久化 AgentEvent，再发布 internal/events.Bus
    |-- memory：热层目录+专题，冷层按工作区 FTS（后期 Loop 单向调用）
    |
    v
server/pkg/agent
```

`pkg/ai` 已删除。大模型调用放在 `pkg/agent`，由 `ModelConfig` 在方法内创建，不由 Runtime 注入。

## 当前目录骨架

```text
CodeDock/
├── apps/
│   └── web/
│       └── app/
├── docs/
├── server/
│   ├── cmd/server/              # 服务启动、配置、Router 和依赖装配
│   ├── internal/
│   │   ├── handler/             # 大部分 HTTP：CRUD、SSE、Start / Continue / Cancel、记忆查看/删除
│   │   ├── agent/               # 运行时编排 + sqlc 持久化
│   │   │   └── memory/          # 热层目录+专题，冷层工作区 FTS 索引
│   │   ├── events/              # 进程内事件总线
│   │   ├── config/
│   │   ├── logger/
│   │   ├── errors/
│   │   └── util/
│   ├── pkg/
│   │   ├── agent/               # 全部通用无状态逻辑，含模型调用与 Tool
│   │   └── db/                  # Client 与 sqlc 生成代码
│   ├── migrations/
│   ├── go.mod
│   └── go.sum
└── AGENTS.md
```

## 依赖方向

```text
cmd/server
  -> internal/handler
  -> internal/agent
  -> internal/events
  -> pkg/db

internal/handler
  -> pkg/db/sqlite.Queries
  -> pkg/agent          # 映射响应、token 统计、Profile 装配
  -> internal/agent     # Worker 领取后的 Loop
  -> internal/agent/memory  # 用户侧记忆响应类型

internal/agent
  -> pkg/db/sqlite.Queries
  -> pkg/agent
  -> internal/events
  -> internal/agent/memory  # 后期 Loop 单向调用

internal/agent/memory
  -> pkg/db/sqlite.Queries
  不 import 父包 internal/agent
  不负责 Prompt / Context Packet / 压缩

pkg/agent
  不依赖 handler、internal、sqlc
  不持有包级状态，不查库
```

## 分层职责

### `internal/handler`

承担大部分接口逻辑：

- Session / Message / Usage / Approval 的增删改查
- 用户侧 TextMemory 的查看与删除（不提供写入，不暴露 message 索引；List 用 user_id / workspace_id，Get/Delete 用 name 默认目录）
- SSE：先按 `afterSeq` / `Last-Event-ID` 回放已落库事件，再 `SubscribeAll` 并按 Session 过滤；客户端断开不取消 Run
- Run 的 Start / Continue / Retry / Cancel 和审批裁决直接在 Handler 中处理，需要执行时再交给 Worker
- 同一 Session 只有一个 active Run：`interrupt` 先取消再开新 Run；`queue` 只落库，当前结束后自动领取

Handler 直接依赖 `*sqlite.Queries`，不经过 Store 接口。

### `internal/agent`

运行时编排和持久化：

- 用 sqlc 读 Session、Run、消息、checkpoint、lease
- 调用 `pkg.Load` / `CompactIfNeeded` / `Build` / `Stream` / `Dispatch`
- 同事务递增 `sessions.last_event_seq` 并插入 `AgentEvent`，提交后再 `Bus.Publish`
- `Transition` 消费模型流，把增量先落库再发到事件总线
- Worker 用带缓冲 channel 领取 Run；启动时恢复非 `waiting_approval` 且 lease 空/过期的 Run
- 审批暂停时写入 `run_tool_checkpoints`，批准后从 checkpoint 继续，不重跑已完成工具

不实现提示词、压缩算法或模型适配。

### `internal/agent/memory`

热层是每个 scope 一篇目录（`kind=index`，`name` 固定 `index`）加多篇专题（`kind=topic`）。scope 只有 `user` / `workspace`。冷层是同一 `workspace_id` 下的 context message FTS。与 Loop 独立：

- 类型、`ByteLen`、`IndexOverBudget`（200 行 / 25KB，本阶段空实现）
- Agent 侧 TextMemory 的 Get / Upsert / Delete / List
- `SearchMessages` 只查不写；`IndexMessage` 供 Loop 后期写 message 时调用
- `ReadTool` / `WriteTool` / `SearchTool` 空 stub，不写入默认 Registry

不 import 父包；不解析 Markdown；不负责 Prompt / Context Packet / 压缩。不放在 `pkg`。不新建 Workspace 业务包，只持有 `workspace_id` 字段。本阶段 Loop 不装载目录、不调用 `IndexMessage`。后期：新 Session / 压缩后装目录；写 message 时 `IndexMessage`。

### `pkg/agent`

全部 Agent 通用逻辑，方法无状态：

- 领域类型、状态机 `CanTransition`、用量 `CountTokens`（UTF-8 字节 / 4）
- 提示词 `Build`
- 上下文 `Load` / `NeedsCompaction` / `CompactIfNeeded`
- Tool 抽象、内存 `Registry`、示例 `ping`，以及无状态 `Dispatch`
- Agent 配置抽象 `profile.Config` 与 `RunConfigSnapshot`
- 模型调用 `Stream` / 压缩：在函数内按 `ModelConfig.Provider` 创建
  - `fake`：读 `Model.Options` 脚本（多段 text / tool_calls、失败次数、可取消挂起），测试不打外网
  - `openai`：OpenAI 兼容 HTTP（`BaseURL` + API Key）

### `pkg/db`

统一数据库入口。SQLite 已接入 sqlc；Handler 和运行时直接使用 `*sqlite.Queries`。启动时按文件名顺序应用 `migrations/*.sql`。

### `internal/events`

进程内同步发布订阅总线。`Subscribe` / `SubscribeAll` 返回 unsubscribe，避免 SSE 泄漏监听器。流式事件是通知，重连必须按 `event_seq` 回放已落库事件。

## 组装关系

```text
Handler CRUD / SSE / Start / Continue / Cancel / 审批裁决 / 记忆查看删除
  -> sqlc
  -> 必要时 pkg.CountTokens
  -> Worker.Submit

Worker
  -> internal/agent.Execute
       -> sqlc 读写
       -> pkg.Load / CompactIfNeeded
       -> pkg.Build
       -> pkg.Stream          # 按 ModelConfig 在 pkg 内创建 fake 或 openai
       -> Transition：先落库再 Bus
       -> pkg.Dispatch        # ping 及测试用 Tool
```

## 配置

`LLM_PROVIDER`（`openai` | `fake`，默认 `fake`）、`LLM_MODEL`、`LLM_API_KEY`、`LLM_BASE_URL`。Handler 创建 Run 时写入 `RunConfigSnapshot`，后续 Turn 只读快照。

修改 Agent 能力或跨端协议时，需要检查契约、取消与终态、流式事件语义以及敏感信息处理。
