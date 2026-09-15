# 编写 CodeDock 插件

外来程序可以在对话的六个口上改数据或换方向，也可以给模型登记方法。插件跑在独立进程里，通过 gRPC 和宿主说话。

没有 `PLUGIN_DIR` 时，服务不拉任何插件，主循环与现在完全一样。

## 从 hello 模板开始

`example/hello` 是作者拷贝的模板。`example/redact` 是脱敏示例：不拦工具，只在 input / request / post-execute 把秘密换成占位符。六个口要哪些字段，以 `codedock/pkg/plugin` 里的结构体为准（`AgentInput`、`AgentInputResult` 等），跳进类型就能看到。

```sh
mkdir -p data/plugins/hello
(cd example/hello && go build -o ../../data/plugins/hello/hello .)
PLUGIN_DIR=$PWD/data/plugins pnpm dev:api
```

脱敏示例：

```sh
mkdir -p data/plugins/redact
(cd example/redact && go build -o ../../data/plugins/redact/redact .)
PLUGIN_DIR=$PWD/data/plugins pnpm dev:api
```

然后对一个会话：

| 你发的 | 能看到的 |
| --- | --- |
| `world` | 用户消息变成 `[hello] world`，开跑前多一条隐藏提示，模型能看到 `hello` 方法 |
| `/skip` | 接口 `handled: true`，不建 Run |
| 工具参数含 `forbidden` | 该次调用被否决（hello） |
| 正文或工具回包含 `AKIA…` / `API_KEY=…` | 落库和发给模型的是占位符（redact） |

复制 `example/hello`，改 `go.mod` 模块名、订阅和策略。作者只 import `codedock/pkg/plugin`。`go.mod` 用 `replace` 指到本仓 `server/`。

目录约定：

```text
PLUGIN_DIR/
  hello/
    hello    # 与子目录同名的二进制
  redact/
    redact
```

## 六个口

每个口是一对 SDK 结构体，实现对应方法即可。没实现的口原样通过。字段含义写在结构体上。

| 口 | 方法 | 入参 | 回包 |
| --- | --- | --- | --- |
| `agent/input` | `OnAgentInput` | `AgentInput`（正文、模式） | `AgentInputResult`；`Handle()` 不建 Run |
| `agent/pre-step` | `OnAgentPreStep` | `AgentPreStep`（系统提示、隐藏消息） | `AgentPreStepResult`；`Block()` 取消本轮 |
| `agent/request` | `OnAgentRequest` | `AgentRequest` | `AgentRequestResult`；只能 `Reply()` |
| `llm/stream` | `OnLLMStream` | `LLMStream`（请求头、请求体） | `LLMStreamResult`；只能 `Reply()`；fake 模型不插 |
| `tools/pre-execute` | `OnToolPreExecute` | `ToolPreExecute`（工具调用） | `ToolPreExecuteResult`；`Deny()` 当失败；`AskApproval()` 进审批 |
| `tools/post-execute` | `OnToolPostExecute` | `ToolPostExecute`（工具结果） | `ToolPostExecuteResult`；只能 `Reply()` |

账本通知走 `OnLedgerNotify`，没有换向。多个插件按子目录名排序，后一个看到前一个改完的结果。同一条链上问过的插件记在 `Seen` 里，不会再问自己。

跨口、跨插件传参数用各口上的 `Context`（`Set("hello.xxx", v)` / `Get`）。这是宿主暂存的 JSON 对象，不进模型、不进消息表、不换向。键建议 `插件名.字段`。`agent/input` 时按会话挂；建 Run 后迁到该 Run。`Handle()` 或 Run 终态会清掉。上限 8KB，超了保留上一份。进程重启即丢。不要把协议塞进 `Hidden`。

隐藏提示用 `sdk.HiddenText("...")` 加进 `AgentPreStep.Hidden`。已批准但还没执行的工具不再走 `OnToolPreExecute`。流式增量 `assistant.delta` 不发给插件。

## Host 白名单

`Bootstrap` 拿到的 `Host` 只能做这些事：

- `Emit`：另发一条与当前口无关的事件。不能发六个口的同名事件。
- `RegisterMethod`：给模型加方法。不能覆盖 `ping`、`memory_read`、`memory_write`、`memory_search`。
- `MemoryGet` / `MemoryUpsert`：按会话读写一篇专题记忆。
- `Complete`：自己打一次模型，不进当前助手流。
- `AppendNotice`：写一条用户看得见的 system 消息（本期只落库，不实时推送）。

## 失败与超时

`PLUGIN_RPC_TIMEOUT` 默认 `10s`。超时或进程挂了：

- `agent/input`、`agent/pre-step`、`tools/pre-execute` 按否决处理
- `agent/request`、`llm/stream`、`tools/post-execute` 保留原数据

进程崩了不会自动拉起。换二进制要重启服务。

## 环境变量

| 变量 | 含义 |
| --- | --- |
| `PLUGIN_DIR` | 插件根目录。每个子目录一个常驻进程。未设则不加载。 |
| `PLUGIN_RPC_TIMEOUT` | 单次 RPC 超时，如 `10s`。 |
