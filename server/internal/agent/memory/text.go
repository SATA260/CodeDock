package memory

import (
	"context"
	"database/sql"
	"errors"
	"time"

	cderr "codedock/internal/errors"
	"codedock/internal/util"
	"codedock/pkg/db/sqlite"
)

// Get 读取一篇目录或专题。
func Get(ctx context.Context, q *sqlite.Queries, key TextMemoryKey) (TextMemory, error) {
	key, err := normalizeKey(key)
	if err != nil {
		return TextMemory{}, err
	}
	if q == nil {
		return TextMemory{}, cderr.Invalid("queries required")
	}
	row, err := q.GetTextMemory(ctx, sqlite.GetTextMemoryParams{
		Scope:   string(key.Scope),
		ScopeID: key.ScopeID,
		Kind:    string(key.Kind),
		Name:    key.Name,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TextMemory{}, cderr.NotFound("text memory not found")
		}
		return TextMemory{}, err
	}
	return mapTextMemory(row), nil
}

// Upsert 覆盖写入一篇目录或专题。目录超限仍写入。
func Upsert(ctx context.Context, q *sqlite.Queries, item TextMemory) (TextMemory, error) {
	if item.Kind == "" {
		item.Kind = KindFromName(item.Name)
	}
	if item.Name == "" && item.Kind == KindIndex {
		item.Name = NameIndex
	}
	key, err := normalizeKey(TextMemoryKey{Scope: item.Scope, ScopeID: item.ScopeID, Kind: item.Kind, Name: item.Name})
	if err != nil {
		return TextMemory{}, err
	}
	if q == nil {
		return TextMemory{}, cderr.Invalid("queries required")
	}
	now := util.Now()
	if item.ID == "" {
		item.ID = util.NewID()
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	item.ByteLen = ByteLen(item.Content)
	row, err := q.UpsertTextMemory(ctx, sqlite.UpsertTextMemoryParams{
		ID:        item.ID,
		Scope:     string(key.Scope),
		ScopeID:   key.ScopeID,
		Kind:      string(key.Kind),
		Name:      key.Name,
		Content:   item.Content,
		ByteLen:   int64(item.ByteLen),
		CreatedAt: util.FormatTime(item.CreatedAt),
		UpdatedAt: util.FormatTime(item.UpdatedAt),
	})
	if err != nil {
		return TextMemory{}, err
	}
	return mapTextMemory(row), nil
}

// Delete 删除一篇目录或专题。
func Delete(ctx context.Context, q *sqlite.Queries, key TextMemoryKey) error {
	key, err := normalizeKey(key)
	if err != nil {
		return err
	}
	if q == nil {
		return cderr.Invalid("queries required")
	}
	return q.DeleteTextMemory(ctx, sqlite.DeleteTextMemoryParams{
		Scope:   string(key.Scope),
		ScopeID: key.ScopeID,
		Kind:    string(key.Kind),
		Name:    key.Name,
	})
}

// DeleteByScope 删除某一 scope 下的全部目录与专题。
func DeleteByScope(ctx context.Context, q *sqlite.Queries, scope TextMemoryScope, scopeID string) error {
	if _, err := normalizeKey(TextMemoryKey{Scope: scope, ScopeID: scopeID, Kind: KindIndex, Name: NameIndex}); err != nil {
		return err
	}
	if q == nil {
		return cderr.Invalid("queries required")
	}
	return q.DeleteTextMemoriesByScope(ctx, sqlite.DeleteTextMemoriesByScopeParams{
		Scope:   string(scope),
		ScopeID: scopeID,
	})
}

// List 列出某一 scope 下的目录与专题。
func List(ctx context.Context, q *sqlite.Queries, scope TextMemoryScope, scopeID string) ([]TextMemory, error) {
	if _, err := normalizeKey(TextMemoryKey{Scope: scope, ScopeID: scopeID, Kind: KindIndex, Name: NameIndex}); err != nil {
		return nil, err
	}
	if q == nil {
		return nil, cderr.Invalid("queries required")
	}
	rows, err := q.ListTextMemories(ctx, sqlite.ListTextMemoriesParams{Scope: string(scope), ScopeID: scopeID})
	if err != nil {
		return nil, err
	}
	out := make([]TextMemory, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapTextMemory(row))
	}
	return out, nil
}

func normalizeKey(key TextMemoryKey) (TextMemoryKey, error) {
	if key.Name == "" {
		key.Name = NameIndex
	}
	if key.Kind == "" {
		key.Kind = KindFromName(key.Name)
	}
	switch key.Scope {
	case ScopeUser, ScopeWorkspace:
	default:
		return key, cderr.Invalid("invalid memory scope")
	}
	if key.ScopeID == "" {
		return key, cderr.Invalid("scope_id is required")
	}
	switch key.Kind {
	case KindIndex:
		if key.Name != NameIndex {
			return key, cderr.Invalid("index name must be index")
		}
	case KindTopic:
		if key.Name == NameIndex {
			return key, cderr.Invalid("topic name cannot be index")
		}
	default:
		return key, cderr.Invalid("invalid memory kind")
	}
	return key, nil
}

func mapTextMemory(row sqlite.TextMemory) TextMemory {
	item := TextMemory{
		ID:        row.ID,
		Scope:     TextMemoryScope(row.Scope),
		ScopeID:   row.ScopeID,
		Kind:      TextMemoryKind(row.Kind),
		Name:      row.Name,
		Content:   row.Content,
		ByteLen:   int(row.ByteLen),
		CreatedAt: parseTime(row.CreatedAt),
		UpdatedAt: parseTime(row.UpdatedAt),
	}
	if item.Kind == KindIndex {
		item.OverBudget = IndexOverBudget(item.Content)
	}
	return item
}

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
