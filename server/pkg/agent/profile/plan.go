package profile

// Plan 只在工作区 .cursor/ 下读写 markdown 计划。
func Plan() Config {
	return configOf("plan", "6", `这轮只规划：只在工作区 .cursor/ 下读写 markdown 计划。不要改仓库里的其他文件，不要跑 shell，不要写记忆。用户要求改代码时，说明当前是 plan，需要切到 agent。`, []string{
		"只用计划工具改 .cursor/ 下的文件，不要动仓库里的其他文件",
		"文件名用简短的 basename；没有 .md 后缀时会补上",
		"需要做事时再调用工具",
	}, []string{
		"plan_list",
		"plan_read",
		"plan_write",
	})
}
