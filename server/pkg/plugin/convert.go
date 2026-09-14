package plugin

import (
	"encoding/json"

	"codedock/pkg/agent/seam"
	"codedock/pkg/agent/tool"
	pluginpb "codedock/pkg/plugin/proto"
)

// envelopeFromPB 把 proto 信封转成 seam.Envelope。
func envelopeFromPB(in *pluginpb.Envelope) seam.Envelope {
	if in == nil {
		return seam.Envelope{}
	}
	return seam.Envelope{
		Type:      in.GetType(),
		ChainID:   in.GetChainId(),
		Seen:      append([]string(nil), in.GetSeen()...),
		SessionID: in.GetSessionId(),
		RunID:     in.GetRunId(),
		TurnID:    in.GetTurnId(),
		Payload:   json.RawMessage(in.GetPayload()),
		Context:   json.RawMessage(in.GetContext()),
	}
}

// envelopeToPB 把 seam.Envelope 转成 proto 信封。
func envelopeToPB(in seam.Envelope) *pluginpb.Envelope {
	return &pluginpb.Envelope{
		Type:      in.Type,
		ChainId:   in.ChainID,
		Seen:      append([]string(nil), in.Seen...),
		SessionId: in.SessionID,
		RunId:     in.RunID,
		TurnId:    in.TurnID,
		Payload:   []byte(in.Payload),
		Context:   []byte(in.Context),
	}
}

// methodFromPB 把 proto 方法描述转成 Method。
func methodFromPB(in *pluginpb.Method) Method {
	if in == nil {
		return Method{}
	}
	return Method{
		Name:             in.GetName(),
		Prompt:           in.GetPrompt(),
		ParametersSchema: json.RawMessage(in.GetParametersSchema()),
		Capabilities:     append([]string(nil), in.GetCapabilities()...),
		RequiresApproval: in.GetRequiresApproval(),
	}
}

// methodToPB 把 Method 转成 proto 方法描述。
func methodToPB(in Method) *pluginpb.Method {
	return &pluginpb.Method{
		Name:             in.Name,
		Prompt:           in.Prompt,
		ParametersSchema: []byte(in.ParametersSchema),
		Capabilities:     append([]string(nil), in.Capabilities...),
		RequiresApproval: in.RequiresApproval,
	}
}

// methodInputFromPB 把 proto 方法入参转成 MethodInput。
func methodInputFromPB(in *pluginpb.MethodInput) MethodInput {
	if in == nil {
		return MethodInput{}
	}
	return MethodInput{
		SessionID: in.GetSessionId(),
		RunID:     in.GetRunId(),
		TurnID:    in.GetTurnId(),
		CallID:    in.GetCallId(),
		Name:      in.GetName(),
		Arguments: json.RawMessage(in.GetArguments()),
	}
}

// methodInputToPB 把 MethodInput 转成 proto 方法入参。
func methodInputToPB(in MethodInput) *pluginpb.MethodInput {
	return &pluginpb.MethodInput{
		SessionId: in.SessionID,
		RunId:     in.RunID,
		TurnId:    in.TurnID,
		CallId:    in.CallID,
		Name:      in.Name,
		Arguments: []byte(in.Arguments),
	}
}

// methodResultFromPB 把 proto 方法结果转成 MethodResult。
func methodResultFromPB(in *pluginpb.MethodResult) MethodResult {
	if in == nil {
		return MethodResult{}
	}
	return MethodResult{
		Success: in.GetSuccess(),
		Output:  json.RawMessage(in.GetOutput()),
		Error:   in.GetError(),
	}
}

// methodResultToPB 把 MethodResult 转成 proto 方法结果。
func methodResultToPB(in MethodResult) *pluginpb.MethodResult {
	return &pluginpb.MethodResult{
		Success: in.Success,
		Output:  []byte(in.Output),
		Error:   in.Error,
	}
}

// MethodToDefinition 把插件方法映射成工具定义。
func MethodToDefinition(m Method) tool.Definition {
	caps := make([]tool.Capability, 0, len(m.Capabilities))
	for _, item := range m.Capabilities {
		if item == "" {
			continue
		}
		caps = append(caps, tool.Capability(item))
	}
	return tool.Definition{
		Name:             m.Name,
		Prompt:           m.Prompt,
		ParametersSchema: m.ParametersSchema,
		Permission: tool.Permission{
			Capabilities:     caps,
			RequiresApproval: m.RequiresApproval,
		},
	}
}
