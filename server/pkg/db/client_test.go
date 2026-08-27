package db

import (
	"context"
	"errors"
	"testing"
)

func TestOpenUnsupportedEngine(t *testing.T) {
	_, err := Open(context.Background(), Config{
		Engine: EnginePostgres,
		DSN:    "postgres://localhost",
	})
	if !errors.Is(err, ErrEngineUnsupported) {
		t.Fatalf("Open() error = %v, want %v", err, ErrEngineUnsupported)
	}
}

func TestOpenSQLite(t *testing.T) {
	client, err := Open(context.Background(), Config{
		Engine: EngineSQLite,
		DSN:    "file:codedock-test?mode=memory&cache=shared",
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	if client.Engine() != EngineSQLite {
		t.Fatalf("Engine() = %q, want %q", client.Engine(), EngineSQLite)
	}
	if client.DB() == nil {
		t.Fatal("DB() is nil")
	}
}
