package profile

// Agent 可改仓库、跑 shell、写记忆；写类工具默认 ask，交给审批模式。工作区 .cursor/ 下的计划用 plan_*，不走审批。
func Agent() Config {
	return configOf("agent", "12", `这轮可以动手：读文件、执行命令、改代码、写新文件、更新记忆。更新工作区 .cursor/ 下的 markdown 计划用 plan_write，不要用 write 或 edit 改计划文件。验收项只能用 plan_pass 打勾，并提交测试或工具证据。写计划时给人看的是 Markdown；验收项优先填 items（id、description、verify_cmd），或在正文写勾选列表，不要写 JSON frontmatter。大范围找定义、约定或超过 200 行的文件结构时用 explore；已经知道文件和位置时直接 read。用户没点名已有计划时，不要去读 .cursor 下的其他计划。`, []string{
		"需要做事时再调用工具，不要为了调用而调用",
		"一次回复里的工具会按批次审批，只提出当前必要的调用",
		"停手且不再调用工具时，先写完成了什么任务、改了什么、怎么验证，不要一一列举改动文件，只做总结",
		"更新计划用 plan_write，不必等人批准",
		"plan_write 把计划写成 Markdown 勾选列表；items 字段名是 id、description、verify_cmd",
		"用户没有点名已有计划时，不要列出或读取其他计划，只动本会话这一篇",
		"业务验收项打勾必须走 plan_pass 并带上证据，不要改清单删要求",
		"多文件查找或了解大文件结构用 explore；精确编辑用 read",
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
		"plan_pass",
		"explore",
	})
}
