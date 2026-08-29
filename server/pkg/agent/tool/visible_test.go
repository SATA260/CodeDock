package tool

import "testing"

func TestModeCapabilities(t *testing.T) {
	cases := []struct {
		mode string
		want []Capability
	}{
		{mode: "plan", want: []Capability{CapabilityRead, CapabilityMemory}},
		{mode: "ask", want: []Capability{CapabilityRead, CapabilityMemory}},
		{mode: "ask_for_approval", want: []Capability{CapabilityRead, CapabilityWrite, CapabilityMemory}},
		{mode: "auto_approve", want: []Capability{CapabilityRead, CapabilityWrite, CapabilityMemory}},
		{mode: "yolo", want: []Capability{CapabilityRead, CapabilityWrite, CapabilityMemory}},
		{mode: "", want: []Capability{CapabilityRead, CapabilityMemory}},
	}
	for _, tc := range cases {
		got := ModeCapabilities(tc.mode)
		if !sameCaps(got, tc.want) {
			t.Fatalf("mode %q: got %v want %v", tc.mode, got, tc.want)
		}
	}
}

func TestCovers(t *testing.T) {
	read := []Capability{CapabilityRead}
	write := []Capability{CapabilityWrite}
	both := []Capability{CapabilityRead, CapabilityWrite}
	if !Covers(read, nil) {
		t.Fatal("empty tool caps should pass")
	}
	if !Covers(read, read) {
		t.Fatal("read should cover read")
	}
	if Covers(read, write) {
		t.Fatal("read should not cover write")
	}
	if !Covers(both, write) {
		t.Fatal("read+write should cover write")
	}
	if !Covers(both, both) {
		t.Fatal("read+write should cover both")
	}
	memory := []Capability{CapabilityMemory}
	if !Covers([]Capability{CapabilityRead, CapabilityMemory}, memory) {
		t.Fatal("read+memory should cover memory")
	}
	if Covers(read, memory) {
		t.Fatal("read should not cover memory")
	}
}

func TestVisibleDefinitions(t *testing.T) {
	all := []Definition{
		{Name: "ping", Permission: Permission{}},
		{Name: "memory_read", Permission: Permission{Capabilities: []Capability{CapabilityMemory}}},
		{Name: "memory_write", Permission: Permission{Capabilities: []Capability{CapabilityMemory}, RequiresApproval: true}},
		{Name: "memory_search", Permission: Permission{Capabilities: []Capability{CapabilityMemory}}},
		{Name: "file_write", Permission: Permission{Capabilities: []Capability{CapabilityWrite}}},
	}
	names := []string{"ping", "memory_read", "memory_write", "memory_search", "file_write"}

	plan := namesOf(VisibleDefinitions(all, names, ModeCapabilities("plan")))
	if !sameNames(plan, []string{"ping", "memory_read", "memory_write", "memory_search"}) {
		t.Fatalf("plan visible = %v", plan)
	}
	approve := namesOf(VisibleDefinitions(all, names, ModeCapabilities("ask_for_approval")))
	if !sameNames(approve, names) {
		t.Fatalf("ask_for_approval visible = %v", approve)
	}
	if got := VisibleDefinitions(all, nil, ModeCapabilities("auto_approve")); len(got) != 0 {
		t.Fatalf("empty names should bind nothing, got %d", len(got))
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

func sameCaps(got, want []Capability) bool {
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
