package tool

import (
	"fmt"
	"testing"
)

func TestPipelineShortCircuit(t *testing.T) {
	denyInspect := fmt.Errorf("bad args")
	agentDeny := EffectDeny

	cases := []struct {
		name string
		in   PipelineInput
		want Effect
	}{
		{
			name: "inspect deny skips agent deny table",
			in:   PipelineInput{Default: EffectAllow, InspectErr: denyInspect, Bound: true, HasAgentEffect: true, AgentEffect: EffectAllow, Approval: ApprovalYolo},
			want: EffectDeny,
		},
		{
			name: "allow ignores agent deny",
			in:   PipelineInput{Default: EffectAllow, Bound: true, HasAgentEffect: true, AgentEffect: agentDeny, Approval: ApprovalManual},
			want: EffectAllow,
		},
		{
			name: "ask plus agent allow skips approval",
			in:   PipelineInput{Default: EffectAsk, Bound: true, HasAgentEffect: true, AgentEffect: EffectAllow, Approval: ApprovalManual},
			want: EffectAllow,
		},
		{
			name: "ask plus agent deny",
			in:   PipelineInput{Default: EffectAsk, Bound: true, HasAgentEffect: true, AgentEffect: EffectDeny, Approval: ApprovalYolo},
			want: EffectDeny,
		},
		{
			name: "ask plus empty table plus yolo",
			in:   PipelineInput{Default: EffectAsk, Bound: true, Approval: ApprovalYolo},
			want: EffectAllow,
		},
		{
			name: "ask plus empty table plus manual",
			in:   PipelineInput{Default: EffectAsk, Bound: true, Approval: ApprovalManual},
			want: EffectAsk,
		},
		{
			name: "ask plus auto stays ask",
			in:   PipelineInput{Default: EffectAsk, Bound: true, Approval: ApprovalAuto},
			want: EffectAsk,
		},
		{
			name: "already approved",
			in:   PipelineInput{Default: EffectAsk, Bound: true, Approval: ApprovalManual, Approved: true},
			want: EffectAllow,
		},
		{
			name: "outside workspace stays ask even with yolo",
			in:   PipelineInput{Default: EffectAllow, Bound: true, OutsideWorkspace: true, Approval: ApprovalYolo},
			want: EffectAsk,
		},
		{
			name: "outside workspace ignores agent allow",
			in:   PipelineInput{Default: EffectAllow, Bound: true, OutsideWorkspace: true, HasAgentEffect: true, AgentEffect: EffectAllow, Approval: ApprovalYolo},
			want: EffectAsk,
		},
		{
			name: "outside workspace approved executes",
			in:   PipelineInput{Default: EffectAllow, Bound: true, OutsideWorkspace: true, Approval: ApprovalManual, Approved: true},
			want: EffectAllow,
		},
		{
			name: "outside plus inspect error still deny",
			in:   PipelineInput{Default: EffectAllow, InspectErr: denyInspect, Bound: true, OutsideWorkspace: true, Approval: ApprovalYolo},
			want: EffectDeny,
		},
		{
			name: "invalid default becomes ask then yolo",
			in:   PipelineInput{Default: Effect("weird"), Bound: true, Approval: ApprovalYolo},
			want: EffectAllow,
		},
		{
			name: "unbound deny even if tool allow and yolo",
			in:   PipelineInput{Default: EffectAllow, Approval: ApprovalYolo},
			want: EffectDeny,
		},
		{
			name: "unbound deny even if agent allow and approved",
			in:   PipelineInput{Default: EffectAsk, HasAgentEffect: true, AgentEffect: EffectAllow, Approval: ApprovalManual, Approved: true},
			want: EffectDeny,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Pipeline(tc.in); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestBound(t *testing.T) {
	if Bound(nil, "ping") || Bound([]string{"ping"}, "") || !Bound([]string{"ping", "read"}, "read") {
		t.Fatal("bound mismatch")
	}
}
