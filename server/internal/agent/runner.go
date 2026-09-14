package agent

import (
	"context"
	"log/slog"
	"sync"

	agenttools "codedock/internal/agent/tools"
	"codedock/internal/events"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/seam"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db"
	"codedock/pkg/db/sqlite"
)

// Runtime 负责 Agent 运行时的整体编排：管理 AgentState、调度 StepJob 与压缩记忆索引。
type Runtime struct {
	db           db.Client
	queries      *sqlite.Queries
	bus          *events.Bus
	worker       *Worker
	engine       *pkgagent.Engine
	tools        tool.Registry
	log          *slog.Logger
	model        pkgagent.ModelConfig
	compact      sync.Map
	compactWG    sync.WaitGroup
	claimMu      sync.Mutex
	claimedSteps map[string]struct{}
	dispatcher   seam.Dispatcher
	extraMethods methodNamer
}

// methodNamer 由插件宿主实现，用来把插件方法并进本轮可执行绑定。
type methodNamer interface {
	MethodNames() []string
}

// New 创建 Runtime 及其 Worker。工具定义在 tools 包注册；ports 只注入工具 Execute 所需的外部实现。
func New(client db.Client, queries *sqlite.Queries, bus *events.Bus, tools tool.Registry, log *slog.Logger, ports agenttools.Ports) *Runtime {
	if tools == nil {
		tools = tool.NewRegistry()
	}
	if log == nil {
		log = slog.Default()
	}
	runtime := &Runtime{
		db:           client,
		queries:      queries,
		bus:          bus,
		tools:        tools,
		log:          log,
		model:        pkgagent.ModelConfig{Provider: "fake", Model: "fake"},
		claimedSteps: map[string]struct{}{},
	}
	runtime.engine = pkgagent.NewEngine(&pkgagent.Brain{}, runtime, tools)
	agenttools.Register(tools, queries, runtime.EnqueueIndexCompact, ports)
	runtime.worker = NewWorker(runtime)
	return runtime
}

// SetModel 设置记忆索引压缩后台任务使用的模型。
func (r *Runtime) SetModel(model pkgagent.ModelConfig) {
	if r == nil {
		return
	}
	if model.Provider == "" {
		model.Provider = "fake"
	}
	if model.Model == "" {
		model.Model = "fake"
	}
	r.model = model
}

// logger 返回运行时日志；Runtime 或字段为空时回退到 slog.Default。
func (r *Runtime) logger() *slog.Logger {
	if r == nil || r.log == nil {
		return slog.Default()
	}
	return r.log
}

// Worker 返回执行 StepJob 的 Worker。
func (r *Runtime) Worker() *Worker {
	if r == nil {
		return nil
	}
	return r.worker
}

// Tools 返回工具注册中心。
func (r *Runtime) Tools() tool.Registry {
	if r == nil {
		return nil
	}
	return r.tools
}

// SetConcurrency 设置进程级 LLM / 工具并发上限。n<=0 表示不限制。
func (r *Runtime) SetConcurrency(llm, tools int) {
	if r == nil || r.engine == nil {
		return
	}
	r.engine.SetGates(pkgagent.NewSlotLimiter(llm), pkgagent.NewSlotLimiter(tools))
}

// SetDispatcher 注入喊话器，并同步给 Engine。若实现 MethodNames，则记为额外可执行方法。
func (r *Runtime) SetDispatcher(d seam.Dispatcher) {
	if r == nil {
		return
	}
	r.dispatcher = d
	if r.engine != nil {
		r.engine.SetDispatcher(d)
	}
	r.extraMethods = nil
	if namer, ok := d.(methodNamer); ok {
		r.extraMethods = namer
	}
}

// Dispatcher 返回当前喊话器。
func (r *Runtime) Dispatcher() seam.Dispatcher {
	if r == nil {
		return nil
	}
	return r.dispatcher
}

// extraMethodNames 返回插件挂上的方法名，Load 时并进本轮可执行绑定。
func (r *Runtime) extraMethodNames() []string {
	if r == nil || r.extraMethods == nil {
		return nil
	}
	return r.extraMethods.MethodNames()
}

// Start 启动 Worker。不自动恢复库里未完成的 Job，需用户显式 RecoverRun。
func (r *Runtime) Start(ctx context.Context) {
	if r.worker != nil {
		r.worker.Start(ctx)
	}
}
