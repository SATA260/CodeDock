package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codedock/pkg/agent/tool"
)

type codingTool struct {
	name   string
	prompt string
	effect tool.Effect
	schema json.RawMessage
	ports  Ports
}

func (t codingTool) Definition() tool.Definition {
	return tool.Definition{
		Name:             t.name,
		Prompt:           t.prompt,
		ParametersSchema: t.schema,
		Permission:       tool.Permission{Effect: t.effect},
		SupportsCancel:   t.name == ToolBash || t.name == ToolPowerShell,
		SupportsRetry:    t.name == ToolRead || t.name == ToolGrep || t.name == ToolFind || t.name == ToolLS,
		Version:          "1",
	}
}

func (t codingTool) Inspect(_ context.Context, input tool.Input) error {
	return inspectCodingPathAt(t.ports, t.name, input.Call.Arguments, workspaceOf(input, t.ports))
}

func (t codingTool) ResolveEffect(_ context.Context, input tool.Input) tool.Effect {
	if (t.name == ToolWrite || t.name == ToolEdit) && isWorkspacePlanFile(t.ports, t.name, input.Call.Arguments, workspaceOf(input, t.ports)) {
		return tool.EffectAllow
	}
	return t.effect
}

// LockAsk 已存在的测试文件在 manual / auto 下必须人批；yolo 仍放行，正确性靠收尾验证。
func (t codingTool) LockAsk(_ context.Context, input tool.Input) bool {
	if t.name != ToolWrite && t.name != ToolEdit {
		return false
	}
	path, err := codingPath(t.name, input.Call.Arguments)
	if err != nil {
		return false
	}
	return IsExistingTestFile(workspaceOf(input, t.ports), path)
}

func (t codingTool) Execute(ctx context.Context, input tool.Input) (tool.Result, error) {
	if err := ctx.Err(); err != nil {
		return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, err
	}
	if err := denyForeignPlanFile(t.ports, t.name, input); err != nil {
		return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
	}
	exec := t.ports.executor()
	result, err := exec.Execute(ctx, t.name, workspaceOf(input, t.ports), input.Call.Arguments)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, err
		}
		return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
	}
	return tool.Result{CallID: input.Call.ID, Name: t.name, Output: raw, Success: true}, nil
}

// denyForeignPlanFile 挡住用 read/write/edit 去碰未绑定的其他计划。
func denyForeignPlanFile(ports Ports, name string, input tool.Input) error {
	if name != ToolRead && name != ToolWrite && name != ToolEdit {
		return nil
	}
	planName := codingPlanFileName(name, input.Call.Arguments)
	if planName == "" {
		return nil
	}
	if name == ToolWrite || name == ToolEdit {
		path, err := codingPath(name, input.Call.Arguments)
		if err == nil {
			abs, resolveErr := ports.executor().resolveToCWD(path, workspaceOf(input, ports))
			if resolveErr == nil {
				if _, statErr := os.Stat(abs); statErr != nil && os.IsNotExist(statErr) {
					return nil
				}
			}
		}
	}
	return denyForeignPlan(input, planName)
}

// codingPlanFileName 从编码工具路径抽出 .cursor 下的计划文件名。
func codingPlanFileName(name string, raw json.RawMessage) string {
	path, err := codingPath(name, raw)
	if err != nil || strings.TrimSpace(path) == "" {
		return ""
	}
	slash := filepath.ToSlash(filepath.Clean(path))
	if !strings.HasSuffix(strings.ToLower(slash), ".md") {
		return ""
	}
	const mark = ".cursor/"
	idx := strings.LastIndex(strings.ToLower(slash), mark)
	if idx < 0 {
		return ""
	}
	rest := slash[idx+len(mark):]
	if rest == "" || strings.Contains(rest, "/") {
		return ""
	}
	return rest
}

func inspectCodingPath(ports Ports, name string, raw json.RawMessage) error {
	return inspectCodingPathAt(ports, name, raw, ports.WorkspaceRoot)
}

func inspectCodingPathAt(ports Ports, name string, raw json.RawMessage, root string) error {
	path, err := codingPath(name, raw)
	if err != nil || path == "" {
		return err
	}
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("workspace root is required")
	}
	exec := ports.executor()
	resolved, err := exec.resolveToCWD(path, root)
	if err != nil {
		return err
	}
	_, err = jailPath(root, resolved, exec.FS)
	return err
}

func codingPath(name string, raw json.RawMessage) (string, error) {
	switch name {
	case ToolRead:
		var input ReadInput
		if err := json.Unmarshal(nonzeroJSON(raw), &input); err != nil {
			return "", err
		}
		return input.Path, nil
	case ToolWrite:
		var input WriteInput
		if err := json.Unmarshal(nonzeroJSON(raw), &input); err != nil {
			return "", err
		}
		return input.Path, nil
	case ToolEdit:
		input, err := decodeEditInput(nonzeroJSON(raw))
		if err != nil {
			return "", err
		}
		return input.Path, nil
	case ToolGrep:
		var input GrepInput
		if err := json.Unmarshal(nonzeroJSON(raw), &input); err != nil {
			return "", err
		}
		if input.Path != nil {
			return *input.Path, nil
		}
		return ".", nil
	case ToolFind:
		var input FindInput
		if err := json.Unmarshal(nonzeroJSON(raw), &input); err != nil {
			return "", err
		}
		if input.Path != nil {
			return *input.Path, nil
		}
		return ".", nil
	case ToolLS:
		var input LSInput
		if err := json.Unmarshal(nonzeroJSON(raw), &input); err != nil {
			return "", err
		}
		if input.Path != nil {
			return *input.Path, nil
		}
		return ".", nil
	case ToolBash, ToolPowerShell:
		var input ShellInput
		if err := json.Unmarshal(nonzeroJSON(raw), &input); err != nil {
			return "", err
		}
		if input.Command == "" {
			return "", fmt.Errorf("command is required")
		}
		return "", nil
	default:
		return "", nil
	}
}

func nonzeroJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	return raw
}

func codingTools(ports Ports) []tool.Tool {
	return []tool.Tool{
		codingTool{name: ToolRead, prompt: "读取工作区内的文本或图片文件。", effect: tool.EffectAllow, schema: schemaOf[ReadInput](), ports: ports},
		codingTool{name: ToolGrep, prompt: "在工作区内用 ripgrep 搜索文本。", effect: tool.EffectAllow, schema: schemaOf[GrepInput](), ports: ports},
		codingTool{name: ToolFind, prompt: "在工作区内用 fd 按文件名查找。", effect: tool.EffectAllow, schema: schemaOf[FindInput](), ports: ports},
		codingTool{name: ToolLS, prompt: "列出工作区内一个目录的条目。", effect: tool.EffectAllow, schema: schemaOf[LSInput](), ports: ports},
		codingTool{name: ToolWrite, prompt: "写入工作区内的文件，必要时创建目录。", effect: tool.EffectAsk, schema: schemaOf[WriteInput](), ports: ports},
		codingTool{name: ToolEdit, prompt: "按 old/new 文本块改写工作区内的文件。", effect: tool.EffectAsk, schema: schemaOf[EditInput](), ports: ports},
		codingTool{name: ToolBash, prompt: "在工作区根目录执行 bash 命令。", effect: tool.EffectAsk, schema: schemaOf[ShellInput](), ports: ports},
		codingTool{name: ToolPowerShell, prompt: "在工作区根目录执行 PowerShell 命令。", effect: tool.EffectAsk, schema: schemaOf[ShellInput](), ports: ports},
	}
}
