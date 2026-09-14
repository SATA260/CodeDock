package main

import (
	"context"
	"strings"
	"time"

	sdk "codedock/pkg/plugin"
)

// probe 给宿主测试用：改正文、换向、故意拖延。
type probe struct{}

// Bootstrap 只订阅 agent/input。
func (probe) Bootstrap(context.Context, sdk.Host) (sdk.Manifest, error) {
	return sdk.Manifest{
		Name:          "probe",
		Subscriptions: []string{sdk.TypeInput},
	}, nil
}

// OnAgentInput 测拖延、input/handled，以及给正文加 +probe。
func (probe) OnAgentInput(_ context.Context, in sdk.AgentInput) (sdk.AgentInputResult, error) {
	if strings.HasPrefix(in.Content, "stall:") {
		time.Sleep(2 * time.Second)
		return in.Reply(), nil
	}
	if in.Content == "handle-me" {
		return in.Handle(), nil
	}
	in.Content += " +probe"
	return in.Reply(), nil
}

// ExecuteMethod 声明本插件没有方法。
func (probe) ExecuteMethod(context.Context, sdk.MethodInput) (sdk.MethodResult, error) {
	return sdk.MethodResult{Success: false, Error: "no methods"}, nil
}

// main 启动 probe 插件进程。
func main() {
	sdk.Serve(probe{})
}
