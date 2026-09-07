package claude

import "testing"

func TestSkeletonFlow(t *testing.T) {
	if _, err := Probe(); err != nil {
		t.Fatal(err)
	}
	if _, err := ListModels(); err != nil {
		t.Fatal(err)
	}
	if _, err := ListModes(); err != nil {
		t.Fatal(err)
	}
	if _, err := Create("local"); err != nil {
		t.Fatal(err)
	}
	if _, err := Effective(""); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply("", Settings{Model: "opus"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Invoke("", "model", "opus"); err != nil {
		t.Fatal(err)
	}
	if err := Mention("", "a.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := Start("", "hi", Input{Text: "hi"}, InputModeStart); err != nil {
		t.Fatal(err)
	}
	if err := AppendProgress("", "", Progress{Kind: ProgressKindText}); err != nil {
		t.Fatal(err)
	}
	if _, err := Require("", ApprovalAsk{Kind: AskKindCommand}); err != nil {
		t.Fatal(err)
	}
	if err := Decide("", AskAnswer{Approved: true, Scope: DecisionOnce}); err != nil {
		t.Fatal(err)
	}
	if err := Expire(""); err != nil {
		t.Fatal(err)
	}
	if err := RejectUnknown(""); err != nil {
		t.Fatal(err)
	}
	if err := Continue(""); err != nil {
		t.Fatal(err)
	}
	if _, err := Start("", "next", Input{Text: "next"}, InputModeQueue); err != nil {
		t.Fatal(err)
	}
	if err := Cancel(""); err != nil {
		t.Fatal(err)
	}
	if _, err := Invoke("", "branch", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Fork(""); err != nil {
		t.Fatal(err)
	}
	if _, err := Hydrate(""); err != nil {
		t.Fatal(err)
	}
	if _, err := Invoke("", "mcp", ""); err != nil {
		t.Fatal(err)
	}
}
