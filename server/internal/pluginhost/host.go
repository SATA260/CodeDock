package pluginhost

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"

	"codedock/internal/agent/memory"
	"codedock/internal/events"
	"codedock/internal/util"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/seam"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db/sqlite"
	sdk "codedock/pkg/plugin"
)

var reservedMethods = map[string]struct{}{
	"ping":          {},
	"memory_read":   {},
	"memory_write":  {},
	"memory_search": {},
}

// Options 是加载插件宿主的入参。
type Options struct {
	Dir      string               // PLUGIN_DIR，每个子目录一个插件
	Timeout  time.Duration        // 单次 RPC 超时；零则用 10s
	Registry tool.Registry        // 用来挂插件方法；空则新建
	Queries  *sqlite.Queries      // 记忆和 AppendNotice 用；可空
	Model    pkgagent.ModelConfig // Complete 自己打模型时用
	Log      *slog.Logger
}

// Host 按子目录拉起插件进程，并实现 seam.Dispatcher。
type Host struct {
	plugins  []*instance
	registry tool.Registry
	queries  *sqlite.Queries
	model    pkgagent.ModelConfig
	timeout  time.Duration
	log      *slog.Logger
	unsub    func()                     // Attach 订阅账本后的取消函数
	ctxMu    sync.Mutex                 // 保护 contexts
	contexts map[string]json.RawMessage // session: / run: 下的插件共享袋子
}

// instance 是一个已拉起的插件进程。
type instance struct {
	name    string // PLUGIN_DIR 子目录名，也是 Seen 里的名字
	client  *goplugin.Client
	plugin  sdk.Handler
	subs    map[string]struct{}
	methods map[string]struct{}
	mu      sync.Mutex // 同一插件串行 RPC
}

// hostRPC 把单个插件的 Host 回调转到本 Host。
type hostRPC struct {
	host *Host
	inst *instance
}

// Load 扫描 PLUGIN_DIR 的每个子目录，按名字排序拉起进程并 Bootstrap。
func Load(ctx context.Context, opts Options) (*Host, error) {
	dir := strings.TrimSpace(opts.Dir)
	if dir == "" {
		return nil, nil
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	if opts.Registry == nil {
		opts.Registry = tool.NewRegistry()
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	h := &Host{
		registry: opts.Registry,
		queries:  opts.Queries,
		model:    opts.Model,
		timeout:  opts.Timeout,
		log:      opts.Log,
		contexts: map[string]json.RawMessage{},
	}
	for _, name := range names {
		bin := filepath.Join(dir, name, name)
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		if _, err := os.Stat(bin); err != nil {
			h.log.Warn("skip plugin without binary", "name", name, "path", bin)
			continue
		}
		if err := h.start(ctx, name, bin); err != nil {
			_ = h.Close()
			return nil, fmt.Errorf("load plugin %s: %w", name, err)
		}
	}
	return h, nil
}

// start 拉起一个插件二进制并完成 Bootstrap。
func (h *Host) start(ctx context.Context, name, bin string) error {
	inst := &instance{name: name, subs: map[string]struct{}{}, methods: map[string]struct{}{}}
	cmd := exec.Command(bin)
	cmd.Dir = filepath.Dir(bin)
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: sdk.Handshake,
		Plugins: map[string]goplugin.Plugin{
			sdk.PluginName: &sdk.GRPCPlugin{Host: &hostRPC{host: h, inst: inst}},
		},
		Cmd:              cmd,
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           hclog.New(&hclog.LoggerOptions{Name: "plugin-" + name, Level: hclog.Error, Output: os.Stderr}),
	})
	inst.client = client
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return err
	}
	raw, err := rpcClient.Dispense(sdk.PluginName)
	if err != nil {
		client.Kill()
		return err
	}
	p, ok := raw.(sdk.Handler)
	if !ok {
		client.Kill()
		return fmt.Errorf("dispensed plugin has unexpected type %T", raw)
	}
	inst.plugin = p
	man, err := p.Bootstrap(ctx, nil)
	if err != nil {
		client.Kill()
		return err
	}
	if man.Name == "" {
		man.Name = name
	}
	for _, sub := range man.Subscriptions {
		if sub != "" {
			inst.subs[sub] = struct{}{}
		}
	}
	h.plugins = append(h.plugins, inst)
	h.log.Info("plugin loaded", "name", man.Name, "subscriptions", man.Subscriptions)
	return nil
}

// Dispatch 按 Seen 逐个问订阅了该口的插件；类型变了立刻返回。
func (h *Host) Dispatch(ctx context.Context, ev seam.Envelope) (seam.Envelope, error) {
	if h == nil {
		return ev, nil
	}
	if ev.ChainID == "" {
		ev.ChainID = util.NewID()
	}
	if seam.IsSeam(ev.Type) {
		ev = h.attachPluginContext(ev)
	}
	seen := append([]string{}, ev.Seen...)
	var err error
	defer func() {
		h.persistPluginContext(ev)
		if ev.Type == seam.TypeInputHandled {
			h.dropSessionPluginContext(ev.SessionID)
		}
	}()
	for _, inst := range h.plugins {
		if !inst.subscribed(ev.Type) || contains(seen, inst.name) {
			continue
		}
		sentType := ev.Type
		var out seam.Envelope
		out, err = inst.callOnEvent(ctx, h.timeout, ev)
		if err != nil {
			return ev, fmt.Errorf("plugin %s: %w", inst.name, err)
		}
		out.ChainID = ev.ChainID
		out.SessionID = ev.SessionID
		out.RunID = ev.RunID
		out.TurnID = ev.TurnID
		if !sdk.AcceptableContext(out.Context) {
			if h.log != nil {
				h.log.Warn("plugin context rejected", "plugin", inst.name, "bytes", len(out.Context))
			}
			out.Context = ev.Context
		}
		seen = append(seen, inst.name)
		out.Seen = append([]string{}, seen...)
		if out.Type == "" {
			out.Type = sentType
		}
		ev = out
		if ev.Type != sentType {
			return ev, nil
		}
	}
	ev.Seen = seen
	return ev, nil
}

// Emit 另发一条与当前口无关的事件。六个口的同名事件会被拒绝。
func (h *Host) Emit(ctx context.Context, ev seam.Envelope) error {
	if h == nil {
		return fmt.Errorf("plugin host is nil")
	}
	if seam.IsSeam(ev.Type) {
		return fmt.Errorf("cannot emit seam event %q", ev.Type)
	}
	ev.ChainID = util.NewID()
	ev.Seen = nil
	go h.notify(context.WithoutCancel(ctx), ev)
	return nil
}

// MethodNames 返回已挂进 Registry 的插件方法名。
func (h *Host) MethodNames() []string {
	if h == nil {
		return nil
	}
	var out []string
	for _, inst := range h.plugins {
		for name := range inst.methods {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Attach 把账本事实异步转给订阅了该类型的插件；跳过 assistant.delta。
func (h *Host) Attach(bus *events.Bus) {
	if h == nil || bus == nil {
		return
	}
	h.unsub = bus.SubscribeAll(func(e events.Event) {
		if e.Type == string(pkgagent.EventAssistantDelta) {
			return
		}
		ev := seam.Envelope{Type: e.Type, ChainID: util.NewID()}
		if ae, ok := e.Payload.(pkgagent.AgentEvent); ok {
			ev.SessionID = ae.SessionID
			ev.RunID = ae.RunID
			if ae.TurnID != nil {
				ev.TurnID = *ae.TurnID
			}
			ev.Payload = ae.Payload
		}
		go h.notify(context.Background(), ev)
	})
}

// Close 停掉全部插件进程。
func (h *Host) Close() error {
	if h == nil {
		return nil
	}
	if h.unsub != nil {
		h.unsub()
		h.unsub = nil
	}
	for _, inst := range h.plugins {
		if inst.client != nil {
			inst.client.Kill()
		}
	}
	h.plugins = nil
	h.ctxMu.Lock()
	h.contexts = nil
	h.ctxMu.Unlock()
	return nil
}

// notify 把非换向事件异步发给订阅了该类型的插件。
func (h *Host) notify(ctx context.Context, ev seam.Envelope) {
	ev = h.attachPluginContext(ev)
	for _, inst := range h.plugins {
		if !inst.subscribed(ev.Type) {
			continue
		}
		if _, err := inst.callOnEvent(ctx, h.timeout, ev); err != nil {
			h.log.Warn("plugin notify failed", "plugin", inst.name, "type", ev.Type, "error", err)
		}
	}
	if isTerminalLedger(ev.Type) {
		h.dropPluginContext(ev.RunID, ev.SessionID)
	}
}

// registerMethod 把插件方法挂进工具 Registry，并拒绝保留名和重名。
func (h *Host) registerMethod(inst *instance, m sdk.Method) error {
	if m.Name == "" {
		return fmt.Errorf("method name is required")
	}
	if _, ok := reservedMethods[m.Name]; ok {
		return fmt.Errorf("method %q is reserved", m.Name)
	}
	if _, err := h.registry.Get(tool.Reference{Name: m.Name}); err == nil {
		return fmt.Errorf("method %q already registered", m.Name)
	}
	def := sdk.MethodToDefinition(m)
	if err := h.registry.Register(&methodTool{inst: inst, def: def, timeout: h.timeout}); err != nil {
		return err
	}
	inst.methods[m.Name] = struct{}{}
	return nil
}

// subscribed 判断该插件是否订阅了这个事件类型。
func (i *instance) subscribed(typ string) bool {
	if i == nil {
		return false
	}
	_, ok := i.subs[typ]
	return ok
}

// callOnEvent 在超时内串行调用插件的 OnEvent。
func (i *instance) callOnEvent(ctx context.Context, timeout time.Duration, ev seam.Envelope) (seam.Envelope, error) {
	if i == nil || i.plugin == nil {
		return ev, fmt.Errorf("plugin is not started")
	}
	if i.client != nil && i.client.Exited() {
		return ev, fmt.Errorf("plugin %s exited", i.name)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return i.plugin.OnEvent(ctx, ev)
}

// callExecute 在超时内串行调用插件的 ExecuteMethod。
func (i *instance) callExecute(ctx context.Context, timeout time.Duration, input tool.Input) (tool.Result, error) {
	if i == nil || i.plugin == nil {
		return tool.Result{}, fmt.Errorf("plugin is not started")
	}
	if i.client != nil && i.client.Exited() {
		return tool.Result{}, fmt.Errorf("plugin %s exited", i.name)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := i.plugin.ExecuteMethod(ctx, sdk.MethodInput{
		SessionID: input.SessionID,
		RunID:     input.RunID,
		TurnID:    input.TurnID,
		CallID:    input.Call.ID,
		Name:      input.Call.Name,
		Arguments: input.Call.Arguments,
	})
	if err != nil {
		return tool.Result{}, err
	}
	return tool.Result{
		CallID:  input.Call.ID,
		Name:    input.Call.Name,
		Success: out.Success,
		Output:  out.Output,
		Error:   out.Error,
	}, nil
}

// Emit 转发插件的另发事件请求。
func (r *hostRPC) Emit(ctx context.Context, ev seam.Envelope) error {
	return r.host.Emit(ctx, ev)
}

// RegisterMethod 把该方法记到发起回调的那个插件上。
func (r *hostRPC) RegisterMethod(_ context.Context, method sdk.Method) error {
	return r.host.registerMethod(r.inst, method)
}

// MemoryGet 按会话读一篇专题记忆。
func (r *hostRPC) MemoryGet(ctx context.Context, key sdk.MemoryKey) (string, error) {
	if r.host.queries == nil {
		return "", fmt.Errorf("memory is not available")
	}
	if key.SessionID == "" || key.Name == "" {
		return "", fmt.Errorf("session_id and name are required")
	}
	sess, err := r.host.queries.GetSession(ctx, key.SessionID)
	if err != nil {
		return "", err
	}
	scope, scopeID := memoryScope(key.Scope, sess)
	item, err := memory.Get(ctx, r.host.queries, memory.TextMemoryKey{
		Scope:   scope,
		ScopeID: scopeID,
		Kind:    memory.KindTopic,
		Name:    key.Name,
	})
	if err != nil {
		return "", err
	}
	return item.Content, nil
}

// MemoryUpsert 按会话写一篇专题记忆。
func (r *hostRPC) MemoryUpsert(ctx context.Context, key sdk.MemoryKey, text string) error {
	if r.host.queries == nil {
		return fmt.Errorf("memory is not available")
	}
	if key.SessionID == "" || key.Name == "" {
		return fmt.Errorf("session_id and name are required")
	}
	sess, err := r.host.queries.GetSession(ctx, key.SessionID)
	if err != nil {
		return err
	}
	scope, scopeID := memoryScope(key.Scope, sess)
	_, err = memory.Upsert(ctx, r.host.queries, memory.TextMemory{
		Scope:   scope,
		ScopeID: scopeID,
		Kind:    memory.KindTopic,
		Name:    key.Name,
		Content: text,
	})
	return err
}

// Complete 用宿主模型配置单独打一次模型，不进当前助手流。
func (r *hostRPC) Complete(ctx context.Context, req sdk.CompleteRequest) (sdk.CompleteResult, error) {
	chat := pkgagent.Chat{
		SessionID:    req.SessionID,
		RunID:        req.RunID,
		Model:        r.host.model,
		SystemPrompt: req.Prompt,
		Messages:     []pkgagent.Message{{Role: pkgagent.RoleUser, Content: pkgagent.EncodeText(req.Text)}},
	}
	stream, err := pkgagent.Stream(ctx, chat)
	if err != nil {
		return sdk.CompleteResult{}, err
	}
	defer stream.Close()
	result, err := stream.Result(ctx)
	if err != nil {
		return sdk.CompleteResult{}, err
	}
	return sdk.CompleteResult{Text: pkgagent.DecodeText(result.Message.Content)}, nil
}

// AppendNotice 写一条用户看得见的 system 消息（本期只落库）。
func (r *hostRPC) AppendNotice(ctx context.Context, sessionID, runID, text string) error {
	if r.host.queries == nil {
		return fmt.Errorf("queries are not available")
	}
	if sessionID == "" || strings.TrimSpace(text) == "" {
		return fmt.Errorf("session_id and text are required")
	}
	now := util.FormatTime(util.Now())
	seq, err := r.host.queries.IncrementEventSeq(ctx, sqlite.IncrementEventSeqParams{UpdatedAt: now, ID: sessionID})
	if err != nil {
		return err
	}
	_, err = r.host.queries.InsertMessage(ctx, sqlite.InsertMessageParams{
		ID:        util.NewID(),
		SessionID: sessionID,
		RunID:     nullString(runID),
		Role:      string(pkgagent.RoleSystem),
		Content:   string(pkgagent.EncodeText(text)),
		EventSeq:  seq,
		CreatedAt: now,
	})
	return err
}

// memoryScope 把插件传来的 scope 落到用户或工作区。
func memoryScope(scope string, sess sqlite.Session) (memory.TextMemoryScope, string) {
	if scope == string(memory.ScopeUser) {
		return memory.ScopeUser, sess.UserID
	}
	return memory.ScopeWorkspace, sess.WorkspaceID
}

// nullString 把空串收成 SQL NULL。
func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

// contains 判断字符串是否已在切片里。
func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// methodTool 把插件方法暴露成 Registry 里的 Tool。
type methodTool struct {
	inst    *instance
	def     tool.Definition
	timeout time.Duration
}

// Definition 返回挂进 Registry 的工具描述。
func (m *methodTool) Definition() tool.Definition { return m.def }

// Execute 把模型的工具调用转到插件进程。
func (m *methodTool) Execute(ctx context.Context, input tool.Input) (tool.Result, error) {
	result, err := m.inst.callExecute(ctx, m.timeout, input)
	if err != nil {
		return tool.Result{CallID: input.Call.ID, Name: m.def.Name, Success: false, Error: err.Error()}, nil
	}
	if result.CallID == "" {
		result.CallID = input.Call.ID
	}
	if result.Name == "" {
		result.Name = m.def.Name
	}
	return result, nil
}
