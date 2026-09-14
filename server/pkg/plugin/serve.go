package plugin

import goplugin "github.com/hashicorp/go-plugin"

// Serve 在插件进程里启动 gRPC 服务，供宿主连接。
func Serve(p Plugin) {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins: map[string]goplugin.Plugin{
			PluginName: &GRPCPlugin{Impl: &runtime{Plugin: p}},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}
