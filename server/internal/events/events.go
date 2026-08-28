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

type subscription struct {
	id      uint64
	handler Handler
}

// Bus 是进程内同步发布订阅事件总线。
// TODO: Redis Stream、WebSocket、SSE 等跨进程分发作为监听适配层接入。
type Bus struct {
	mu             sync.RWMutex
	nextID         uint64
	listeners      map[string][]subscription
	globalHandlers []subscription
}

// New 创建事件总线。
func New() *Bus {
	return &Bus{
		listeners: make(map[string][]subscription),
	}
}

// Subscribe 为指定事件类型注册处理函数，返回取消订阅函数。
// 处理函数按照注册顺序同步调用。
func (b *Bus) Subscribe(eventType string, h Handler) func() {
	if b == nil || h == nil {
		return func() {}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	id := b.nextID
	b.listeners[eventType] = append(b.listeners[eventType], subscription{id: id, handler: h})
	return func() {
		b.remove(eventType, id)
	}
}

// SubscribeAll 注册接收全部事件的全局处理函数，返回取消订阅函数。
// 全局处理函数在指定类型的处理函数之后调用。
func (b *Bus) SubscribeAll(h Handler) func() {
	if b == nil || h == nil {
		return func() {}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	id := b.nextID
	b.globalHandlers = append(b.globalHandlers, subscription{id: id, handler: h})
	return func() {
		b.remove("", id)
	}
}

// remove 按订阅 ID 删除指定类型或全局监听器。
func (b *Bus) remove(eventType string, id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if eventType == "" {
		b.globalHandlers = withoutID(b.globalHandlers, id)
		return
	}
	filtered := withoutID(b.listeners[eventType], id)
	if len(filtered) == 0 {
		delete(b.listeners, eventType)
		return
	}
	b.listeners[eventType] = filtered
}

// withoutID 从订阅列表中去掉指定 ID。
func withoutID(items []subscription, id uint64) []subscription {
	filtered := items[:0]
	for _, item := range items {
		if item.id != id {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// Publish 将事件分发给该类型的全部处理函数。
// 指定类型的处理函数先执行，随后执行全局处理函数。
// 每个处理函数都同步调用，并单独恢复 panic，避免影响其他处理函数。
func (b *Bus) Publish(e Event) {
	if b == nil {
		return
	}
	b.mu.RLock()
	handlers := append([]subscription(nil), b.listeners[e.Type]...)
	globals := append([]subscription(nil), b.globalHandlers...)
	b.mu.RUnlock()

	for _, item := range handlers {
		invoke(e, item.handler, false)
	}
	for _, item := range globals {
		invoke(e, item.handler, true)
	}
}

// invoke 同步调用处理函数，并单独恢复 panic。
func invoke(e Event, h Handler, global bool) {
	defer func() {
		if r := recover(); r != nil {
			if global {
				slog.Error("panic in global event listener", "event_type", e.Type, "recovered", r)
				return
			}
			slog.Error("panic in event listener", "event_type", e.Type, "recovered", r)
		}
	}()
	h(e)
}
