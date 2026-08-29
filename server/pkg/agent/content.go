package agent

import "encoding/json"

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
