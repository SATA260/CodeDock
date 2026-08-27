package events

import (
	"log/slog"
	"sync"
)

// Event 表示由 Handler 或业务模块发布的领域事件。
type Event struct {
	Type        string // 例如 "issue:created"、"inbox:new"
	WorkspaceID string // 用于路由到正确的 Hub 房间
	ActorType   string // "member"、"agent" 或 "system"
	ActorID     string
	Payload     any // 可序列化为 JSON，结构与当前 WebSocket 载荷一致

	// 以下可选范围字段帮助实时分发层将事件路由到比
	// `workspace:{WorkspaceID}` 更具体的范围。设置后，监听器无需重新
	// 反序列化 Payload 即可确定目标 Redis Stream 或 Hub 房间。
	TaskID        string
	ChatSessionID string
}

// Handler 是处理事件的函数。
type Handler func(Event)

// Bus 是进程内同步发布订阅事件总线。
// TODO: Redis Stream、WebSocket、SSE 等跨进程分发作为监听适配层接入。
type Bus struct {
	mu             sync.RWMutex
	listeners      map[string][]Handler
	globalHandlers []Handler
}

// New 创建事件总线。
func New() *Bus {
	return &Bus{
		listeners: make(map[string][]Handler),
	}
}

// Subscribe 为指定事件类型注册处理函数。
// 处理函数按照注册顺序同步调用。
func (b *Bus) Subscribe(eventType string, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.listeners[eventType] = append(b.listeners[eventType], h)
}

// SubscribeAll 注册接收全部事件的全局处理函数。
// 全局处理函数在指定类型的处理函数之后调用。
func (b *Bus) SubscribeAll(h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.globalHandlers = append(b.globalHandlers, h)
}

// Publish 将事件分发给该类型的全部处理函数。
// 指定类型的处理函数先执行，随后执行全局处理函数。
// 每个处理函数都同步调用，并单独恢复 panic，避免影响其他处理函数。
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	handlers := b.listeners[e.Type]
	globals := b.globalHandlers
	b.mu.RUnlock()

	for _, h := range handlers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in event listener", "event_type", e.Type, "recovered", r)
				}
			}()
			h(e)
		}()
	}

	for _, h := range globals {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in global event listener", "event_type", e.Type, "recovered", r)
				}
			}()
			h(e)
		}()
	}
}
