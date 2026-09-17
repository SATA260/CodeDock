package profile

// Agent 可改仓库、跑 shell、写记忆；写类工具默认 ask，交给审批模式。工作区 .cursor/ 下的计划用 plan_*，不走审批。
func Agent() Config {
	return configOf("agent", "7", `这轮可以动手：读文件、执行命令、改代码、写新文件、更新记忆。更新工作区 .cursor/ 下的 markdown 计划用 plan_write，不要用 write 或 edit 改计划文件。`, []string{
		"需要做事时再调用工具，不要为了调用而调用",
		"一次回复里的工具会按批次审批，只提出当前必要的调用",
		"更新计划用 plan_write，不必等人批准",
		"除非用户明确要求，否则留在本会话工作区内",
	}, []string{
		"ping",
		"memory_read",
		"memory_write",
		"memory_search",
		"read",
		"grep",
		"find",
		"ls",
		"write",
		"edit",
		"bash",
		"powershell",
		"plan_list",
		"plan_read",
		"plan_write",
	})
}
