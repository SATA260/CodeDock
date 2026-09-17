package codex

import (
	"context"

	cderr "codedock/internal/errors"
	pkg "codedock/pkg/codex"
)

// Probe 体检本机 Codex：装没装、版本、有没有授权。版本读不到就留空，不单独报错。
func (rt *Runtime) Probe(ctx context.Context) (pkg.EngineStatus, error) {
	path, err := rt.lookPath(rt.bin)
	if err != nil {
		return pkg.EngineStatus{Hint: "本机没有安装 Codex CLI。"}, nil
	}
	out, _ := rt.version(ctx, path)
	status := pkg.EngineStatus{Available: true, Version: ParseVersion(out)}
	client, err := rt.ensureClient(ctx)
	if err != nil {
		status.Hint = err.Error()
		return status, nil
	}
	account, err := client.AccountRead(ctx)
	if err != nil {
		status.Hint = err.Error()
		return status, nil
	}
	status.Authorized = pkg.Authorized(account)
	if !status.Authorized {
		status.Hint = "本机 Codex 尚未取得授权，可以看选项但不能开回合。"
	}
	return status, nil
}

// ListModels 列出 Codex 允许选的模型。
func (rt *Runtime) ListModels(ctx context.Context) ([]pkg.ModelInfo, error) {
	client, err := rt.ensureClient(ctx)
	if err != nil {
		return nil, err
	}
	includeHidden := false
	var models []pkg.ModelInfo
	cursor := ""
	for {
		page, err := client.ModelList(ctx, pkg.ModelListParams{Cursor: cursor, IncludeHidden: &includeHidden, Limit: 100})
		if err != nil {
			return nil, mapRPC(err)
		}
		for _, raw := range page.Data {
			info, err := pkg.ParseModel(raw)
			if err != nil {
				continue
			}
			models = append(models, info)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if cfg, err := client.ConfigRead(ctx, ""); err == nil {
		models = pkg.MergeConfiguredModel(models, cfg)
	}
	return models, nil
}

// ListModes 列出 Plan 档和权限预设。
func (rt *Runtime) ListModes(ctx context.Context) ([]pkg.ModeInfo, error) {
	modes := pkg.CollaborationModes()
	client, err := rt.ensureClient(ctx)
	if err != nil {
		return modes, err
	}
	cursor := ""
	for {
		page, err := client.PermissionProfileList(ctx, pkg.CursorListParams{Cursor: cursor, Limit: 100})
		if err != nil {
			return modes, nil
		}
		for _, raw := range page.Data {
			info, err := pkg.ParsePermissionProfile(raw)
			if err != nil {
				continue
			}
			modes = append(modes, info)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	return modes, nil
}

// Commands 列出斜杠命令。
func (rt *Runtime) Commands() []pkg.CommandSpec {
	return pkg.Commands()
}

func (rt *Runtime) requireReady(ctx context.Context) (*pkg.Client, error) {
	status, err := rt.Probe(ctx)
	if err != nil {
		return nil, err
	}
	if !status.Available {
		return nil, cderr.Unavailable("%s", first(status.Hint, "codex is not installed"))
	}
	client, err := rt.ensureClient(ctx)
	if err != nil {
		return nil, err
	}
	if !status.Authorized {
		return nil, cderr.Unauthorized("%s", first(status.Hint, "codex is not authorized"))
	}
	return client, nil
}
