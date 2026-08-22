# CodeDock Agent 开发说明

修改 CodeDock 代码前，先阅读 [`docs/architecture.md`](docs/architecture.md)。该文档是当前目录归属和模块边界的依据。

## 目录放置规则

- 服务启动、配置读取、Router 和依赖装配放在 `server/cmd/server`。
- HTTP 和 WebSocket 的解析、校验和响应放在 `server/internal/handler`。
- Agent 契约、Session、Tool、Result 和 Provider 适配放在 `server/pkg/agent`。
- 直接模型调用和模型供应商适配放在 `server/pkg/ai`。
- 数据库结构演进放在 `server/migrations`。
- Web 路由和页面入口放在 `apps/web`。
- 共享后端契约和通用适配器放在 `server/pkg`；不要把具体产品业务流程放在那里。

## 边界规则

- Handler 负责协议边界和请求转发，不直接启动子进程或调用模型。
- Agent 契约不得依赖 React、UI 包或路由框架。
- Provider 的参数、流式输出、Session 恢复和错误映射不得泄漏到 Handler。
- 修改面向客户端的协议时，同时更新服务端协议定义和 Web 端对应的类型与解析。
- 流式事件是通知，不是真实数据源；重连时应重新获取或恢复 Agent 状态。
- 不要预先创建 Issue、Task、Review、Workspace 或其他具体业务域目录。
- 不要为了预设产品形态创建尚未实现的通用目录。
- 不要把产品工作流放入 Agent 通用层或 `server/pkg`。

当需求变更没有明显的代码归属时，先依据 `docs/architecture.md` 对其分类，再开始编写代码。
