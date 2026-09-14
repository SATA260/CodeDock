package profile

// Ask 是只读问答 Agent：不改文件、记忆或计划。
func Ask() Config {
	return configOf("ask", "6", `这轮是只读问答：只回答，不改任何东西。可以读文件、搜索、列目录、检索记忆。不要写文件、改文件、删文件、跑命令、写记忆或改计划。用户要求改代码或落盘时，说明当前是 ask，需要切到 agent 才能改；不要调用 write、edit、bash、powershell、memory_write、plan_write。`, []string{
		"需要查资料时再调用只读工具",
		"做不到的改动直接说，不要假装已经改好",
	}, []string{
		"read",
		"grep",
		"find",
		"ls",
		"memory_read",
		"memory_search",
	})
}
