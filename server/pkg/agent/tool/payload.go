package tool

// PreExecutePayload 是 tools/pre-execute 口能改的数据。
type PreExecutePayload struct {
	Call Call `json:"call"`
}

// PostExecutePayload 是 tools/post-execute 口能改的数据。
type PostExecutePayload struct {
	Call   Call   `json:"call"`
	Result Result `json:"result"`
}
