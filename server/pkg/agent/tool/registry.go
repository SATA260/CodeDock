package tool

import (
	"fmt"
	"sync"
)

type memoryRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry 创建进程内工具注册中心。
func NewRegistry() Registry {
	return &memoryRegistry{tools: make(map[string]Tool)}
}

// Register 按工具名覆盖写入注册表。
func (r *memoryRegistry) Register(item Tool) error {
	if item == nil {
		return fmt.Errorf("tool is nil")
	}
	def := item.Definition()
	if def.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[def.Name] = item
	return nil
}

// Get 按名称取出工具；指定 Version 时还校验版本。
func (r *memoryRegistry) Get(ref Reference) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.tools[ref.Name]
	if !ok {
		return nil, fmt.Errorf("tool %q not found", ref.Name)
	}
	if ref.Version != "" && item.Definition().Version != "" && ref.Version != item.Definition().Version {
		return nil, fmt.Errorf("tool %q version %q not found", ref.Name, ref.Version)
	}
	return item, nil
}

// GetAll 按引用列表批量取出工具，任一缺失则失败。
func (r *memoryRegistry) GetAll(refs []Reference) ([]Tool, error) {
	out := make([]Tool, 0, len(refs))
	for _, ref := range refs {
		item, err := r.Get(ref)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// Prompts 汇总全部已注册工具的提示词与 schema。
func (r *memoryRegistry) Prompts() []Prompt {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Prompt, 0, len(r.tools))
	for _, item := range r.tools {
		def := item.Definition()
		out = append(out, Prompt{
			Name:             def.Name,
			Content:          def.Prompt,
			ParametersSchema: def.ParametersSchema,
			OutputSchema:     def.OutputSchema,
		})
	}
	return out
}

// Definitions 返回当前注册的全部工具定义。
func Definitions(reg Registry) []Definition {
	if reg == nil {
		return nil
	}
	prompts := reg.Prompts()
	out := make([]Definition, 0, len(prompts))
	for _, prompt := range prompts {
		item, err := reg.Get(Reference{Name: prompt.Name})
		if err != nil {
			continue
		}
		out = append(out, item.Definition())
	}
	return out
}
