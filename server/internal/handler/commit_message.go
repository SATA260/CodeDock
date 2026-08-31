package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	cderr "codedock/internal/errors"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/git"
)

const conventionalCommitPrompt = `Write a GitHub Copilot-style commit message from the staged file list and diffs. Do not copy the example wording; write only from this staged change.

First line only: Conventional Commit type(scope): subject.
- type: feat, fix, docs, style, refactor, perf, test, chore, or ci
- add a scope only when one area is obvious
- subject: imperative, about 50 characters, no period, names the overall change

Then a blank line and 3 to 7 bullets, one change each:
- start every line with "- "
- past tense like Copilot: Added, Updated, Refactored, Introduced, Improved
- say what changed and why it matters, not every hunk
- skip lockfiles and generated files
- no fences, preamble, or signature

Example:
feat: enhance commit message generation and prompt management

- Added a PromptPicker for selecting and saving custom prompts.
- Updated the workspace panel to generate and apply drafts.
- Refactored backend prompt load and save.
- Introduced an API to generate messages from staged diffs.`
const draftMaxOutputTokens int64 = 320
const draftStreamTimeout = 8 * time.Second
const draftRequestTimeout = 9 * time.Second

const (
	promptIDConventional = "conventional"
	promptIDCustom       = "custom"
)

const patchBudgetTokens int64 = 1500
const perFilePatchTokens int64 = 300

type messagePack struct {
	Inventory string
	Patches   string
}

type promptStore struct {
	Selected string `json:"selected"`
	Custom   string `json:"custom"`
}

type gitPromptRequest struct {
	Selected string `json:"selected"`
	Custom   string `json:"custom"`
}

// PromptPreset 是产品预制的一份 system prompt。
type PromptPreset struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	SystemPrompt string `json:"system_prompt"`
}

// PromptConfig 是本仓当前选中的生成说明提示词整局。
type PromptConfig struct {
	Presets      []PromptPreset `json:"presets"`
	Selected     string         `json:"selected"`
	Custom       string         `json:"custom"`
	SystemPrompt string         `json:"system_prompt"`
}

func commitPromptPresets(custom string) []PromptPreset {
	return []PromptPreset{
		{ID: promptIDConventional, Name: "Conventional", SystemPrompt: conventionalCommitPrompt},
		{ID: promptIDCustom, Name: "自定义", SystemPrompt: custom},
	}
}

func validPromptID(id string) bool {
	switch id {
	case promptIDConventional, promptIDCustom:
		return true
	default:
		return false
	}
}

func resolvePrompt(store promptStore) string {
	if store.Selected == promptIDCustom && strings.TrimSpace(store.Custom) != "" {
		return store.Custom
	}
	return conventionalCommitPrompt
}

func promptConfigFrom(store promptStore) PromptConfig {
	if !validPromptID(store.Selected) {
		store.Selected = promptIDConventional
	}
	return PromptConfig{
		Presets:      commitPromptPresets(store.Custom),
		Selected:     store.Selected,
		Custom:       store.Custom,
		SystemPrompt: resolvePrompt(store),
	}
}

func (a *API) loadPromptStore(repo git.Repo) promptStore {
	path, err := codedockFile(repo, "commit-message.json")
	if err == nil {
		body, err := os.ReadFile(path)
		if err == nil && len(bytesTrimSpace(body)) > 0 {
			var store promptStore
			if json.Unmarshal(body, &store) == nil && (store.Selected != "" || store.Custom != "") {
				if !validPromptID(store.Selected) {
					store.Selected = promptIDConventional
				}
				return store
			}
		}
	}
	old, err := codedockFile(repo, "commit-message-prompt")
	if err == nil {
		body, err := os.ReadFile(old)
		if err == nil && strings.TrimSpace(string(body)) != "" {
			return promptStore{Selected: promptIDCustom, Custom: string(body)}
		}
	}
	return promptStore{Selected: promptIDConventional}
}

func bytesTrimSpace(body []byte) []byte {
	return []byte(strings.TrimSpace(string(body)))
}

func (a *API) savePromptStore(repo git.Repo, store promptStore) error {
	path, err := codedockFile(repo, "commit-message.json")
	if err != nil {
		return err
	}
	body, err := json.Marshal(store)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func (a *API) resolvedPrompt(repo git.Repo) string {
	return resolvePrompt(a.loadPromptStore(repo))
}

// GitGetPrompt 读本仓生成说明用的提示词整局。
func (a *API) GitGetPrompt(w http.ResponseWriter, r *http.Request) {
	repo, _, err := a.openSite(r.URL.Query().Get("checkout"))
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, promptConfigFrom(a.loadPromptStore(repo)))
}

// GitSetPrompt 保存当前选中的预制或自定义提示词。
func (a *API) GitSetPrompt(w http.ResponseWriter, r *http.Request) {
	var req gitPromptRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if !validPromptID(req.Selected) {
		writeError(w, cderr.Invalid("unknown prompt id"))
		return
	}
	repo, _, err := a.openSite("")
	if err != nil {
		writeGitError(w, err)
		return
	}
	store := promptStore{Selected: req.Selected, Custom: req.Custom}
	if err := a.savePromptStore(repo, store); err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, promptConfigFrom(store))
}

// GitGenerateMessage 读已暂存 diff，生成说明草稿。
func (a *API) GitGenerateMessage(w http.ResponseWriter, r *http.Request) {
	var req gitCheckoutRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	repo, co, err := a.openSite(req.Checkout)
	if err != nil {
		writeGitError(w, err)
		return
	}
	files, err := git.Diff(repo, co, "staged")
	if err != nil {
		writeGitError(w, err)
		return
	}
	if len(files) == 0 {
		writeError(w, cderr.Invalid("nothing staged"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), draftRequestTimeout)
	defer cancel()
	draft := a.generateDraft(ctx, repo, files)
	draft.Title = strings.TrimSpace(draft.Title)
	if draft.Title == "" {
		draft = parseDraft(fallbackDraft(files))
	}
	writeJSON(w, http.StatusOK, draft)
}

func (a *API) generateDraft(ctx context.Context, repo git.Repo, files []git.DiffFile) MessageDraft {
	prepared := make([]git.DiffFile, 0, len(files))
	for _, file := range files {
		class := classifyPatch(file)                       // 这份 diff 是 text / binary / generated / empty
		if class != "text" || isGeneratedPath(file.Path) { // lockfile、go.sum 不当正文
			file.Patch = ""
			prepared = append(prepared, file)
			continue
		}
		file.Patch = clipPatch(file.Patch, perFilePatchTokens) // 单文件超上限就截，标 truncated
		prepared = append(prepared, file)
	}

	pack := messagePack{
		Inventory: fileInventory(prepared),                      // 每个文件一行 kind+path，正文被砍也不丢
		Patches:   fillPatchBudget(prepared, patchBudgetTokens), // 只装 text，满预算停
	}
	text, ok := a.streamDraft(ctx, repo, pack) // 当前提示词 + 清单 + 正文，只打一次
	if !ok {
		return parseDraft(fallbackDraft(files)) // 模型失败：用文件列表凑一条
	}
	draft := parseDraft(text) // 第一行标题，其余正文
	if strings.TrimSpace(draft.Title) == "" {
		return parseDraft(fallbackDraft(files)) // 空标题同样回退
	}
	return draft
}

func classifyPatch(file git.DiffFile) string {
	if file.Binary {
		return "binary"
	}
	if isGeneratedPath(file.Path) {
		return "generated"
	}
	if strings.TrimSpace(file.Patch) == "" {
		return "empty"
	}
	return "text"
}

func isGeneratedPath(path string) bool {
	switch strings.ToLower(filepath.Base(path)) {
	case "pnpm-lock.yaml", "package-lock.json", "yarn.lock", "npm-shrinkwrap.json",
		"go.sum", "cargo.lock", "gemfile.lock", "poetry.lock", "composer.lock":
		return true
	default:
		return false
	}
}

func fileInventory(files []git.DiffFile) string {
	var b strings.Builder
	for _, file := range files {
		switch classifyPatch(file) {
		case "binary":
			b.WriteString("binary ")
		case "generated":
			b.WriteString("generated ")
		}
		b.WriteString(file.Kind)
		b.WriteByte(' ')
		b.WriteString(file.Path)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func fillPatchBudget(files []git.DiffFile, budget int64) string {
	sorted := append([]git.DiffFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	var b strings.Builder
	var used int64
	for _, file := range sorted {
		if classifyPatch(file) != "text" {
			continue
		}
		n := pkgagent.CountTokens(file.Patch)
		if used+n > budget {
			break
		}
		b.WriteString(file.Patch)
		b.WriteByte('\n')
		used += n
	}
	return strings.TrimSpace(b.String())
}

func clipPatch(patch string, max int64) string {
	if strings.TrimSpace(patch) == "" {
		return patch
	}
	if max <= 0 {
		return "... truncated"
	}
	if pkgagent.CountTokens(patch) <= max {
		return patch
	}
	limit := int(max * 4)
	if limit >= len(patch) {
		return patch
	}
	cut := patch[:limit]
	for !utf8.ValidString(cut) && len(cut) > 0 {
		cut = cut[:len(cut)-1]
	}
	return strings.TrimRight(cut, "\n") + "\n... truncated"
}

func (a *API) streamDraft(ctx context.Context, repo git.Repo, pack messagePack) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, draftStreamTimeout)
	defer cancel()
	var user strings.Builder
	if pack.Inventory != "" {
		user.WriteString(pack.Inventory)
		user.WriteByte('\n')
	}
	if pack.Patches != "" {
		user.WriteByte('\n')
		user.WriteString(pack.Patches)
	}
	stream, err := pkgagent.Stream(ctx, pkgagent.Chat{
		Model:           a.gitModel(),
		SystemPrompt:    a.resolvedPrompt(repo),
		MaxOutputTokens: draftMaxOutputTokens,
		Messages: []pkgagent.Message{{
			Role:    pkgagent.RoleUser,
			Content: pkgagent.EncodeText(user.String()),
		}},
	})
	if err != nil {
		return "", false
	}
	defer func() { _ = stream.Close() }()
	text := readDraftText(stream)
	if text == "" || text == "ok" {
		return "", false
	}
	return text, true
}

func readDraftText(stream pkgagent.ModelStream) string {
	var b strings.Builder
	for ev := range stream.Events() {
		if ev.Type == pkgagent.ModelStreamTextDelta {
			b.WriteString(pkgagent.DecodeText(ev.Delta))
		}
	}
	return strings.TrimSpace(b.String())
}

func parseDraft(text string) MessageDraft {
	text = strings.TrimSpace(text)
	title, body, ok := strings.Cut(text, "\n")
	if !ok {
		return MessageDraft{Title: text}
	}
	return MessageDraft{Title: strings.TrimSpace(title), Body: strings.TrimSpace(body)}
}

func fallbackDraft(files []git.DiffFile) string {
	if len(files) == 0 {
		return ""
	}
	title := "chore: update staged files"
	if len(files) == 1 {
		title = "chore: update " + files[0].Path
	}
	var body strings.Builder
	for _, file := range files {
		switch file.Kind {
		case "added":
			body.WriteString("- Added ")
		case "deleted":
			body.WriteString("- Removed ")
		case "renamed":
			body.WriteString("- Renamed ")
		default:
			body.WriteString("- Updated ")
		}
		body.WriteString(file.Path)
		body.WriteByte('\n')
	}
	return title + "\n\n" + strings.TrimSpace(body.String())
}
