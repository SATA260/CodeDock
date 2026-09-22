# CodeDock 架构与目录边界

本文档定义 CodeDock 当前的技术骨架。目录按能力拆分；Issue、Task、Review、Workspace 等业务目录不属于本项目的基础结构。

Agent Loop 已闭环：Handler 写用户消息与 Run，Worker 领取后由 Runtime 装上下文、调模型、执行 Tool，事件先落库再经 Bus 由 SSE 消费。三个内置 Agent（ask / plan / agent）共用同一 Loop；审批走工具 → Agent 表 → 审批模式流水线。模型不再直接结束有副作用的任务：`HadSideEffects` 为真时必须经过 `verifying`，通过后再写面向用户的收尾说明才 `completed`。

## 总体架构

```text
apps/web  (路由 + NEXT_PUBLIC_* + AgentClient / GitClient / CodexClient / ClaudeClient / BoardClient)
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
    |-- CRUD / SSE / Start / Continue / Cancel / Restore / 审批 --> pkg/db/sqlite
    |-- 领取 Run 后的 Loop --> internal/agent
    |-- 用户记忆查看 / 删除 --> pkg/db/sqlite
    |-- Git HTTP --> pkg/git（本机 CLI，无产品流程）
    |-- Claude HTTP --> pkg/claude（本机 Claude Code，不落库）
    |-- 看板 /works /board --> internal/board（Work / Checkout / Placement / Inbox）
    |
    v
server/internal/agent
    |-- sqlc 读写 Run / Turn / 事件 / 用量 / checkpoint
    |-- 调用 pkg/agent 无状态方法
    |-- 先持久化 AgentEvent，再发布 internal/events.Bus
    |-- memory：热层目录+专题（含 work 范围），冷层按工作区 FTS；本回合装 user+Work
    |-- tools：工具定义在本包；New 注入 Ports（Execute 用的外部实现）后 Register
    |
    v
server/pkg/agent
server/pkg/git
server/pkg/github
server/pkg/codex
server/pkg/claude
```

`pkg/ai` 已删除。大模型调用放在 `pkg/agent`，由 `ModelConfig` 在方法内创建，不由 Runtime 注入。

## 当前目录骨架

```text
CodeDock/
├── apps/
│   └── web/                     # Next.js 路由与平台装配；不解析 SSE
├── packages/
│   ├── core/                    # 无头业务；按业务域拆（现有 chat/ git/ codex/ claude/ board/），不要 src/
│   ├── ui/                      # 无业务语义；components / lib / styles，不要 src/
│   └── views/                   # 组合层；按业务域拆（现有 chat/ git/ codex/ claude/ board/），不要 src/
├── docs/
├── plugin/example/          # 插件拷贝模板；不改正文、不换向
├── plugin/redact/           # 脱敏插件；PLUGIN_DIR 指到 plugin/
├── data/                    # 运行时文件（sqlite 等），gitignore
├── server/
│   ├── cmd/server/              # 服务启动、配置、Router 和依赖装配
│   ├── internal/
│   │   ├── handler/             # 大部分 HTTP：CRUD、SSE、Start / Continue / Cancel、记忆查看/删除、Git、看板、Claude Code
│   │   │   └── codex/           # 独立 /codex HTTP 薄桥接
│   │   ├── agent/               # 运行时编排 + sqlc 持久化
│   │   │   ├── memory/          # 热层目录+专题（user / workspace / work），冷层工作区 FTS 索引
│   │   │   └── tools/           # 具体工具定义：ping、memory_*、编码八工具、plan_*、explore
│   │   ├── board/               # Work / Checkout / Info / Placement / Inbox / Packet；不写记忆、不建 worktree
│   │   ├── codex/               # 本机 app-server 生命周期与内存排队/问票/SSE
│   │   ├── events/              # 进程内事件总线
│   │   ├── pluginhost/          # go-plugin 宿主：拉进程、Dispatch、Host 白名单
│   │   ├── config/
│   │   ├── logger/
│   │   ├── errors/
│   │   └── util/
│   ├── pkg/
│   │   ├── agent/               # 全部通用无状态逻辑，含模型调用与 Tool 抽象
│   │   │   └── seam/            # Envelope / Dispatcher / 六个口的类型常量
│   │   ├── plugin/              # 插件 SDK 与 proto；作者只 import 这个包
│   │   ├── git/                 # 无状态 Git CLI 操作，供 Handler 直接调用
│   │   ├── github/              # 无状态 gh CLI：issue/pr view 快照；不落 token、不合 PR
│   │   ├── claude/              # 无状态 Claude Code 对接；从本机 Claude 读，不落库
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
  -> internal/pluginhost
  -> pkg/db

internal/handler
  -> pkg/db/sqlite.Queries
  -> pkg/agent          # 映射响应、token 统计、Profile 装配
  -> pkg/git            # 本机 Git CLI 操作
  -> pkg/github         # 本机 gh Issue/PR 快照
  -> pkg/claude         # 本机 Claude Code 对接
  -> internal/board     # Work / 看板聚合 / Inbox 转交
  -> internal/agent     # Worker 领取后的 Loop
  -> internal/agent/memory  # 用户侧记忆响应类型

internal/handler/codex
  -> internal/codex
  -> pkg/codex

internal/codex
  -> pkg/codex          # JSONL 协议客户端；不查库
  启动本机 `codex app-server --stdio`
  排队、草稿、问票、SSE 只放内存

internal/board
  -> pkg/db/sqlite.Queries
  -> pkg/agent          # 审批 kind 文本
  不写记忆、不 spawn CLI、不建 worktree

internal/agent
  -> pkg/db/sqlite.Queries
  -> pkg/agent
  -> internal/events
  -> internal/agent/memory
  -> internal/agent/tools
  -> internal/board     # native 读 Placement 与 Packet

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

internal/pluginhost
  -> pkg/plugin
  -> pkg/agent / pkg/agent/seam / pkg/agent/tool
  -> internal/agent/memory
  -> pkg/db/sqlite.Queries
  -> internal/events
  不 import 父包 internal/agent

pkg/plugin
  -> pkg/agent / pkg/agent/seam / pkg/agent/tool
  -> pkg/plugin/proto
  不依赖 handler、internal、sqlc
  插件作者只 import 这个包

pkg/agent
  不依赖 handler、internal、sqlc
  不持有包级状态，不查库
  不知道 gRPC / go-plugin
  Tool 包只含接口、Registry、Dispatch，不含具体工具定义
  seam 是叶子包：信封与六个口，agent 与 tool 都能 import

pkg/git
  不依赖 handler、internal、sqlc
  无状态，只 exec 本机 git；不写产品流程

pkg/github
  不依赖 handler、internal、sqlc
  无状态，只 exec 本机 gh issue/pr view；不落 token

pkg/claude
  不依赖 handler、internal、sqlc
  无状态，从本机 Claude 读会话 / 实录 / 配置，不落库；不写产品流程

pkg/codex
  看板的 Codex 子模块：领域类型与 app-server JSONL 协议客户端
  不依赖 handler、internal、sqlc
  不查库、不 spawn `codex`；Transport 由 internal/codex 注入
  不进 pkg/agent

packages/core
  不依赖 React、Next、DOM、process.env、AI SDK
  按业务域拆目录（chat / git / codex / claude / board），不要 src/
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
  按业务域拆目录，与 core 对齐（现有 chat / git / codex / claude / board）
  AgentProvider 在包根注入 client + userId；CodexProvider / ClaudeProvider / BoardProvider 注入各自 Client
  ChatPage 组会话 / 看板两态；导航用回调

apps/web
  -> packages/views
  -> packages/core          # 创建 AgentClient / GitClient / CodexClient / ClaudeClient / BoardClient
  -> packages/ui            # 引入 tokens.css
  不直接解析 SSE 或 event type
```

## 分层职责

### `internal/handler`

承担大部分接口逻辑：

- Session / Message / Usage / Approval 的增删改查。创建 Session 时在本包冻结 `workspace_id`（工作目录）：用户指定的路径必须是已存在目录，否则 400；未指定（空或 `default`）则 `GIT_REPO`，再否则 cwd。不 import `internal/agent/tools`。新对话选目录由 web 弹出系统目录选择框。`sessions.summary` 在首次用户消息写入，列表与详情返回
- 用户侧 TextMemory 的查看与删除（不提供写入，不暴露 message 索引；List 用 user_id / workspace_id / work_id，Get/Delete 用 name 默认目录）
- 看板 HTTP（`/works`、`/board`、`/placements`、`/inbox/decision`、`/session-links`）：调 `internal/board`。Inbox 按引擎转给已有 `/approvals`、`/claude/...`、`/codex/...` 裁决，不把三引擎问票收成同一种结构。Issue/PR 快照走 `pkg/github`
- SSE：先按 `afterSeq` / `Last-Event-ID` 回放已落库事件，再 `SubscribeAll` 并按 Session 过滤；客户端断开不取消 Run
- 事件 JSON 回放：`GET /sessions/{id}/event-log`，供前端一次 hydrate，不替代 SSE 直播
- Run 的 Start / Continue / Retry / Cancel / Restore 和审批裁决直接在 Handler 中处理，需要执行时再交给 Worker。`POST /runs/{id}/restore` 按快照粒度还原工作区。验证熔断单带 `kind=verify`（旧 `evaluate` 单仍可回放），人可 `accept` / `retry` / `abort`
- 编码场景验收（真实模型写盘、取消、进程重启后 Continue）见 [testing-coding.md](testing-coding.md)，不走 fake
- 同一 Session 只有一个 active Run：已有 active 时 409。要打断当前轮，先 Cancel 再 Start
- Git HTTP（`/git/*`）：校验 checkout、组响应，直接调用 `pkg/git`。带 `session_id` 时仓库根是该会话冻结的 `workspace_id`；未带则 `GIT_REPO`，再否则 cwd。`GET /git/status` 回 `SiteState` 整局（含 `is_repo`、跟踪、ahead/behind、integrating）
- Claude Code HTTP（`/claude/*`）：直接调用 `pkg/claude`。会话 / 实录 / 配置从本机 Claude 读，不查库、不落库

Handler 直接依赖 `*sqlite.Queries`，不经过 Store 接口。Git 带 `session_id` 时只查该 Session 的 `workspace_id`，不经过 Store。Git 与 Claude Code 不写产品表。

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

热层是每个 scope 一篇目录（`kind=index`，`name` 固定 `index`）加多篇专题（`kind=topic`）。scope 是 `user` / `workspace` / `work`（`ScopeWork`）。冷层是同一 `workspace_id` 下的 context message FTS。与 Loop 独立：

- 类型、`ByteLen`、`IndexOverBudget`、`ClipIndex`（200 行 / 25KB）
- Agent 侧 TextMemory 的 Get / Upsert / Delete / List
- `SearchMessages` 只查不写；`IndexMessage` 由 Loop 写 message 时调用

不 import 父包；不解析 Markdown；不负责 Prompt / Context Packet / 对话压缩；不定义 Tool。不放在 `pkg`。不新建 Workspace 业务包，只持有 `workspace_id` 字段。Loop 在新 Session / 对话压缩后装 **user + 当前 Work** 冻结目录（有 placement 时）；`workspace` 热层保留但不进本回合。Packet 是独立 system，不入库、不抄进记忆。超限目录立刻写入，由 Runtime 后台 `pkg.CompactIndex` 改短盖写，不自动建专题，不改当前 Session 冻结前缀。未归组会话只能写 user 记忆。

### `internal/agent/tools`

工具定义全部在本包。Runtime `New` 接收 `Ports`（Execute 要调用的外部实现），再 `Register`：

- 本包写工具名、入参/出参、schema、默认 Effect 和编排。`ping`、记忆三件、编码八工具（`read`/`write`/`edit`/`ls`/`grep`/`find`/`bash`/`powershell`）、`plan_list`/`plan_read`/`plan_write`/`plan_pass`、只读 `explore`。`edit`/`write` 写盘后做语法轻检，新硬伤当场回滚；`bash`/`powershell` 改到可检查源文件后同样回滚。已存在测试文件走 LockedAsk（manual / auto 必须人批；yolo 放行，正确性靠 verify）。`plan_write` 只增不删验收项；打勾必须走 `plan_pass` 并提交证据。验收项字段必须是 `id` / `description` / `verify_cmd`，写在工具入参 `items` 或正文的 Markdown 勾选列表里；落盘给人看的是普通 Markdown，不写 JSON frontmatter。会话绑定一份当前计划（`RunHarness.ActivePlan`）：用户未点名时 `plan_list` 不扫目录里的其他计划，`plan_read` / 编码 `read` 不能读未绑定文件
- Execute 与第 1 层路径/计划名校验只通过 `Ports`（可替换 FS/RunCommand）和会话已冻结的工作目录。本包不负责解析或冻结 `workspace_id`，只消费 Handler 写入的路径（或 `Ports.WorkspaceRoot` 进程回落）。目录内走工具默认 Effect；目录外任何模式（含 yolo）都不能直接执行，必须先走独立复审（`EvaluatorModel`，缺省回落主模型），通过才跑，说不清再开人单。Agent 表也不能抬成 allow
- Git 用户操作仍走 HTTP + `pkg/git`，不在本包实现 Git Tool

每个工具只定义入参/出参结构体；执行用 `encoding/json`，给模型的 schema 由 `jsonschema.For` 从类型推断。发给模型的工具表按本轮 `Profile.Tools.Names` 裁过，不再把 ask / plan 用不到的写类工具一并送给网关。`Names` 仍是可执行绑定。模式规则由 `Build` 注入一条 developer 消息；发给网关时紧跟底座 system。审批流水线：工具默认+参数校验 → 本 Agent `Names`（未绑定 deny，yolo / 已批准都不能抬）→ Agent `Effects` → `approval`（manual/auto/yolo）；每层只审上一层的 `ask`。一批待批工具对应一条审批，一次提交审完再流转。不 import 父包 `internal/agent`。测试用 Tool 可留在测试文件。

### 插件

主循环在六个口把当前数据递给 `seam.Dispatcher`：`agent/input`、`agent/pre-step`、`agent/request`、`llm/stream`、`tools/pre-execute`、`tools/post-execute`。没有 Dispatcher 时原样通过。作者侧每个口是一对入参/回包结构体（`OnAgentInput` 的 `AgentInput` / `AgentInputResult` 等），换向是回包字段，不解信封。

多个插件按子目录名排序依次改同一份载荷。回包类型不变则继续；类型变成 `input/handled` / `run/blocked` / `tools/denied` / `tools/ask` 则换方向。同一条链上问过的插件记入 `Seen`，不会再问自己。插件之间的参数走 `PluginContext`（信封 `Context`），由宿主按会话/Run 暂存，不进模型、不进消息表。

插件跑在独立进程里，经 go-plugin gRPC 通信。宿主白名单：`Emit`（不能发六个口的同名事件）、`RegisterMethod`、记忆读写、`Complete`、`AppendNotice`。超时或进程挂了：拦截口按否决，只改数据的口保留原样。`assistant.delta` 不发给插件。已批准的工具不再拦一次。换插件二进制要重启服务。未设 `PLUGIN_DIR` 不拉进程。

详见 [plugin.md](plugin.md)。

### `internal/board`

产品工作流：Work / Info / Placement / Board 聚合 / Inbox 编排 / Packet。一张卡可挂多路会话。目录绑在会话上，不挂在卡上；已有会话可以事后绑定或更换目录。旧会话不自动建卡；可先聊再补挂。删卡只断开归属，不删会话、不删磁盘。看板只聚合摘要，不加载对话正文。目录 Git 状态现问 `pkg/git`。不写记忆正文、不 spawn CLI、不建 worktree。

### `pkg/git`

无状态 Git CLI：`Open` / `Status`（`SiteState` 整局）/ Diff / 图 / 暂存提交 / reset / revert / 推拉 / remote / 分支 / worktree / `stash create` 副本 / 冲突读写。不进 `pkg/agent`，不写 HTTP 或产品流程。Workspace / Branch / Undo / 说明 / Agent 快照的产品组合在 Handler。

### `pkg/github`

无状态本机 `gh` CLI：`issue view` / `pr view` JSON 快照。不关 issue、不合 PR、不落 token。不进 `pkg/agent`，不写产品流程。会话头上的 Issue/PR 由 Handler 写入 `session_issues` / `session_pulls`。

### `pkg/claude`

无状态 Claude Code 对接：引擎探测、会话、配置、斜杠命令、附件、回合、实录、审批，以及对本机 Claude 说话。会话 / 实录 / 模型 / 权限档从本机 Claude 读，不落库。不进 `pkg/agent`，不走本地对话的工具 / 记忆 / 压缩。Handler 直接调用。

### `pkg/agent`

全部 Agent 通用逻辑，方法无状态：

- 领域类型、状态机 `CanTransition`、用量 `CountTokens`（UTF-8 字节 / 4）
- 提示词 `Build`
- 上下文 `Load` / `NeedsCompaction` / `CompactIfNeeded`
- Tool 抽象、内存 `Registry`、无状态 `Dispatch`（不含具体工具定义）
- Agent 配置抽象 `profile.Config` 与 `RunConfigSnapshot`
- 模型调用 `Stream` / 压缩：在函数内按 `ModelConfig.Provider` 创建
  - `fake`：读 `Model.Options` 脚本（多段 text / tool_calls、失败次数、可取消挂起、verify 脚本），测试用
  - `openai`：OpenAI 兼容 HTTP（`BaseURL` + API Key）
- 正确性工作流：`Brain` 在 `llm_result` 且无待批工具时，若 `HadSideEffects` 则发 `verify`，通过后写收尾说明再 `completed`。状态含 `verifying`；旧行仍可能是 `evaluating`，恢复时当验证已通过。事件含 `verify.started` / `verify.result` / `verify.skipped` / `snapshot.skipped`；旧 `evaluate.result` 只清思考态，不再画复审卡片
- `EvaluatorModel` 只用于工具出站审批；`SubagentModel` 是 explore 子代理模型。空则回落主模型
- explore 小循环只绑 `read` / `grep` / `find` / `ls` / `memory_search`，有轮次、工具次数和超时预算
- 会话级计划隔离：未点名不读 `.cursor` 下其他计划

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

跨端无头业务，无 UI。按业务域拆目录，文件直接放在 `packages/core/<domain>/`，不要 `src/`。现有 `chat/`：Session / Message / Run / 审批的 HTTP、SSE、Timeline reducer。Git 前端在 `git/`（`GitClient`，不扩 `AgentClient`）。Codex 前端在 `codex/`（`CodexClient`，不扩 `AgentClient`）。Claude 前端在 `claude/`（`ClaudeClient`，不扩 `AgentClient`）。看板前端在 `board/`（`BoardClient`，不扩 `AgentClient`）。`baseUrl` / `userId` 由调用方注入。不依赖 React。thinking 用 Run 状态（`queued` / `loading_context` / `running_llm` / `verifying`），不是模型 reasoning token。验证另有独立时间线卡片；跳过验证不画卡片。

### `packages/ui`

无业务语义。不要 `src/`，也不按业务域拆：

- `components/ui/`：Button、Collapsible
- `components/`：Conversation、Message（`MessageResponse` 用 Streamdown 渲染 Markdown）、Reasoning、Tool、Confirmation、PromptInput
- `lib/`、`styles/`：`cn`、JSON 展示、zinc token

不依赖 core，不知道 Session / Run / TimelineItem。

### `packages/views`

组合 core + ui。按业务域拆，与 core 对齐，不要 `src/`。现有 `chat/`：`ChatPage` 两态。会话模式是三栏（左侧按 Work 分组、中间对话、右侧 Plan / 文件 / Git）。看板模式藏左栏与中间对话，中间是横向 Work 列（列高滚动、每行最多 3 列、底部分页），完整对话用可拖悬浮窗，右侧窗口栏不加 `chat` 种类。新建会话可选 Local / Codex / Claude，可先聊再补挂。包根 `provider.tsx` 注入 `AgentClient` + `userId`。`ChatPage` 接 `sessionId` / `boardMode` 与导航回调。Git 在 `git/`：`GitProvider` 只注入 `GitClient`。Codex / Claude 同上。看板在 `board/`：`BoardProvider` 只注入 `BoardClient`。不 import `next/*`。新业务新建目录，不预建 Issue / Task / Review / Workspace。

### `apps/web`

路由、`NEXT_PUBLIC_API_BASE` / `NEXT_PUBLIC_USER_ID`、创建 `AgentClient` / `GitClient` / `CodexClient` / `ClaudeClient` / `BoardClient`、包 `AgentProvider` / `GitProvider` / `CodexProvider` / `ClaudeProvider` / `BoardProvider`、`router.push`。本机 Web 直连 `:8080`（仅回环 Origin 的 CORS）。对话页装配 `GitClient` 给右侧 Git 窗口。Codex 走对话页的 `/` 与 `/s/c/:id`，Claude 走 `/` 与 `/s/claude/:id`，看板走 `/board`，`/s/...` 仍是会话模式。

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

`LLM_PROVIDER`（`openai` | `fake`，默认 `fake`）、`LLM_MODEL`、`LLM_API_KEY`、`LLM_BASE_URL`。工具出站审批用 `EVALUATOR_PROVIDER` / `EVALUATOR_MODEL`（及可选 Key/BaseURL）；explore 用 `SUBAGENT_PROVIDER` / `SUBAGENT_MODEL`。空则回落主模型。`GIT_REPO` 指向本地仓库根，未设则用进程 cwd（不向上找 `.git`）。未设 `DB_DSN` 时 SQLite 写仓根 `data/codedock.db`，不写 `server/`。`PLUGIN_DIR` 指向插件根目录，未设则不拉插件进程；`PLUGIN_RPC_TIMEOUT` 默认 `10s`。`CODEX_BIN` 为本机 Codex CLI（默认 `codex`）。Handler 创建 Run 时写入 `RunConfigSnapshot`（含 `EvaluatorModel` / `SubagentModel`），后续 Turn 只读快照。收尾验证跑工作区 `.cursor/verify.yaml`，以及本会话绑定计划里非 `manual` 的 `verify_cmd`。取消已标 `cancel_requested` 后不得再落 `completed`；`verifying` 上的取消立即终态。

HTTP 出站领域对象使用 snake_case JSON。Router 只对本地回环 Origin 放行 CORS，便于本机 Web 直连 `:8080`。Web 用 `NEXT_PUBLIC_API_BASE`（默认 `http://localhost:8080`）和 `NEXT_PUBLIC_USER_ID`（默认 `local`）。

修改 Agent 能力或跨端协议时，需要检查契约、取消与终态、流式事件语义以及敏感信息处理。
