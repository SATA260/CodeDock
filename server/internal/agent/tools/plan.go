package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codedock/pkg/agent/tool"
)

type planListInput struct{}

type planReadInput struct {
	Name string `json:"name"`
}

type planWriteInput struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type planListOutput struct {
	Names []string `json:"names"`
}

type planItemOutput struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type planTool struct {
	name   string
	prompt string
	schema json.RawMessage
	ports  Ports
}

func (t planTool) Definition() tool.Definition {
	return tool.Definition{
		Name:             t.name,
		Prompt:           t.prompt,
		ParametersSchema: t.schema,
		Permission:       tool.Permission{Effect: tool.EffectAllow},
		Version:          "1",
	}
}

func (t planTool) Inspect(_ context.Context, input tool.Input) error {
	if t.name == "plan_list" {
		return nil
	}
	_, err := planNameFromArgs(t.name, input.Call.Arguments)
	return err
}

func (t planTool) Execute(ctx context.Context, input tool.Input) (tool.Result, error) {
	if err := ctx.Err(); err != nil {
		return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, err
	}
	dir, err := planDir(workspaceOf(input, t.ports))
	if err != nil {
		return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
	}
	switch t.name {
	case "plan_list":
		entries, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
				continue
			}
			names = append(names, entry.Name())
		}
		return okToolResult(input.Call.ID, t.name, planListOutput{Names: names})
	case "plan_read":
		name, err := planNameFromArgs(t.name, input.Call.Arguments)
		if err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		return okToolResult(input.Call.ID, t.name, planItemOutput{Name: name, Content: string(body)})
	case "plan_write":
		var args planWriteInput
		if err := json.Unmarshal(nonzeroJSON(input.Call.Arguments), &args); err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		name, err := sanitizePlanName(args.Name)
		if err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(args.Content), 0o644); err != nil {
			return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: err.Error()}, nil
		}
		return okToolResult(input.Call.ID, t.name, planItemOutput{Name: name, Content: args.Content})
	default:
		return tool.Result{CallID: input.Call.ID, Name: t.name, Success: false, Error: "unknown plan tool"}, nil
	}
}

func planNameFromArgs(name string, raw json.RawMessage) (string, error) {
	var args planReadInput
	if err := json.Unmarshal(nonzeroJSON(raw), &args); err != nil {
		return "", err
	}
	if name == "plan_write" {
		var write planWriteInput
		if err := json.Unmarshal(nonzeroJSON(raw), &write); err != nil {
			return "", err
		}
		args.Name = write.Name
	}
	return sanitizePlanName(args.Name)
}

func sanitizePlanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	base := filepath.Base(name)
	if base != filepath.Clean(name) || strings.Contains(name, "/") || strings.Contains(name, `\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid plan name")
	}
	if !strings.HasSuffix(strings.ToLower(base), ".md") {
		base += ".md"
	}
	return base, nil
}

func isWorkspacePlanFile(ports Ports, name string, raw json.RawMessage, root string) bool {
	path, err := codingPath(name, raw)
	if err != nil || strings.TrimSpace(path) == "" || strings.TrimSpace(root) == "" {
		return false
	}
	exec := ports.executor()
	resolved, err := exec.resolveToCWD(path, root)
	if err != nil {
		return false
	}
	jailed, err := jailPath(root, resolved, exec.FS)
	if err != nil {
		return false
	}
	rel, err := pathRel(filepath.Clean(root), jailed)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	base, ok := strings.CutPrefix(rel, ".cursor/")
	if !ok || base == "" || strings.Contains(base, "/") {
		return false
	}
	return strings.HasSuffix(strings.ToLower(base), ".md")
}

func planDir(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("workspace root is required")
	}
	dir := filepath.Join(root, ".cursor")
	if _, err := jailPath(root, dir, nil); err != nil {
		return "", err
	}
	return dir, nil
}

func okToolResult(callID, name string, body any) (tool.Result, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return tool.Result{CallID: callID, Name: name, Success: false, Error: err.Error()}, nil
	}
	return tool.Result{CallID: callID, Name: name, Output: raw, Success: true}, nil
}

func planTools(ports Ports) []tool.Tool {
	return []tool.Tool{
		planTool{name: "plan_list", prompt: "列出工作区 .cursor/ 下的 markdown 计划。", schema: schemaOf[planListInput](), ports: ports},
		planTool{name: "plan_read", prompt: "读取工作区 .cursor/ 下的一篇 markdown 计划。", schema: schemaOf[planReadInput](), ports: ports},
		planTool{name: "plan_write", prompt: "新建或覆盖工作区 .cursor/ 下的一篇 markdown 计划。", schema: schemaOf[planWriteInput](), ports: ports},
	}
}
