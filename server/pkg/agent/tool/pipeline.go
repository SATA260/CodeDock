package tool

// PipelineInput 是一次调用进入审批流水线的输入。
type PipelineInput struct {
	Default          Effect       // 工具默认，只能是 allow 或 ask
	InspectErr       error        // 第 1 层参数校验失败
	Bound            bool         // 是否在本 Agent 的 Names 里；未绑定则 deny
	OutsideWorkspace bool         // 路径落在会话工作区外；本会话权限管不到
	AgentEffect      Effect       // 第 2 层覆盖；空表示无记录
	HasAgentEffect   bool         // 表里是否有该工具
	Approval         ApprovalMode // 第 3 层
	Approved         bool         // 审批单已批准本调用
	LockedAsk        bool         // 既有测试文件等：Agent 表 allow 不能抬；yolo 仍放行，正确性靠收尾验证
}

// Pipeline 按工具校验 → Agent 绑定/表 → 审批模式依次裁定。
// 未列入本 Agent Names 的工具直接 deny；工具默认 allow、Effects allow、已批准都不能抬。
// 每一层只处理上一层仍为 ask 的结果；allow / deny 原样传递。
// 工作区外的调用本会话权限管不到：校验通过后强制 ask，Agent 表和 yolo 都不能抬成 allow；独立复审或人批通过后 Approved 才放行。
// 既有测试文件 LockedAsk：manual / auto 必须人批，Agent 表不能抬；yolo 走普通审批层直接放行。
func Pipeline(in PipelineInput) Effect {
	if in.InspectErr != nil {
		return EffectDeny
	}
	if !in.Bound {
		return EffectDeny
	}
	if in.OutsideWorkspace {
		if in.Approved {
			return EffectAllow
		}
		return EffectAsk
	}
	if in.LockedAsk && in.Approval != ApprovalYolo {
		if in.Approved {
			return EffectAllow
		}
		return EffectAsk
	}
	effect := layerTool(in)
	if effect == EffectAsk {
		effect = layerAgent(in)
	}
	if effect == EffectAsk {
		effect = layerApproval(in)
	}
	return effect
}

func layerTool(in PipelineInput) Effect {
	if in.Default == EffectAllow || in.Default == EffectAsk {
		return in.Default
	}
	return EffectAsk
}

func layerAgent(in PipelineInput) Effect {
	if !in.HasAgentEffect {
		return EffectAsk
	}
	switch in.AgentEffect {
	case EffectAllow, EffectAsk, EffectDeny:
		return in.AgentEffect
	default:
		return EffectAsk
	}
}

func layerApproval(in PipelineInput) Effect {
	if in.Approved {
		return EffectAllow
	}
	if in.Approval == ApprovalYolo {
		return EffectAllow
	}
	return EffectAsk
}

// Bound 判断工具名是否在 Agent 绑定列表里。
func Bound(names []string, name string) bool {
	if name == "" {
		return false
	}
	for _, item := range names {
		if item == name {
			return true
		}
	}
	return false
}
