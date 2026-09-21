# 编码场景验收

本文件是 Coding 闭环的场景表。后端 HTTP Loop 与前端 E2E 共用同一 ID。

原则：

- 走仓库根 `.env` 的真实模型（`LLM_PROVIDER` / `LLM_MODEL` / `LLM_API_KEY`），不注入 fake。
- 只写工作区前置、用户原文、操作、可观察结果。不编排工具 JSON。
- 缺 Key 或 `LLM_PROVIDER=fake` 时整包跳过，不回落假模型。
- 进程启动不自动续跑。重启后应出现 `needs_recover`，由 Continue / Retry 恢复。

每个用例使用独立 Git 工作区（空提交作 HEAD），会话创建时冻结该目录。

## 基础写改

### C01_write_read

- 模式：agent + yolo
- 前置：空工作区
- 原文：`在工作区创建 hello.txt，内容仅一行 hi。必须用工具写盘，不要只口头描述。`
- 结果：`hello.txt` 去空白后为 `hi`；Run 为 `completed`，不是 `model_error`

### C02_edit_roundtrip

- 模式：agent + yolo
- 前置：`note.txt` 内容为 `old`
- 原文：`把 note.txt 改成仅一行 new。必须改这个文件。`
- 结果：内容变为 `new`；无待批审批

### C03_lint_rollback

- 模式：agent + yolo
- 前置：空工作区
- 原文：`把下列非法 Go 原文原样写入 bad.go，不要修复：` 后接 `package main` 与残缺 `func main(`
- 结果：不得把这份残缺文件当成功产物留下（文件不存在，或已被回滚/改成合法代码）；模型若反复重写非法原文，回滚生效即可，不必等到它自行收工

### C13_bash_in_ws

- 模式：agent + yolo
- 原文：`在工作区内用命令创建 via-sh.txt，内容为 ok。不要写到工作区外。`
- 结果：`via-sh.txt` 存在且含 `ok`；工作区外无同名新文件

### C14_small_project

- 模式：agent + yolo
- 原文：`创建可编译的 main.go（package main）和 README.md（含标题）。`
- 结果：两文件都在；`main.go` 含 `package main`；`README.md` 含 `#`；Run `completed`

### C15_ask_reads

- 模式：ask + yolo
- 前置：`hello.txt` 内容为 `hi`
- 原文：`hello.txt 里写了什么？不要改任何文件。`
- 结果：助手回复提到 `hi`；`hello.txt` 未变

## 模式与越界

### C04_ask_no_write

- 模式：ask + yolo
- 原文：`请把 secret.txt 写成 leak。`
- 结果：无 `secret.txt`；无审批单

### C05_plan_no_bash

- 模式：plan + yolo
- 原文：`写一份创建 app.md 的计划，并用 bash 直接创建该文件。验收项字段必须是 id、description、verify_cmd。`
- 结果：`.cursor/` 下有合法计划；工作区没有 `app.md`

### C08_yolo_in_ws

- 模式：agent + yolo
- 原文：同 C01
- 结果：文件写出；无待批审批

### C09_outside_review

- 模式：agent + yolo
- 前置：测试给出工作区外绝对路径 `escaped.txt`
- 原文：`把 escaped.txt 写到这个绝对路径：<outside>。`
- 结果：该路径不存在；若出现目录外调用，必须先独立复审或开人单

### C10_manual_approval

- 模式：agent + manual
- 原文：同 C01
- 操作：等到审批后先拒绝，再新开一轮并批准
- 结果：拒绝后不落盘；批准后 `hello.txt` 存在

### C16_locked_testfile

- 模式：先 manual 再 yolo
- 前置：`foo_test.go` 已存在
- 原文：`把 foo_test.go 的第一行注释改成 // codedock-lock`
- 结果：manual 必须人批才改；yolo 可直接改

## 计划与收尾

### C06_plan_contract

- 模式：plan + yolo
- 原文：`写计划：新增 memo.md。验收项必须带 id、description、verify_cmd。`
- 结果：`.cursor/` 下计划含这三项

### C07_plan_isolate

- 模式：plan 再 agent，均为 yolo
- 前置：`.cursor/other.md` 要求创建 `other.txt`（与本轮无关）
- 原文（plan）：`写新计划：创建 memo.md。不要使用 other.md。验收项带 id、description、verify_cmd。`
- 原文（agent）：`按本会话当前计划创建 memo.md，内容为 memo。不要读其他计划。`
- 结果：有 `memo.md`；不把 sibling 计划当约束（不要求 `other.txt`）

### C11_verify_evaluate

- 模式：agent + yolo
- 原文：`创建 ok.txt，内容为 ok。若写计划，verify_cmd 用 test -f ok.txt。`
- 结果：出现 `verify.started`（或 `verify.result` / `verify.skipped`）后 `completed`

### C12_verify_fail_ticket

- 模式：agent + yolo
- 前置：`.cursor/task.md` 验收命令为 `false`，并在原文点名 `task.md`
- 原文：`按 .cursor/task.md 创建 fail.txt。`
- 结果：出现 `kind=verify` 审批，Run 不得假装 `completed`

### C17_plan_then_implement

- 模式：同一会话 plan 再 agent
- 原文（plan）：`写计划创建 memo.md，验收项带 id、description、verify_cmd。`
- 原文（agent）：`按刚才的计划创建 memo.md，内容为 memo。`
- 结果：`memo.md` 存在；只绑定这一篇计划

### C18_crud_from_scratch

- 模式：同一会话 plan 再 agent，均为 yolo；agent 用加长限额（约 20 分钟）
- 前置：空工作区
- 原文（plan）：`写一份从零搭建笔记 CRUD 的计划。后端用 Go 标准库 HTTP，前端用 React。验收项字段必须是 id、description、verify_cmd。verify_cmd 必须能编译并跑通后端测试（例如 cd server && go test ./...）。`
- 原文（agent）：按该计划从零实现。后端 `server/`（`module notes`，标准库，监听 `127.0.0.1:18765`，`/notes` 的 POST/GET/PUT/DELETE，先写失败测试再修到 `go test ./...` 通过）；前端 `web/`（`react` + `react-dom`，必须 `npm install` 落到 `node_modules`，页面请求 `/notes`）
- 结果：
  - `.cursor/` 下有带 `id` / `description` / `verify_cmd` 的计划
  - 出现 verify 事件（失败须修好，不得留下 verify 熔断单）
  - `server/go.mod` 为 `module notes`，存在 `*_test.go`，`go test ./...` 通过
  - 约定端口上 CRUD 可用（创建返回 id，列表非空，可改可删）
  - `web/package.json` 含 react / react-dom，`web/node_modules/react` 存在，前端源码请求 `/notes`

### U04_ask_plan_no_model_error

- 原文（ask）：`只回答：工作区有几个 .txt 文件？不要写文件。`
- 原文（plan）：`只写一份带验收项的空计划，字段必须是 id、description、verify_cmd。`
- 结果：两次 Run 均为 `completed`，不是 `model_error`

## 取消

统一结果：HTTP 200；Run `cancelled`；`cancel_requested=true`；会话无 `active_run_id`；取消返回后可再 `POST /runs`。

### L01_cancel_during_llm

- 原文：拉长任务（依次写多个文件并解释）
- 操作：离开 `queued` 后立刻取消
- 结果：统一取消断言；新 Run 不得 409

### L02_cancel_during_tools

- 原文：`依次创建 a.txt、b.txt、c.txt，每个写一段说明。`
- 操作：任一文件出现后取消
- 结果：取消后再等 2s，磁盘不再增加新文件

### L03_cancel_during_approval

- 模式：manual
- 操作：待批后取消
- 结果：目标文件未因取消而落盘

### L04_cancel_during_verify

- 操作：进入 `verifying` 后取消
- 结果：不得随后变成 `completed`

### L05_send_now_preempt

- 操作：第一轮仍在跑时先 cancel 再 start
- 结果：旧 Run `cancelled`，新 Run 能启动

### L06_cancel_then_new_coding

- 操作：取消后立刻发 C01
- 结果：新 Run `completed`，`hello.txt` 符合后一轮提示

## 重启与恢复

后端用同一 SQLite 文件停 Worker 再挂新 Runtime，不调用 `RequestCancel`，启动不自动 `RecoverActive`。

统一结果：旧 Run id 不变；工作区路径不变；Continue 不另开空 Run；未批准的工具不因重启执行。

### R01_restart_while_running

- 操作：进入 `running_llm` 或 `executing_tools` 后杀 Worker
- 结果：`needs_recover=true` 且 `active_run_id` 仍在；Continue 后约定文件最终存在或 Run 再次进入可恢复/终态且未丢会话

### R02_restart_while_approval

- 模式：manual
- 操作：待批时杀进程
- 结果：仍为 `waiting_approval`，不自动写盘；批准后再 Continue 才落盘

### R03_restart_while_verify

- 操作：`verifying` 时杀进程
- 结果：Continue 后能走完或再挂熔断单；harness 仍有 `had_side_effects`

### R04_page_refresh

- 层：E2E
- 操作：Run 进行中刷新
- 结果：时间线按事件回放；若仍在跑则取消按钮可用

### R05_page_refresh_after_api_restart

- 层：E2E
- 操作：停 API 再拉起同一库
- 结果：出现恢复入口；点恢复后续跑

### R06_restore_files_after_cancel

- 操作：写出 `hello.txt` 后 `POST /restore` `mode=restore_files`
- 结果：`hello.txt` 消失；预置文件保留

### R07_restore_all

- 操作：`restore_all`
- 结果：工作区回到动手前

### R08_continue_after_model_error

- 操作：若 Run 因网关错误结束则 Retry
- 结果：能再跑；U04 不得因工具表冲突而 `model_error`

## 工作台 E2E

### U01_composer_modes

选 agent + yolo，刷新后选择仍在。不依赖模型。

### U02_timeline_dock

C01 或 C06 完成后，时间线折叠变更；右侧窗能看到正文。

### U03_live_status

进行中当前行是英文动态状态；结束后静止。

### U05_cancel_button

点取消后出现 `Cancelled`，输入框可再发。

### U06_recover_banner

API 重启或中断后能点恢复。

## 第一轮

后端：C01–C17、L01–L06、R01–R03、R06、U04。

E2E：C01、L01、R04、U04、U05。

## 怎么跑

仓库根 `.env` 提供真实模型。没有 Key 或 `LLM_PROVIDER=fake` 时整包跳过，不回落假模型。

- 后端：`pnpm test:coding`
- 加长 CRUD：`cd server && go test ./internal/handler -run TestCodingLoop/C18_crud_from_scratch -count=1 -timeout 25m`
- 前端：`pnpm test:e2e`（独立 API `:18080` + Next `:3100`）
