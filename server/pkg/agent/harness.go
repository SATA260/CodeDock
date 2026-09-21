package agent

// ApprovalKind 区分工具审批与验证/复审熔断单。
type ApprovalKind string

const (
	ApprovalKindTools    ApprovalKind = "tools"    // 现有工具批次审批
	ApprovalKindVerify   ApprovalKind = "verify"   // 验证失败或环境问题
	ApprovalKindEvaluate ApprovalKind = "evaluate" // 复审驳回超限或说不清
)

// OverrideAction 是人对验证/复审单的三种裁决。
type OverrideAction string

const (
	OverrideAccept OverrideAction = "accept" // 我看过了，就这样结束
	OverrideRetry  OverrideAction = "retry"  // 再让模型试一次
	OverrideAbort  OverrideAction = "abort"  // 回滚并取消
)

// RunHarness 是 Run 上需要跨步持久化的正确性工作流字段。
type RunHarness struct {
	HadSideEffects          bool           `json:"had_side_effects"`
	SnapshotID              string         `json:"snapshot_id,omitempty"`
	SnapshotHead            string         `json:"snapshot_head,omitempty"`
	UntrackedFiles          []string       `json:"untracked_files,omitempty"`
	VerifyRound             int            `json:"verify_round"`
	LastVerifyFingerprint   string         `json:"last_verify_fingerprint,omitempty"`
	LastVerifySummary       string         `json:"last_verify_summary,omitempty"`
	EvaluateRound           int            `json:"evaluate_round"`
	LastEvaluateFingerprint string         `json:"last_evaluate_fingerprint,omitempty"`
	LastEvaluateSummary     string         `json:"last_evaluate_summary,omitempty"`
	WrapUpPending           bool           `json:"wrap_up_pending,omitempty"`
	ApprovalKind            ApprovalKind   `json:"approval_kind,omitempty"`
	OverrideAction          OverrideAction `json:"override_action,omitempty"`
	ActivePlan              string         `json:"active_plan,omitempty"` // 本会话绑定的计划文件名
}

// Harness 抽出当前状态里需要落库的正确性字段。
func (s AgentState) Harness() RunHarness {
	return RunHarness{
		HadSideEffects:          s.HadSideEffects,
		SnapshotID:              s.SnapshotID,
		SnapshotHead:            s.SnapshotHead,
		UntrackedFiles:          append([]string(nil), s.UntrackedFiles...),
		VerifyRound:             s.VerifyRound,
		LastVerifyFingerprint:   s.LastVerifyFingerprint,
		LastVerifySummary:       s.LastVerifySummary,
		EvaluateRound:           s.EvaluateRound,
		LastEvaluateFingerprint: s.LastEvaluateFingerprint,
		LastEvaluateSummary:     s.LastEvaluateSummary,
		WrapUpPending:           s.WrapUpPending,
		ApprovalKind:            s.ApprovalKind,
		OverrideAction:          s.OverrideAction,
		ActivePlan:              s.ActivePlan,
	}
}

// ApplyHarness 把落库的正确性字段写回状态。
func (s *AgentState) ApplyHarness(h RunHarness) {
	if s == nil {
		return
	}
	s.HadSideEffects = h.HadSideEffects
	s.SnapshotID = h.SnapshotID
	s.SnapshotHead = h.SnapshotHead
	s.UntrackedFiles = append([]string(nil), h.UntrackedFiles...)
	s.VerifyRound = h.VerifyRound
	s.LastVerifyFingerprint = h.LastVerifyFingerprint
	s.LastVerifySummary = h.LastVerifySummary
	s.EvaluateRound = h.EvaluateRound
	s.LastEvaluateFingerprint = h.LastEvaluateFingerprint
	s.LastEvaluateSummary = h.LastEvaluateSummary
	s.WrapUpPending = h.WrapUpPending
	s.ApprovalKind = h.ApprovalKind
	s.OverrideAction = h.OverrideAction
	s.ActivePlan = h.ActivePlan
}

// NormalizeApprovalKind 空值按 tools 处理。
func NormalizeApprovalKind(kind ApprovalKind) ApprovalKind {
	if kind == "" {
		return ApprovalKindTools
	}
	return kind
}

// MaxVerifyRounds 返回验证打回上限，未配置时用 3。
func MaxVerifyRounds(limits RunLimits) int {
	if limits.MaxVerifyRounds > 0 {
		return limits.MaxVerifyRounds
	}
	return 3
}

// MaxEvaluateRounds 返回复审打回上限，未配置时用 2。
func MaxEvaluateRounds(limits RunLimits) int {
	if limits.MaxEvaluateRounds > 0 {
		return limits.MaxEvaluateRounds
	}
	return 2
}

// FallbackModel 优先用专用模型，空则回落主模型。
func FallbackModel(preferred, fallback ModelConfig) ModelConfig {
	if stringsTrim(preferred.Provider) == "" && stringsTrim(preferred.Model) == "" {
		return fallback
	}
	if stringsTrim(preferred.Provider) == "" {
		preferred.Provider = fallback.Provider
	}
	if stringsTrim(preferred.Model) == "" {
		preferred.Model = fallback.Model
	}
	if len(preferred.Options) == 0 {
		preferred.Options = fallback.Options
	}
	return preferred
}

func stringsTrim(value string) string {
	for len(value) > 0 && (value[0] == ' ' || value[0] == '\t') {
		value = value[1:]
	}
	for len(value) > 0 && (value[len(value)-1] == ' ' || value[len(value)-1] == '\t') {
		value = value[:len(value)-1]
	}
	return value
}
