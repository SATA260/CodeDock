package agent

import (
	"encoding/json"

	"codedock/pkg/agent/tool"
)

// TextContent 是用户与助手消息的统一文本载荷。
type TextContent struct {
	Text string `json:"text"`
}

// ToolResultContent 是工具结果消息的统一载荷。
type ToolResultContent struct {
	CallID string          `json:"call_id"`
	Output json.RawMessage `json:"output"`
}

// EncodeText 将文本编码为消息 Content。
func EncodeText(text string) json.RawMessage {
	body, err := json.Marshal(TextContent{Text: text})
	if err != nil {
		return json.RawMessage(`{"text":""}`)
	}
	return body
}

// DecodeText 从消息 Content 取出文本。
func DecodeText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var payload TextContent
	if err := json.Unmarshal(content, &payload); err == nil {
		return payload.Text
	}
	var raw string
	if err := json.Unmarshal(content, &raw); err == nil {
		return raw
	}
	return string(content)
}

// EncodeToolResult 将工具结果编码为消息 Content。
func EncodeToolResult(callID string, output json.RawMessage) json.RawMessage {
	if len(output) == 0 {
		output = json.RawMessage("null")
	}
	body, err := json.Marshal(ToolResultContent{CallID: callID, Output: output})
	if err != nil {
		return json.RawMessage(`{"call_id":"","output":null}`)
	}
	return body
}

// EncodeToolError 把工具失败编码成带 call_id 的结果，方便回填模型。
func EncodeToolError(callID, message string) json.RawMessage {
	if message == "" {
		message = "tool failed"
	}
	payload, err := json.Marshal(map[string]string{"error": message})
	if err != nil {
		payload = json.RawMessage(`{"error":"tool failed"}`)
	}
	return EncodeToolResult(callID, payload)
}

// CompleteToolResults 补齐助手 tool_calls 缺失的 tool 消息，避免下一轮模型调用因缺结果失败。
func CompleteToolResults(messages []Message) []Message {
	if len(messages) == 0 {
		return messages
	}
	out := make([]Message, 0, len(messages)+4)
	var pending []tool.Call
	seen := map[string]struct{}{}

	flush := func() {
		for _, call := range pending {
			if call.ID == "" {
				continue
			}
			if _, ok := seen[call.ID]; ok {
				continue
			}
			out = append(out, Message{
				Role:    RoleTool,
				Content: EncodeToolError(call.ID, "tool result missing"),
			})
		}
		pending = nil
		seen = map[string]struct{}{}
	}

	for _, msg := range messages {
		switch msg.Role {
		case RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				flush()
				out = append(out, msg)
				pending = append([]tool.Call(nil), msg.ToolCalls...)
				seen = map[string]struct{}{}
				continue
			}
			flush()
			out = append(out, msg)
		case RoleTool:
			var result ToolResultContent
			_ = json.Unmarshal(msg.Content, &result)
			if result.CallID == "" {
				for _, call := range pending {
					if call.ID == "" {
						continue
					}
					if _, ok := seen[call.ID]; !ok {
						result.CallID = call.ID
						break
					}
				}
				output := result.Output
				if len(output) == 0 {
					if text := DecodeText(msg.Content); text != "" {
						output, _ = json.Marshal(map[string]string{"error": text})
					}
				}
				if result.CallID != "" {
					msg.Content = EncodeToolResult(result.CallID, output)
				}
			}
			if result.CallID != "" {
				seen[result.CallID] = struct{}{}
			}
			out = append(out, msg)
		case RoleUser:
			flush()
			out = append(out, msg)
		default:
			out = append(out, msg)
		}
	}
	flush()
	return out
}
