# CodeDock 架构与目录边界

本文档定义 CodeDock 当前的技术骨架。目录按能力拆分；Issue、Task、Review、Workspace 等业务目录不属于本项目的基础结构。

Agent Loop 已闭环：Handler 写用户消息与 Run，Worker 领取后由 Runtime 装上下文、调模型、执行 Tool，事件先落库再经 Bus 由 SSE 消费。三个内置 Agent（ask / plan / agent）共用同一 Loop；审批走工具 → Agent 表 → 审批模式流水线。

## 总体架构

```text
apps/web  (路由 + NEXT_PUBLIC_* + AgentClient / GitClient / CodexClient)
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
    |-- Git HTTP --> pkg/git（本机 CLI，无产品流程）
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
server/pkg/git
server/pkg/codex
```

`pkg/ai` 已删除。大模型调用放在 `pkg/agent`，由 `ModelConfig` 在方法内创建，不由 Runtime 注入。

## 当前目录骨架

```text
CodeDock/
├── apps/
│   └── web/                     # Next.js 路由与平台装配；不解析 SSE
├── packages/
│   ├── core/                    # 无头业务；按业务域拆（现有 chat/ git/ codex/），不要 src/
│   ├── ui/                      # 无业务语义；components / lib / styles，不要 src/
│   └── views/                   # 组合层；按业务域拆（现有 chat/ git/ codex/），不要 src/
├── docs/
├── data/                    # 运行时文件（sqlite 等），gitignore
├── server/
│   ├── cmd/server/              # 服务启动、配置、Router 和依赖装配
│   ├── internal/
│   │   ├── handler/             # 大部分 HTTP：CRUD、SSE、Start / Continue / Cancel、记忆查看/删除、Git
│   │   │   └── codex/           # 独立 /codex HTTP 薄桥接
│   │   ├── agent/               # 运行时编排 + sqlc 持久化
│   │   │   ├── memory/          # 热层目录+专题，冷层工作区 FTS 索引
│   │   │   └── tools/           # 具体工具定义：ping、memory_*、编码八工具、plan_*
│   │   ├── codex/               # 本机 app-server 生命周期与内存排队/问票/SSE
│   │   ├── events/              # 进程内事件总线
│   │   ├── config/
│   │   ├── logger/
│   │   ├── errors/
│   │   └── util/
│   ├── pkg/
│   │   ├── agent/               # 全部通用无状态逻辑，含模型调用与 Tool 抽象
│   │   ├── git/                 # 无状态 Git CLI 操作，供 Handler 直接调用
│   │   ├── codex/               # 看板的 Codex 子模块：协议客户端与领域类型
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
  -> internal/handler/codex
  -> internal/codex
  -> internal/agent
  -> internal/events
  -> pkg/db

internal/handler
  -> pkg/db/sqlite.Queries
  -> pkg/agent          # 映射响应、token 统计、Profile 装配
  -> pkg/git            # 本机 Git CLI 操作
  -> internal/agent     # Worker 领取后的 Loop
  -> internal/agent/memory  # 用户侧记忆响应类型

internal/handler/codex
  -> internal/codex
  -> pkg/codex

internal/codex
  -> pkg/codex          # JSONL 协议客户端；不查库
  启动本机 `codex app-server --stdio`
  排队、草稿、问票、SSE 只放内存

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

pkg/git
  不依赖 handler、internal、sqlc
  无状态，只 exec 本机 git；不写产品流程

pkg/codex
  看板的 Codex 子模块：领域类型与 app-server JSONL 协议客户端
  不依赖 handler、internal、sqlc
  不查库、不 spawn `codex`；Transport 由 internal/codex 注入
  不进 pkg/agent

packages/core
  不依赖 React、Next、DOM、process.env、AI SDK
  按业务域拆目录（chat / git / codex），不要 src/
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
  按业务域拆目录，与 core 对齐（现有 chat / git / codex）
  AgentProvider 在包根注入 client + userId；CodexProvider 注入 CodexClient
  ChatPage 在新建会话时选择 Agent / Codex 模式；导航用回调

apps/web
  -> packages/views
  -> packages/core          # 创建 AgentClient
  -> packages/ui            # 引入 tokens.css
  不直接解析 SSE 或 event type
```

## 分层职责

### `internal/handler`

承担大部分接口逻辑：

- Session / Message / Usage / Approval 的增删改查。创建 Session 时在本包冻结 `workspace_id`（工作目录）：用户指定的路径必须是已存在目录，否则 400；未指定（空或 `default`）则 `GIT_REPO`，再否则 cwd。不 import `internal/agent/tools`。新对话选目录由 web 弹出目录浏览框。`sessions.summary` 在首次用户消息写入，列表与详情返回
- 用户侧 TextMemory 的查看与删除（不提供写入，不暴露 message 索引；List 用 user_id / workspace_id，Get/Delete 用 name 默认目录）
- SSE：先按 `afterSeq` / `Last-Event-ID` 回放已落库事件，再 `SubscribeAll` 并按 Session 过滤；客户端断开不取消 Run
- 事件 JSON 回放：`GET /sessions/{id}/event-log`，供前端一次 hydrate，不替代 SSE 直播
- Run 的 Start / Continue / Retry / Cancel 和审批裁决直接在 Handler 中处理，需要执行时再交给 Worker
- 同一 Session 只有一个 active Run：已有 active 时 409。要打断当前轮，先 Cancel 再 Start
- Git HTTP（`/git/*`）：校验 checkout、组响应，直接调用 `pkg/git`。带 `session_id` 时仓库根是该会话冻结的 `workspace_id`；未带则 `GIT_REPO`，再否则 cwd。`GET /git/status` 回 `SiteState` 整局（含 `is_repo`、跟踪、ahead/behind、integrating）

Handler 直接依赖 `*sqlite.Queries`，不经过 Store 接口。Git 带 `session_id` 时只查该 Session 的 `workspace_id`，不经过 Store。

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

- 本包写工具名、入参/出参、schema、默认 Effect 和编排。`ping`、记忆三件、编码八工具（`read`/`write`/`edit`/`ls`/`grep`/`find`/`bash`/`powershell`）、`plan_list`/`plan_read`/`plan_write`
- Execute 与第 1 层路径/计划名校验只通过 `Ports`（可替换 FS/RunCommand）和会话已冻结的工作目录。本包不负责解析或冻结 `workspace_id`，只消费 Handler 写入的路径（或 `Ports.WorkspaceRoot` 进程回落）。目录内走工具默认 Effect；目录外必须审批，Agent 表和 yolo 都不能抬成 allow。批过后才能在目录外执行
- Git 用户操作仍走 HTTP + `pkg/git`，不在本包实现 Git Tool

每个工具只定义入参/出参结构体；执行用 `encoding/json`，给模型的 schema 由 `jsonschema.For` 从类型推断。发给模型的是注册表全量工具；`Profile.Tools.Names` 是可执行绑定。模式规则由 `Build` 注入一条 developer 消息；发给网关时紧跟底座 system，不改底座正文。审批流水线：工具默认+参数校验 → 本 Agent `Names`（未绑定 deny，yolo / 已批准都不能抬）→ Agent `Effects` → `approval`（manual/auto/yolo）；每层只审上一层的 `ask`。一批待批工具对应一条审批，一次提交审完再流转。不 import 父包 `internal/agent`。测试用 Tool 可留在测试文件。

### `pkg/git`

无状态 Git CLI：`Open` / `Status`（`SiteState` 整局）/ Diff / 图 / 暂存提交 / reset / revert / 推拉 / remote / 分支 / worktree / `stash create` 副本 / 冲突读写。不进 `pkg/agent`，不写 HTTP 或产品流程。Workspace / Branch / Undo / 说明 / Agent 快照的产品组合在 Handler。

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

### `pkg/codex`

看板的 Codex 子模块。领域类型与 `codex app-server` JSONL 协议客户端给看板 / `internal/codex` 调用。不查库、不 spawn CLI。`session_id` 即官方 `thread_id`。不进 `pkg/agent`。

### `internal/codex`

本机 `codex app-server --stdio` 生命周期与内存编排：懒启动、握手、崩溃后不重发当前回合。官方 `thread/list/read` 是历史数据源；排队、附件草稿、问票和 SSE 环只驻进程内。未安装或未授权不能拖垮主服务。

### `internal/handler/codex`

独立 `/codex/*` HTTP 薄桥接。不写 Codex 业务表。

### `pkg/db`

统一数据库入口。SQLite 已接入 sqlc；Handler 和运行时直接使用 `*sqlite.Queries`。启动时按文件名顺序应用 `migrations/*.sql`。

### `internal/events`

进程内同步发布订阅总线。`Subscribe` / `SubscribeAll` 返回 unsubscribe，避免 SSE 泄漏监听器。流式事件是通知，重连必须按 `event_seq` 回放已落库事件。

### `packages/core`

跨端无头业务，无 UI。按业务域拆目录，文件直接放在 `packages/core/<domain>/`，不要 `src/`。现有 `chat/`：Session / Message / Run / 审批的 HTTP、SSE、Timeline reducer。Git 前端在 `git/`（`GitClient`，不扩 `AgentClient`）。Codex 前端在 `codex/`（`CodexClient`，不扩 `AgentClient`）。`baseUrl` / `userId` 由调用方注入。不依赖 React。第一版 thinking 用 Run 状态（`queued` / `loading_context` / `running_llm`），不是模型 reasoning token。

### `packages/ui`

无业务语义。不要 `src/`，也不按业务域拆：

- `components/ui/`：Button、Collapsible
- `components/`：Conversation、Message（`MessageResponse` 用 Streamdown 渲染 Markdown）、Reasoning、Tool、Confirmation、PromptInput
- `lib/`、`styles/`：`cn`、JSON 展示、zinc token

不依赖 core，不知道 Session / Run / TimelineItem。

### `packages/views`

组合 core + ui。按业务域拆，与 core 对齐，不要 `src/`。现有 `chat/`：`ChatPage`、侧栏、瀑布、审批、prompt；新建会话可选 Agent 或 Codex 模式。包根 `provider.tsx` 注入 `AgentClient` + `userId`。`ChatPage` 接 `sessionId` 与 `onOpenSession`。Git 在 `git/`：`GitProvider` 只注入 `GitClient`，不进 `AgentContext`。Codex 在 `codex/`：`CodexProvider` 只注入 `CodexClient`，由 `ChatPage` 组合，不单独做 Codex 页。不 import `next/*`。新业务新建目录，不预建 Issue / Task / Review / Workspace。

### `apps/web`

路由、`NEXT_PUBLIC_API_BASE` / `NEXT_PUBLIC_USER_ID`、创建 `AgentClient` / `CodexClient`、包 `AgentProvider` / `CodexProvider`、`router.push`。本机 Web 直连 `:8080`（仅回环 Origin 的 CORS）。Git 页在 `(chat)` 组外的 `/git`，只装配 `GitClient`。Codex 不单独路由，走对话页的 `/` 与 `/s/c/:id`。开发态顶栏（对话 / 仓库）只放 web，views 不知道路径。

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
       -> pkg.Dispatch        # 经流水线后执行已绑定工具
```

## 配置

`LLM_PROVIDER`（`openai` | `fake`，默认 `fake`）、`LLM_MODEL`、`LLM_API_KEY`、`LLM_BASE_URL`。`GIT_REPO` 指向本地仓库根，未设则用进程 cwd（不向上找 `.git`）。未设 `DB_DSN` 时 SQLite 写仓根 `data/codedock.db`，不写 `server/`。`CODEX_BIN` 为本机 Codex CLI（默认 `codex`）。Handler 创建 Run 时写入 `RunConfigSnapshot`，后续 Turn 只读快照。

HTTP 出站领域对象使用 snake_case JSON。Router 只对本地回环 Origin 放行 CORS，便于本机 Web 直连 `:8080`。Web 用 `NEXT_PUBLIC_API_BASE`（默认 `http://localhost:8080`）和 `NEXT_PUBLIC_USER_ID`（默认 `local`）。

修改 Agent 能力或跨端协议时，需要检查契约、取消与终态、流式事件语义以及敏感信息处理。
