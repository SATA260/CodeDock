# CodeDock 架构与目录边界

本文档定义 CodeDock 当前的技术骨架。目录按能力拆分；Issue、Task、Review、Workspace 等业务目录不属于本项目的基础结构。

当前阶段只搭架构，方法均为空实现，不写具体业务规则。

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
    |
    v
server/internal/agent
    |-- sqlc 读写 Run / Turn / 事件 / 用量
    |-- 调用 pkg/agent 无状态方法
    |-- 发布 internal/events.Bus
    |
    v
server/pkg/agent
```

`pkg/ai` 已删除。大模型调用和 Eino 适配都放在 `pkg/agent`。

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
│   │   ├── handler/             # 大部分 HTTP：CRUD、SSE、Start / Continue / Cancel
│   │   ├── agent/               # 运行时编排 + sqlc 持久化
│   │   ├── events/              # 进程内事件总线
│   │   ├── config/
│   │   ├── logger/
│   │   ├── errors/
│   │   └── util/
│   ├── pkg/
│   │   ├── agent/               # 全部通用无状态逻辑，含 Eino 模型调用
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

internal/agent
  -> pkg/db/sqlite.Queries
  -> pkg/agent
  -> internal/events

pkg/agent
  不依赖 handler、internal、sqlc
  不持有包级状态，不查库
```

## 分层职责

### `internal/handler`

承担大部分接口逻辑：

- Session / Message / Usage / Approval 的增删改查
- SSE：回放 `afterSeq` 之后的持久化事件，再订阅 `internal/events.Bus`
- Run 的 Start / Continue / Retry / Cancel 和审批裁决直接在 Handler 中处理，需要执行时再交给 Worker

Handler 直接依赖 `*sqlite.Queries`，不经过 Store 接口。

### `internal/agent`

运行时编排和持久化：

- 用 sqlc 读 Session、Run、消息、checkpoint、lease
- 调用 `pkg.Load` / `CompactIfNeeded` / `Build` / `Stream` / `Dispatch`
- `Transition(bus, stream)` 把 LLM 增量发到事件总线
- 用 sqlc 写 Run、Turn、消息、用量、事件、checkpoint
- Worker 用 channel 和 goroutine 领取 Run

不实现提示词、压缩算法或 Eino 适配。

### `pkg/agent`

全部 Agent 通用逻辑，方法无状态：

- 领域类型与枚举
- 用量 `CountTokens`，统计一段文本的 token 数
- 提示词 `Build`
- 上下文 `Load` / `NeedsCompaction` / `CompactIfNeeded`
- Tool 抽象与无状态 `Dispatch`
- Agent 配置抽象 `profile.Config`
- 模型调用 `Stream` / 压缩：本包内创建 Eino `ToolCallingChatModel`，不由运行时注入

### `pkg/db`

统一数据库入口。SQLite 已接入 sqlc；Handler 和运行时直接使用 `*sqlite.Queries`。

### `internal/events`

进程内同步发布订阅总线，供运行时发布、SSE 订阅。

## 组装关系

```text
Handler CRUD / SSE / Start / Continue / Cancel / 审批裁决
  -> sqlc
  -> 必要时 pkg.CountTokens
  -> Worker.Submit

Worker
  -> internal/agent.Execute
       -> sqlc 读写
       -> pkg.Load / CompactIfNeeded
       -> pkg.Build
       -> pkg.Stream
       -> Transition(bus, stream)
       -> pkg.Dispatch
```

修改 Agent 能力或跨端协议时，需要检查契约、取消与终态、流式事件语义以及敏感信息处理。
