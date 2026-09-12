# CodeDock Agent 开发说明

修改 CodeDock 代码前，先阅读 [`docs/architecture.md`](docs/architecture.md)。该文档是当前目录归属和模块边界的依据。

Agent Loop 已闭环：用户发文本、装上下文、调模型、产出文字或 Tool、事件落库并由 SSE 消费。默认注册 `ping`、记忆工具、编码八工具与 `plan_*`。Git 用户操作走 HTTP + `pkg/git`，不经过 Agent Tool。仓库根是当前会话冻结的工作目录（请求带 `session_id`）；未带会话才回落 `GIT_REPO` / cwd。前端 Git 在 `packages/core/git`、`packages/views/git` 与 `apps/web` 的 `/git`，不扩 `AgentClient`。

## 目录放置规则

- 服务启动、配置读取、Router 和依赖装配放在 `server/cmd/server`。
- 大部分 HTTP 逻辑放在 `server/internal/handler`：Session / Message / Usage / Approval 的 CRUD，SSE，Run 的 Start / Continue / Cancel，审批裁决，用户侧记忆查看/删除，以及 Git（直接调 `pkg/git`）。创建 Session 时在本包冻结 `workspace_id`。不 import `internal/agent/tools`。新对话选目录由 web 弹出目录浏览框（`apps/web` 列本机目录），不走 Agent Tool。
- Agent 运行时编排和 sqlc 持久化放在 `server/internal/agent`。
- Markdown 记忆（热层目录+专题）与 context message 索引（冷层按工作区 FTS）放在 `server/internal/agent/memory`；不放 `pkg/memory`。memory 不 import 父包 `internal/agent`，不定义 Tool。
- 具体工具定义放在 `server/internal/agent/tools`。工具名、入参/出参、schema、权限和编排都在本包；Execute 若要调外部能力，只通过 `Ports` 里的接口。Runtime `New` 时由 `cmd/server` 注入 `Ports` 的具体实现，再 `Register`。每个工具只定义入参/出参结构体，执行用 `encoding/json`，schema 从类型推断。`tools` 可 import `memory`，不 import 父包 `internal/agent`。
- Agent 通用无状态逻辑放在 `server/pkg/agent`：类型、token 统计、提示词、上下文、Tool 抽象（不含具体工具定义）、Agent 配置、模型调用。
- Git CLI 操作放在 `server/pkg/git`：无状态，不写产品流程；Handler 直接调用。不进 `pkg/agent`。
- 进程内事件总线放在 `server/internal/events`。
- 数据库入口和 sqlc 生成代码放在 `server/pkg/db`。
- 数据库结构演进放在 `server/migrations`。
- 无头业务放在 `packages/core`（`@codedock/core`）：按业务域拆（现有 `chat/`、`git/`），文件直接在域目录下，不要 `src/`。不依赖 React、Next、DOM、`process.env`。`baseUrl` / `userId` 由调用方注入。Git 用独立 `GitClient`。
- 无业务 UI 放在 `packages/ui`（`@codedock/ui`）：`components/`、`lib/`、`styles/`，不要 `src/`，不按业务域拆。不依赖 core，不知道 Session / Run / TimelineItem。
- 组合层放在 `packages/views`（`@codedock/views`）：按业务域拆，与 core 对齐（现有 `chat/`、`git/`）。包根 `provider.tsx` 注入 Agent client；Git 用 `views/git` 的 `GitProvider`。不 import `next/*`；导航用回调。不要 `src/`，不预建空业务域。
- Web 路由和平台装配放在 `apps/web`：读 `NEXT_PUBLIC_*`、创建 `AgentClient` / `GitClient`、包对应 Provider、`router.push`。`/git` 放在 `(chat)` 组外。开发态切页顶栏只放 web。不解析 SSE。
- 依赖方向：`apps/web` → `packages/views` → `packages/core`；`packages/views` → `packages/ui`。`ui` 不依赖 `core`。未来 CLI 只依赖 `core`。
- 不要创建 `server/pkg/ai`。大模型调用属于 `pkg/agent`。

## 边界规则

- `pkg/agent` 不持有包级状态，不查库，不依赖 handler 或 internal。
- 模型由 `pkg/agent` 在 `Stream` / `CompactIfNeeded` 内按 `ModelConfig` 创建：`provider=fake` 走脚本化假模型（测试用），`provider=openai` 走 OpenAI 兼容 HTTP。不由 Runtime 注入模型实例。
- Handler 直接使用 `*sqlite.Queries` 做 CRUD、SSE 回放、Run 的 Start / Continue / Cancel 和审批裁决；领取后的 Loop 才进入 `internal/agent`。
- `internal/agent` 负责把 pkg 的计算结果持久化为 Run、Turn、消息、用量和事件。`AgentEvent` 必须先同事务写入并递增 `sessions.last_event_seq`，提交后再 `events.Bus.Publish`。
- 工具定义都在 `internal/agent/tools`（`ping`、记忆、编码八工具、`plan_*`）。外部模块只实现 `Ports` 上的接口，由 `cmd/server` 在初始化时注入。三个内置 Agent（ask / plan / agent）各自一份 `Profile`：底座 system 与发给模型的工具表共用全量注册表，`mode` 冻进 Run 的是可执行 `Names` 与一条 developer 模式规则（发给网关时紧跟底座 system）。工作区在创建 Session 时冻结到 `workspace_id`（用户指定的已存在目录；未指定则 `GIT_REPO` / cwd。指定了但不存在则创建失败）；本会话权限只覆盖该目录，目录外的文件操作必须审批（Agent 表和 yolo 都不能直接放行）。审批是流水线：工具默认+参数校验 → 本 Agent `Names`（未绑定 deny，yolo 不能抬）→ Agent `Effects` 表 → `approval`（manual / auto / yolo）；每层只审上一层的 `ask`，任一层 `deny` 则不可调用。`auto` 开单后由独立复审模型填单，失败则升级给人。一次模型回复的待批 Tool 合成一条审批；拒绝或单个工具失败不打死 Run。Git 用户操作仍走 HTTP + `pkg/git`，不经过 Agent Tool。
- `internal/agent/memory` 负责 TextMemory 的 Get / Upsert / Delete / List、`SearchMessages` 和 `IndexMessage`；用户侧只看/删目录与专题。不负责 Prompt / Context Packet / 对话压缩，不自动建专题，不定义 Tool。Loop 在新 Session / 对话压缩后装冻结目录；写 message 时 `IndexMessage`。超限目录由 Runtime 后台 `CompactIndex` 改短盖写，不改当前 Session 冻结前缀。
- 不要使用 Store 接口包装 sqlc。
- Agent 契约不得依赖 React、UI 包或路由框架。
- 流式事件是通知，不是真实数据源；重连时应按 `event_seq` 回放。
- 不要预先创建 Issue、Task、Review、Workspace 或其他具体业务域目录。
- 不要把产品工作流放入 `server/pkg`。
- 前端三层不得反依赖：`core` 不依赖 React / Next / DOM / `process.env`；`ui` 不依赖 `core`；`views` 不 import `next/*`；`apps/web` 只做路由与平台装配。
- 前端按业务域拆模块，不要 `src/`：`core` / `views` 用同名域目录（现有 `chat`）；`ui` 只用 `components` / `lib` / `styles`。新业务再建目录，不预建空文件夹。

当需求变更没有明显的代码归属时，先依据 `docs/architecture.md` 对其分类，再开始编写代码。
