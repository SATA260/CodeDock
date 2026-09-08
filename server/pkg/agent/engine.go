package agent

import "context"

// Engine 执行一步：按 Brain 的指令调用对应执行器，自身不直接写库、不发事件、不调度下一步。
type Engine struct {
	brain *Brain
}

// NewEngine 创建执行引擎。brain 为空时自动构造一个空 Brain。
func NewEngine(brain *Brain) *Engine {
	if brain == nil {
		brain = &Brain{}
	}
	return &Engine{brain: brain}
}

// Step 执行一步：先让 Brain 决策，再按指令类型分发到对应执行器。
// 空实现阶段所有执行器直接返回 nil，仅保留调用关系。
func (e *Engine) Step(ctx context.Context, state AgentState, job StepJob) (StepResult, error) {
	if e == nil {
		return StepResult{}, nil
	}
	instructions, err := e.brain.Decide(job.Phase, job.Payload, state)
	if err != nil {
		return StepResult{}, err
	}
	out := StepResult{State: state}
	for _, in := range instructions {
		switch in.Type {
		case InstructionCallLLM, InstructionLoadContext:
			if err := e.callLLM(ctx, state, in); err != nil {
				return StepResult{}, err
			}
		case InstructionCallToolsBatch:
			if err := e.callToolsBatch(ctx, state, in); err != nil {
				return StepResult{}, err
			}
		case InstructionFinish:
			if err := e.finish(ctx, state, in); err != nil {
				return StepResult{}, err
			}
		default:
			// TODO: compress_context / request_human_approve
		}
	}
	return out, nil
}

func (e *Engine) callLLM(ctx context.Context, state AgentState, in Instruction) error {
	_ = ctx
	_ = state
	_ = in
	// TODO: 加载上下文、压缩、构造 prompt、调用模型流。
	return nil
}

func (e *Engine) callToolsBatch(ctx context.Context, state AgentState, in Instruction) error {
	_ = ctx
	_ = state
	_ = in
	// TODO: 解析 tool_call 批次，串行或并行分发执行。
	return nil
}

func (e *Engine) finish(ctx context.Context, state AgentState, in Instruction) error {
	_ = ctx
	_ = state
	_ = in
	// TODO: 将 Run 置为终态并给出结束原因。
	return nil
}
