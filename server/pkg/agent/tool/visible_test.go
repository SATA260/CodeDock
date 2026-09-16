package tool

import (
	"context"
	"testing"
)

type namedStub struct{ name string }

func (s namedStub) Definition() Definition {
	return Definition{Name: s.name, Prompt: s.name}
}

func (s namedStub) Execute(context.Context, Input) (Result, error) {
	return Result{Name: s.name, Success: true}, nil
}

func TestVisibleDefinitions(t *testing.T) {
	all := []Definition{
		{Name: "ping", Permission: Permission{Effect: EffectAsk}},
		{Name: "memory_read", Permission: Permission{Effect: EffectAllow}},
		{Name: "memory_write", Permission: Permission{Effect: EffectAsk}},
		{Name: "plan_write", Permission: Permission{Effect: EffectAllow}},
	}
	got := namesOf(VisibleDefinitions(all, []string{"memory_read", "missing", "plan_write"}))
	if !sameNames(got, []string{"memory_read", "plan_write"}) {
		t.Fatalf("visible = %v", got)
	}
	if got := VisibleDefinitions(all, nil); len(got) != 0 {
		t.Fatalf("empty names should bind nothing, got %d", len(got))
	}
}

func TestDefinitionsStableOrder(t *testing.T) {
	reg := NewRegistry()
	for _, name := range []string{"write", "ping", "read"} {
		if err := reg.Register(namedStub{name}); err != nil {
			t.Fatal(err)
		}
	}
	var first []string
	for i := 0; i < 20; i++ {
		got := namesOf(Definitions(reg))
		if first == nil {
			first = got
			continue
		}
		if !sameNames(got, first) {
			t.Fatalf("order changed %v -> %v", first, got)
		}
	}
	if !sameNames(first, []string{"ping", "read", "write"}) {
		t.Fatalf("want alpha %v", first)
	}
}

func namesOf(defs []Definition) []string {
	out := make([]string, 0, len(defs))
	for _, def := range defs {
		out = append(out, def.Name)
	}
	return out
}

func sameNames(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
