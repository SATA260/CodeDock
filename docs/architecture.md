# CodeDock 架构与目录边界

本文档定义 CodeDock 当前的技术骨架。目录按能力拆分；Issue、Task、Review、Workspace 等业务目录不属于本项目的基础结构。

Agent Loop 已闭环：Handler 写用户消息与 Run，Worker 领取后由 Runtime 装上下文、调模型、执行 Tool，事件先落库再经 Bus 由 SSE 消费。默认注册 `ping` 与记忆工具。

## 总体架构

```text
apps/web  (路由 + NEXT_PUBLIC_* + AgentClient / AgentProvider)
    |
    v
packages/views  (Chat 壳 / hooks，无 next/*)
    |         \
    v          v
packages/core   packages/ui
(HTTP / SSE /   (primitives / AI Elements / tokens，无业务)
 Timeline reducer，无 React)
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
    |-- memory：热层目录+专题，冷层按工作区 FTS；Loop 装目录并 IndexMessage
    |-- tools：工具定义在本包；New 注入 Ports（Execute 用的外部实现）后 Register
    |
    v
server/pkg/agent
```

`pkg/ai` 已删除。大模型调用放在 `pkg/agent`，由 `ModelConfig` 在方法内创建，不由 Runtime 注入。

## 当前目录骨架

```text
CodeDock/
├── apps/
│   └── web/                     # Next.js 路由与平台装配；不解析 SSE
├── packages/
│   ├── core/                    # 无头业务；按业务域拆（现有 chat/），不要 src/
│   ├── ui/                      # 无业务语义；components / lib / styles，不要 src/
│   └── views/                   # 组合层；按业务域拆（现有 chat/），不要 src/
├── docs/
├── server/
│   ├── cmd/server/              # 服务启动、配置、Router 和依赖装配
│   ├── internal/
│   │   ├── handler/             # 大部分 HTTP：CRUD、SSE、Start / Continue / Cancel、记忆查看/删除
│   │   ├── agent/               # 运行时编排 + sqlc 持久化
│   │   │   ├── memory/          # 热层目录+专题，冷层工作区 FTS 索引
│   │   │   └── tools/           # 具体工具定义：ping、memory_*
│   │   ├── events/              # 进程内事件总线
│   │   ├── config/
│   │   ├── logger/
│   │   ├── errors/
│   │   └── util/
│   ├── pkg/
│   │   ├── agent/               # 全部通用无状态逻辑，含模型调用与 Tool 抽象
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
  -> internal/agent/memory
  -> internal/agent/tools

internal/agent/memory
  -> pkg/db/sqlite.Queries
  不 import 父包 internal/agent
  不定义 Tool
  不负责 Prompt / Context Packet / 压缩

internal/agent/tools
  -> pkg/agent/tool
  -> internal/agent/memory
  -> pkg/db/sqlite.Queries
  不 import 父包 internal/agent

pkg/agent
  不依赖 handler、internal、sqlc
  不持有包级状态，不查库
  Tool 包只含接口、Registry、Dispatch，不含具体工具定义

packages/core
  不依赖 React、Next、DOM、process.env、AI SDK
  按业务域拆目录（chat、以后的 auth / memory），不要 src/
  文件直接落在 packages/core/<domain>/
  baseUrl / userId 由调用方注入

packages/ui
  不依赖 core，不知道 Session / Run / TimelineItem
  只提供通用组件与 token：components / lib / styles
  不要 src/，不按业务域拆

packages/views
  -> packages/core
  -> packages/ui
  不 import next/*
  按业务域拆目录，与 core 对齐（现有 chat）
  AgentProvider 在包根注入 client + userId；导航用回调

apps/web
  -> packages/views
  -> packages/core          # 创建 AgentClient
  -> packages/ui            # 引入 tokens.css
  不直接解析 SSE 或 event type
```

## 分层职责

### `internal/handler`

承担大部分接口逻辑：

- Session / Message / Usage / Approval 的增删改查。`sessions.summary` 在首次用户消息写入，列表与详情返回
- 用户侧 TextMemory 的查看与删除（不提供写入，不暴露 message 索引；List 用 user_id / workspace_id，Get/Delete 用 name 默认目录）
- SSE：先按 `afterSeq` / `Last-Event-ID` 回放已落库事件，再 `SubscribeAll` 并按 Session 过滤；客户端断开不取消 Run
- 事件 JSON 回放：`GET /sessions/{id}/event-log`，供前端一次 hydrate，不替代 SSE 直播
- Run 的 Start / Continue / Retry / Cancel 和审批裁决直接在 Handler 中处理，需要执行时再交给 Worker
- 同一 Session 只有一个 active Run：`interrupt` 先取消再开新 Run；`queue` 只落库，当前结束后自动领取

Handler 直接依赖 `*sqlite.Queries`，不经过 Store 接口。

### `internal/agent`

运行时编排和持久化：

- 用 sqlc 读 Session、Run、消息、checkpoint、lease
- 调用 `pkg.Load` / `CompactIfNeeded` / `Build` / `Stream` / `Dispatch`
- 同事务递增 `sessions.last_event_seq` 并插入 `AgentEvent`，提交后再 `Bus.Publish`
- `Transition` 消费模型流，把增量先落库再发到事件总线
- Worker 用带缓冲 channel 领取 Run；启动时恢复非 `waiting_approval`、以及 checkpoint 已写入裁决的 `waiting_approval`
- 一次模型回复里的待批 Tool 合成一条审批；前端一次提交对每条批/拒，全部裁定后才从 `waiting_approval` 恢复。被拒或执行失败的工具把错误结果喂回模型，默认 `best_effort` 继续其余调用，不把 Run 标成 `model_error`。checkpoint 分开记录已执行 / 已批准 / 已拒绝

不实现提示词、压缩算法或模型适配。

### `internal/agent/memory`

热层是每个 scope 一篇目录（`kind=index`，`name` 固定 `index`）加多篇专题（`kind=topic`）。scope 只有 `user` / `workspace`。冷层是同一 `workspace_id` 下的 context message FTS。与 Loop 独立：

- 类型、`ByteLen`、`IndexOverBudget`、`ClipIndex`（200 行 / 25KB）
- Agent 侧 TextMemory 的 Get / Upsert / Delete / List
- `SearchMessages` 只查不写；`IndexMessage` 由 Loop 写 message 时调用

不 import 父包；不解析 Markdown；不负责 Prompt / Context Packet / 对话压缩；不定义 Tool。不放在 `pkg`。不新建 Workspace 业务包，只持有 `workspace_id` 字段。Loop 在新 Session / 对话压缩后装冻结目录（独立 system 消息，不拼进静态 prompt，不入库）。超限目录立刻写入，由 Runtime 后台 `pkg.CompactIndex` 改短盖写，不自动建专题，不改当前 Session 冻结前缀。

### `internal/agent/tools`

工具定义全部在本包。Runtime `New` 接收 `Ports`（Execute 要调用的外部实现），再 `Register`：

- 本包写工具名、入参/出参、schema、权限和编排。`ping`，以及 `memory_read` / `memory_write` / `memory_search`（依赖 `memory` 与 Session）
- Execute 若依赖外部能力，只通过 `Ports` 上的接口调用；由 `cmd/server` 在初始化时注入具体实现。未注入的字段不注册对应工具
- 本阶段没有外部 Port（不实现文件 / Shell / Git）

每个工具只定义入参/出参结构体；执行用 `encoding/json`，给模型的 schema 由 `jsonschema.For` 从类型推断。Agent 通过 `Profile.Tools.Names` 绑定工具。运行模式提供 `read` / `write` / `memory` 能力，只有模式覆盖了工具声明的全部能力时该工具才对模型可见且可 Dispatch。记忆工具声明 `memory`。审批仍由工具声明 `RequiresApproval`，`ask_for_approval` 暂停、`auto_approve` / `yolo` 自动过。一批待批工具对应一条审批，一次提交审完再流转。不 import 父包 `internal/agent`。测试用 Tool 可留在测试文件。

### `pkg/agent`

全部 Agent 通用逻辑，方法无状态：

- 领域类型、状态机 `CanTransition`、用量 `CountTokens`（UTF-8 字节 / 4）
- 提示词 `Build`
- 上下文 `Load` / `NeedsCompaction` / `CompactIfNeeded`
- Tool 抽象、内存 `Registry`、无状态 `Dispatch`（不含具体工具定义）
- Agent 配置抽象 `profile.Config` 与 `RunConfigSnapshot`
- 模型调用 `Stream` / 压缩：在函数内按 `ModelConfig.Provider` 创建
  - `fake`：读 `Model.Options` 脚本（多段 text / tool_calls、失败次数、可取消挂起），测试不打外网
  - `openai`：OpenAI 兼容 HTTP（`BaseURL` + API Key）

### `pkg/db`

统一数据库入口。SQLite 已接入 sqlc；Handler 和运行时直接使用 `*sqlite.Queries`。启动时按文件名顺序应用 `migrations/*.sql`。

### `internal/events`

进程内同步发布订阅总线。`Subscribe` / `SubscribeAll` 返回 unsubscribe，避免 SSE 泄漏监听器。流式事件是通知，重连必须按 `event_seq` 回放已落库事件。

### `packages/core`

跨端无头业务，无 UI。按业务域拆目录，文件直接放在 `packages/core/<domain>/`，不要 `src/`。现有 `chat/`：Session / Message / Run / 审批的 HTTP、SSE、Timeline reducer。有鉴权再加 `auth/`，有记忆再加 `memory/`，不预建空目录。`baseUrl` / `userId` 由调用方注入。不依赖 React。第一版 thinking 用 Run 状态（`queued` / `loading_context` / `running_llm`），不是模型 reasoning token。

### `packages/ui`

无业务语义。不要 `src/`，也不按业务域拆：

- `components/ui/`：Button、Collapsible
- `components/`：Conversation、Message（`MessageResponse` 用 Streamdown 渲染 Markdown）、Reasoning、Tool、Confirmation、PromptInput
- `lib/`、`styles/`：`cn`、JSON 展示、zinc token

不依赖 core，不知道 Session / Run / TimelineItem。

### `packages/views`

组合 core + ui。按业务域拆，与 core 对齐，不要 `src/`。现有 `chat/`：`ChatPage`、侧栏、瀑布、审批、prompt。包根 `provider.tsx` 注入 `AgentClient` + `userId`。`ChatPage` 接 `sessionId` 与 `onOpenSession`。不 import `next/*`。新业务新建目录，不预建 Issue / Task / Review / Workspace。

### `apps/web`

路由、`NEXT_PUBLIC_API_BASE` / `NEXT_PUBLIC_USER_ID`、创建 `AgentClient`、包 `AgentProvider`、`router.push`。本机 Web 直连 `:8080`（CORS）。

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
       -> pkg.Dispatch        # ping、memory_* 及测试用 Tool
```

## 配置

`LLM_PROVIDER`（`openai` | `fake`，默认 `fake`）、`LLM_MODEL`、`LLM_API_KEY`、`LLM_BASE_URL`。Handler 创建 Run 时写入 `RunConfigSnapshot`，后续 Turn 只读快照。

HTTP 出站领域对象使用 snake_case JSON。Router 对带 Origin 的请求回显 CORS，便于本机 Web 直连 `:8080`。Web 用 `NEXT_PUBLIC_API_BASE`（默认 `http://localhost:8080`）和 `NEXT_PUBLIC_USER_ID`（默认 `local`）。

修改 Agent 能力或跨端协议时，需要检查契约、取消与终态、流式事件语义以及敏感信息处理。
