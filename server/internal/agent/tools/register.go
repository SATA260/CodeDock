package tools

import (
	"codedock/pkg/agent/tool"
	"codedock/pkg/db/sqlite"
)

// Ports 是 Execute 要调用的外部实现，在 Runtime 初始化时注入。
// 工具名、入参/出参、schema、权限和编排都在本包定义；外部模块只实现这里的接口。
// 会话工作区在创建 Session 时冻结，经 tool.Input.WorkspaceRoot 传入；本字段只是进程级回落。
type Ports struct {
	WorkspaceRoot string      // 进程回落工作目录（通常是 GIT_REPO / cwd）；会话级根优先
	FS            FileSystem  // 可替换文件系统；测试用内存 FS
	RunCommand    CommandFunc // 可替换命令执行；空则走本机
	LookPath      func(file string) (string, error)
}

func (p Ports) executor() *Executor {
	exec := NewExecutor()
	if p.FS != nil {
		exec.FS = p.FS
	}
	if p.RunCommand != nil {
		exec.RunCommand = p.RunCommand
	}
	if p.LookPath != nil {
		exec.LookPath = p.LookPath
	}
	return exec
}

// Register 注册本包定义的工具，并把 Ports 接到对应 Execute。
// q 为 nil 时只注册不依赖存储的工具。
func Register(reg tool.Registry, q *sqlite.Queries, onOverBudget OverBudgetFunc, ports Ports) {
	if reg == nil {
		return
	}
	_ = reg.Register(Ping())
	if q != nil {
		_ = reg.Register(ReadTool(q))
		_ = reg.Register(WriteTool(q, onOverBudget))
		_ = reg.Register(SearchTool(q))
	}
	registerPortTools(reg, ports)
}

// registerPortTools 注册编码与 plan 工具。工作区按会话冻结，不因 Ports 为空而跳过注册。
func registerPortTools(reg tool.Registry, ports Ports) {
	for _, item := range codingTools(ports) {
		_ = reg.Register(item)
	}
	for _, item := range planTools(ports) {
		_ = reg.Register(item)
	}
}
