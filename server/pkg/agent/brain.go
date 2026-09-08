package agent

import "encoding/json"

// Brain 根据当前状态决定下一步做什么，自身不执行 I/O。
type Brain struct{}

// Decide 由唤醒原因（phase）和当前状态推断出本步骤应执行哪些指令。
// 当前为空实现：后续按 phase 返回 call_llm / call_tools_batch / finish 等指令。
func (b *Brain) Decide(phase Phase, payload json.RawMessage, state AgentState) ([]Instruction, error) {
	_ = phase
	_ = payload
	_ = state
	return nil, nil
}
