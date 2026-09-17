package plugin

import (
	"context"
	"fmt"

	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	"codedock/pkg/agent/seam"
	pluginpb "codedock/pkg/plugin/proto"
)

// GRPCPlugin 同时给宿主当客户端、给插件进程当服务端。
type GRPCPlugin struct {
	goplugin.NetRPCUnsupportedPlugin
	Impl Handler // 插件进程里的实现；宿主侧由 Serve 包成 runtime
	Host Host    // 宿主白名单，经 broker 回传给插件
}

// GRPCServer 在插件进程里挂 Plugin 服务。
func (p *GRPCPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	pluginpb.RegisterPluginServer(s, &pluginServer{impl: p.Impl, broker: broker})
	return nil
}

// GRPCClient 在宿主进程里拿到插件客户端，并拉起 Host 回调服务。
func (p *GRPCPlugin) GRPCClient(_ context.Context, broker *goplugin.GRPCBroker, conn *grpc.ClientConn) (interface{}, error) {
	return &pluginClient{
		client: pluginpb.NewPluginClient(conn),
		broker: broker,
		host:   p.Host,
	}, nil
}

var _ goplugin.GRPCPlugin = (*GRPCPlugin)(nil)

// pluginClient 是宿主调用插件进程的 gRPC 客户端。
type pluginClient struct {
	client pluginpb.PluginClient
	broker *goplugin.GRPCBroker
	host   Host
}

// Bootstrap 把 Host 服务端交给插件，并取回 Manifest。
func (c *pluginClient) Bootstrap(ctx context.Context, _ Host) (Manifest, error) {
	id := c.broker.NextId()
	go c.broker.AcceptAndServe(id, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)
		pluginpb.RegisterHostServer(s, &hostServer{host: c.host})
		return s
	})
	resp, err := c.client.Bootstrap(ctx, &pluginpb.BootstrapRequest{HostServerId: id})
	if err != nil {
		return Manifest{}, err
	}
	return Manifest{Name: resp.GetName(), Subscriptions: append([]string(nil), resp.GetSubscriptions()...)}, nil
}

// OnEvent 把信封发给插件进程。
func (c *pluginClient) OnEvent(ctx context.Context, ev seam.Envelope) (seam.Envelope, error) {
	resp, err := c.client.OnEvent(ctx, envelopeToPB(ev))
	if err != nil {
		return ev, err
	}
	return envelopeFromPB(resp), nil
}

// ExecuteMethod 在插件进程里执行已登记的方法。
func (c *pluginClient) ExecuteMethod(ctx context.Context, in MethodInput) (MethodResult, error) {
	resp, err := c.client.ExecuteMethod(ctx, methodInputToPB(in))
	if err != nil {
		return MethodResult{}, err
	}
	return methodResultFromPB(resp), nil
}

// pluginServer 是插件进程里的 Plugin 服务。
type pluginServer struct {
	pluginpb.UnimplementedPluginServer
	impl   Handler
	broker *goplugin.GRPCBroker
}

// Bootstrap 拨通宿主回调，再调作者的 Bootstrap。
func (s *pluginServer) Bootstrap(ctx context.Context, req *pluginpb.BootstrapRequest) (*pluginpb.Manifest, error) {
	if s.impl == nil {
		return nil, fmt.Errorf("plugin implementation is nil")
	}
	conn, err := s.broker.Dial(req.GetHostServerId())
	if err != nil {
		return nil, err
	}
	host := &hostClient{client: pluginpb.NewHostClient(conn)}
	man, err := s.impl.Bootstrap(ctx, host)
	if err != nil {
		return nil, err
	}
	return &pluginpb.Manifest{Name: man.Name, Subscriptions: man.Subscriptions}, nil
}

// OnEvent 把 proto 信封交给 Handler。
func (s *pluginServer) OnEvent(ctx context.Context, req *pluginpb.Envelope) (*pluginpb.Envelope, error) {
	if s.impl == nil {
		return nil, fmt.Errorf("plugin implementation is nil")
	}
	out, err := s.impl.OnEvent(ctx, envelopeFromPB(req))
	if err != nil {
		return nil, err
	}
	return envelopeToPB(out), nil
}

// ExecuteMethod 把 proto 方法入参交给 Handler。
func (s *pluginServer) ExecuteMethod(ctx context.Context, req *pluginpb.MethodInput) (*pluginpb.MethodResult, error) {
	if s.impl == nil {
		return nil, fmt.Errorf("plugin implementation is nil")
	}
	out, err := s.impl.ExecuteMethod(ctx, methodInputFromPB(req))
	if err != nil {
		return nil, err
	}
	return methodResultToPB(out), nil
}

// hostClient 是插件进程回调宿主的客户端。
type hostClient struct {
	client pluginpb.HostClient
}

// Emit 请宿主另发一条事件。
func (c *hostClient) Emit(ctx context.Context, ev seam.Envelope) error {
	_, err := c.client.Emit(ctx, envelopeToPB(ev))
	return err
}

// RegisterMethod 请宿主给模型加方法。
func (c *hostClient) RegisterMethod(ctx context.Context, method Method) error {
	_, err := c.client.RegisterMethod(ctx, methodToPB(method))
	return err
}

// MemoryGet 请宿主读一篇专题记忆。
func (c *hostClient) MemoryGet(ctx context.Context, key MemoryKey) (string, error) {
	resp, err := c.client.MemoryGet(ctx, &pluginpb.MemoryKey{
		SessionId: key.SessionID,
		Scope:     key.Scope,
		Name:      key.Name,
	})
	if err != nil {
		return "", err
	}
	return resp.GetText(), nil
}

// MemoryUpsert 请宿主写一篇专题记忆。
func (c *hostClient) MemoryUpsert(ctx context.Context, key MemoryKey, text string) error {
	_, err := c.client.MemoryUpsert(ctx, &pluginpb.MemoryEntry{
		Key: &pluginpb.MemoryKey{
			SessionId: key.SessionID,
			Scope:     key.Scope,
			Name:      key.Name,
		},
		Text: text,
	})
	return err
}

// Complete 请宿主单独打一次模型。
func (c *hostClient) Complete(ctx context.Context, req CompleteRequest) (CompleteResult, error) {
	resp, err := c.client.Complete(ctx, &pluginpb.CompleteRequest{
		SessionId: req.SessionID,
		RunId:     req.RunID,
		Prompt:    req.Prompt,
		Text:      req.Text,
	})
	if err != nil {
		return CompleteResult{}, err
	}
	return CompleteResult{Text: resp.GetText()}, nil
}

// AppendNotice 请宿主写一条用户可见的 system 消息。
func (c *hostClient) AppendNotice(ctx context.Context, sessionID, runID, text string) error {
	_, err := c.client.AppendNotice(ctx, &pluginpb.Notice{
		SessionId: sessionID,
		RunId:     runID,
		Text:      text,
	})
	return err
}

// hostServer 是宿主进程里给插件回调的 Host 服务。
type hostServer struct {
	pluginpb.UnimplementedHostServer
	host Host
}

// Emit 转发插件的另发事件请求。
func (s *hostServer) Emit(ctx context.Context, req *pluginpb.Envelope) (*pluginpb.Empty, error) {
	if s.host == nil {
		return nil, fmt.Errorf("host is nil")
	}
	if err := s.host.Emit(ctx, envelopeFromPB(req)); err != nil {
		return nil, err
	}
	return &pluginpb.Empty{}, nil
}

// RegisterMethod 转发插件的方法登记。
func (s *hostServer) RegisterMethod(ctx context.Context, req *pluginpb.Method) (*pluginpb.Empty, error) {
	if s.host == nil {
		return nil, fmt.Errorf("host is nil")
	}
	if err := s.host.RegisterMethod(ctx, methodFromPB(req)); err != nil {
		return nil, err
	}
	return &pluginpb.Empty{}, nil
}

// MemoryGet 转发插件的记忆读取。
func (s *hostServer) MemoryGet(ctx context.Context, req *pluginpb.MemoryKey) (*pluginpb.MemoryValue, error) {
	if s.host == nil {
		return nil, fmt.Errorf("host is nil")
	}
	text, err := s.host.MemoryGet(ctx, MemoryKey{SessionID: req.GetSessionId(), Scope: req.GetScope(), Name: req.GetName()})
	if err != nil {
		return nil, err
	}
	return &pluginpb.MemoryValue{Text: text}, nil
}

// MemoryUpsert 转发插件的记忆写入。
func (s *hostServer) MemoryUpsert(ctx context.Context, req *pluginpb.MemoryEntry) (*pluginpb.Empty, error) {
	if s.host == nil {
		return nil, fmt.Errorf("host is nil")
	}
	key := req.GetKey()
	if err := s.host.MemoryUpsert(ctx, MemoryKey{SessionID: key.GetSessionId(), Scope: key.GetScope(), Name: key.GetName()}, req.GetText()); err != nil {
		return nil, err
	}
	return &pluginpb.Empty{}, nil
}

// Complete 转发插件的单独模型调用。
func (s *hostServer) Complete(ctx context.Context, req *pluginpb.CompleteRequest) (*pluginpb.CompleteResult, error) {
	if s.host == nil {
		return nil, fmt.Errorf("host is nil")
	}
	out, err := s.host.Complete(ctx, CompleteRequest{
		SessionID: req.GetSessionId(),
		RunID:     req.GetRunId(),
		Prompt:    req.GetPrompt(),
		Text:      req.GetText(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginpb.CompleteResult{Text: out.Text}, nil
}

// AppendNotice 转发插件的 system 通知写入。
func (s *hostServer) AppendNotice(ctx context.Context, req *pluginpb.Notice) (*pluginpb.Empty, error) {
	if s.host == nil {
		return nil, fmt.Errorf("host is nil")
	}
	if err := s.host.AppendNotice(ctx, req.GetSessionId(), req.GetRunId(), req.GetText()); err != nil {
		return nil, err
	}
	return &pluginpb.Empty{}, nil
}
