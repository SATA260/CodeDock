# CodeDock 架构与目录边界

本文档定义 CodeDock 当前的技术骨架。目录按能力拆分，参考 multica 的服务端、共享包和应用分层方式，但不引入具体产品业务域。

CodeDock 是 Agent 项目。当前只保留服务入口、协议边界、Agent 契约和模型适配等通用能力；Issue、Task、Review、Workspace 等业务目录不属于本项目的基础结构。

## 总体架构

```text
Web 客户端
    |
    | HTTP/WebSocket 请求
    v
server/cmd/server
    |
    v
server/internal/handler
    |
    +-- server/pkg/agent
    +-- server/pkg/ai
```

服务端入口负责装配 HTTP 服务和路由，`internal/handler` 负责协议边界，`pkg/agent` 负责 Agent 契约与适配，`pkg/ai` 负责模型调用适配。执行环境、事件分发、持久化等能力只有在真实实现出现时再增加目录。

## 当前目录骨架

```text
CodeDock/
├── apps/
│   └── web/
│       └── app/                   # Web 路由和页面入口
│
├── docs/                          # 架构和开发文档
│
├── server/
│   ├── cmd/
│   │   └── server/                # 服务启动和依赖装配
│   ├── internal/
│   │   └── handler/               # HTTP/WebSocket 协议边界
│   ├── pkg/
│   │   ├── agent/                 # Agent 契约和 Backend/Provider 适配
│   │   └── ai/                    # 直接模型调用和模型适配
│   ├── migrations/                # 数据库结构演进（需要持久化时使用）
│   ├── go.mod
│   └── go.sum
│
└── AGENTS.md
```

不要为了预设产品形态而创建不存在的业务目录或通用层目录。

## 依赖方向

```text
apps/web
    -> server HTTP/WebSocket API

server/cmd/server
    -> server/internal/handler

server/internal/handler
    -> server/pkg/agent / server/pkg/ai
```

- `cmd/server` 只负责配置、基础设施初始化、Router 和依赖装配。
- `internal/handler` 只负责请求解析、上下文建立、Agent 操作转发和响应序列化。
- `pkg/agent` 定义稳定的 Agent Backend、Session、Message、Tool、Cancellation、Usage 和 Result 语义。
- `pkg/ai` 封装不需要完整 Agent 生命周期的直接模型调用和 Provider 差异。
- `pkg` 只放稳定契约和通用适配器，不放具体产品业务流程。

## Agent 边界

### `server/pkg/agent`

负责通用 Agent 契约和不同 Agent CLI 或 Provider 的适配。Provider 参数、流式输出、Session 恢复和错误映射不应泄漏到 Handler。

### `server/pkg/ai`

负责直接模型调用和模型供应商适配。需要完整 Agent 生命周期的能力仍应通过 `pkg/agent` 的统一契约表达。

### `server/internal/handler`

负责 HTTP/WebSocket 协议边界，不直接启动子进程、不直接调用模型，也不承载具体产品业务规则。

## Agent 通用链路

```text
HTTP/WebSocket request
  -> cmd/server
  -> internal/handler
  -> pkg/agent 或 pkg/ai
  -> protocol response / stream event
```

修改 Agent 能力或跨端协议时，需要检查 Agent 契约、Provider 适配、取消与终态处理、流式事件语义以及敏感信息处理。

## 目录放置判断

| 代码职责 | 目录 |
| --- | --- |
| HTTP/WebSocket 协议边界 | `server/internal/handler` |
| Agent 契约或 Provider 适配 | `server/pkg/agent` |
| 直接模型调用 | `server/pkg/ai` |
| 服务启动和依赖装配 | `server/cmd/server` |
| 数据库结构演进 | `server/migrations` |
| Web 路由和页面入口 | `apps/web` |
