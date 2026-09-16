package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReviewFake(t *testing.T) {
	calls := []ApprovalToolCall{{ID: "c1", Name: "ping"}}
	escalate, err := Review(context.Background(), ModelConfig{Provider: "fake", Model: "fake"}, calls)
	if err != nil || !escalate.Escalate {
		t.Fatalf("empty review should escalate: %+v %v", escalate, err)
	}

	failCfg := ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(FakeOptions{Review: &FakeReview{Fail: true}})}
	if _, err := Review(context.Background(), failCfg, calls); err == nil {
		t.Fatal("expected fail")
	}

	okCfg := ModelConfig{Provider: "fake", Model: "fake", Options: mustRaw(FakeOptions{Review: &FakeReview{
		Decisions: []ApprovalDecision{{ToolCallID: "c1", Status: ApprovalApproved, Reason: "ok"}},
	}})}
	got, err := Review(context.Background(), okCfg, calls)
	if err != nil || got.Escalate || len(got.Decisions) != 1 || got.Decisions[0].Status != ApprovalApproved {
		t.Fatalf("got=%+v err=%v", got, err)
	}

	bad, err := Review(context.Background(), ModelConfig{Provider: "other", Model: "x"}, calls)
	if err != nil || !bad.Escalate {
		t.Fatalf("unsupported should escalate: %+v %v", bad, err)
	}
}

func TestReviewerPromptStatesCriteria(t *testing.T) {
	prompt := reviewerPrompt()
	for _, want := range []string{"approved", "denied", "说不清", "破坏性", "JSON 数组"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestParseReviewContent(t *testing.T) {
	calls := []ApprovalToolCall{{ID: "c1"}, {ID: "c2"}}
	raw := `here [{"tool_call_id":"c1","status":"approved"},{"tool_call_id":"c2","status":"denied"}] done`
	got, ok := parseReviewContent(raw, calls)
	if !ok || len(got) != 2 {
		t.Fatalf("got=%v ok=%v", got, ok)
	}
	if _, ok := parseReviewContent("not json", calls); ok {
		t.Fatal("unclear should fail")
	}
}

func TestReviewCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Review(ctx, ModelConfig{Provider: "fake"}, []ApprovalToolCall{{ID: "c1"}}); err == nil {
		t.Fatal("expected cancel")
	}
}

func TestReviewOpenAI(t *testing.T) {
	calls := []ApprovalToolCall{{ID: "c1", Name: "ping"}, {ID: "c2", Name: "write"}}
	t.Run("escalate without key", func(t *testing.T) {
		got, err := Review(context.Background(), ModelConfig{Provider: "openai", Model: "gpt"}, calls)
		if err != nil || !got.Escalate {
			t.Fatalf("%+v %v", got, err)
		}
	})
	t.Run("approve from json", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"content": `[{"tool_call_id":"c1","status":"approved","reason":"ok"},{"tool_call_id":"c2","status":"denied","reason":"no"}]`,
					},
				}},
			})
		}))
		defer server.Close()
		opts, _ := json.Marshal(map[string]string{"api_key": "k", "base_url": server.URL})
		got, err := Review(context.Background(), ModelConfig{Provider: "openai", Model: "gpt", Options: opts}, calls)
		if err != nil || got.Escalate || len(got.Decisions) != 2 {
			t.Fatalf("%+v %v", got, err)
		}
	})
	t.Run("unclear content", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"I am not sure"}}]}`))
		}))
		defer server.Close()
		opts, _ := json.Marshal(map[string]string{"api_key": "k", "base_url": server.URL})
		got, err := Review(context.Background(), ModelConfig{Provider: "openai", Model: "gpt", Options: opts}, calls)
		if err != nil || !got.Escalate {
			t.Fatalf("%+v %v", got, err)
		}
	})
	t.Run("http error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer server.Close()
		opts, _ := json.Marshal(map[string]string{"api_key": "k", "base_url": server.URL})
		got, err := Review(context.Background(), ModelConfig{Provider: "openai", Model: "gpt", Options: opts}, calls)
		if err != nil || !got.Escalate {
			t.Fatalf("%+v %v", got, err)
		}
	})
	t.Run("empty calls", func(t *testing.T) {
		got, err := Review(context.Background(), ModelConfig{Provider: "fake"}, nil)
		if err != nil || !got.Escalate {
			t.Fatalf("%+v %v", got, err)
		}
	})
	t.Run("empty choices and invalid url", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"choices":[]}`))
		}))
		defer server.Close()
		opts, _ := json.Marshal(map[string]string{"api_key": "k", "base_url": server.URL})
		got, err := Review(context.Background(), ModelConfig{Provider: "openai", Model: "gpt", Options: opts}, calls)
		if err != nil || !got.Escalate {
			t.Fatalf("%+v %v", got, err)
		}
		opts, _ = json.Marshal(map[string]string{"api_key": "k", "base_url": "http://["})
		got, err = Review(context.Background(), ModelConfig{Provider: "openai", Model: "gpt", Options: opts}, calls)
		if err != nil || !got.Escalate {
			t.Fatalf("bad url %+v %v", got, err)
		}
		server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`not-json`))
		}))
		defer server2.Close()
		opts, _ = json.Marshal(map[string]string{"api_key": "k", "base_url": server2.URL})
		got, err = Review(context.Background(), ModelConfig{Provider: "openai", Model: "gpt", Options: opts}, calls)
		if err != nil || !got.Escalate {
			t.Fatalf("parse %+v %v", got, err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(30 * time.Millisecond)
		}))
		defer server.Close()
		opts, _ := json.Marshal(map[string]string{"api_key": "k", "base_url": server.URL})
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		_, err := Review(ctx, ModelConfig{Provider: "openai", Model: "gpt", Options: opts}, calls)
		if err == nil {
			t.Fatal("expected timeout")
		}
	})
}
