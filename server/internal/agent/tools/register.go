package tools

import (
	"codedock/pkg/agent/tool"
	"codedock/pkg/db/sqlite"
)

// Ports 是 Execute 要调用的外部实现，在 Runtime 初始化时注入。
// 工具名、入参/出参、schema、权限和编排都在本包定义；外部模块只实现这里的接口。
// 某字段为 nil 则不注册对应工具。本阶段没有需要外部实现的工具（不实现文件 / Shell / Git）。
type Ports struct{}

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

// registerPortTools 按已注入的 Port 注册依赖外部实现的工具。
func registerPortTools(reg tool.Registry, ports Ports) {
	_ = reg
	_ = ports
}
