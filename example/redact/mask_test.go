package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/tool"
	sdk "codedock/pkg/plugin"
)

func TestMaskTextAssignmentKeepsKey(t *testing.T) {
	out, n := maskText("API_KEY=sk-1234")
	if n != 1 || !strings.HasPrefix(out, "API_KEY=«REDACTED:assignment:") || strings.Contains(out, "sk-1234") {
		t.Fatalf("got %q n=%d", out, n)
	}
}

func TestMaskTextAssignmentVariants(t *testing.T) {
	cases := []string{
		"export AWS_SECRET=wxyz",
		"password: hunter2",
		`TOKEN = "ghp_xxx"`,
	}
	for _, in := range cases {
		out, n := maskText(in)
		if n != 1 || strings.Contains(out, "wxyz") && strings.Contains(in, "wxyz") {
			t.Fatalf("%q -> %q n=%d", in, out, n)
		}
		if strings.Contains(out, "hunter2") || strings.Contains(out, "ghp_xxx") {
			t.Fatalf("value leaked: %q -> %q", in, out)
		}
		if !strings.Contains(out, "«REDACTED:assignment:") {
			t.Fatalf("want assignment placeholder: %q", out)
		}
	}
}

func TestMaskTextIgnoresPlainKeys(t *testing.T) {
	for _, in := range []string{"timeout=30", "user_name=alice"} {
		out, n := maskText(in)
		if n != 0 || out != in {
			t.Fatalf("%q -> %q n=%d", in, out, n)
		}
	}
}

func TestMaskTextShapes(t *testing.T) {
	aws := "AKIAIOSFODNN7EXAMPLE"
	out, n := maskText("id=" + aws)
	if n != 1 || strings.Contains(out, aws) || !strings.Contains(out, "«REDACTED:aws_ak:") {
		t.Fatalf("aws %q n=%d", out, n)
	}
	again, _ := maskText(aws)
	if again != token("aws_ak", aws) {
		t.Fatalf("stable token %q", again)
	}

	gh := "ghp_01234567890123456789"
	out, n = maskText(gh)
	if n != 1 || !strings.Contains(out, "«REDACTED:github:") {
		t.Fatalf("github %q n=%d", out, n)
	}

	sk := "sk-abcdefghijklmnopqrstuvwxyz"
	out, n = maskText(sk)
	if n != 1 || !strings.Contains(out, "«REDACTED:openai:") {
		t.Fatalf("openai %q n=%d", out, n)
	}
	short, n := maskText("see sk-xxx and sk-test")
	if n != 0 || short != "see sk-xxx and sk-test" {
		t.Fatalf("short sk %q n=%d", short, n)
	}

	slack := "xoxb-1234567890"
	out, n = maskText(slack)
	if n != 1 || !strings.Contains(out, "«REDACTED:slack:") {
		t.Fatalf("slack %q n=%d", out, n)
	}

	conn := "postgres://me:secret@host/db"
	out, n = maskText(conn)
	if n != 1 || strings.Contains(out, "me:secret") || !strings.HasPrefix(out, "postgres://«REDACTED:conn:") || !strings.Contains(out, "host/db") {
		t.Fatalf("conn %q n=%d", out, n)
	}
}

func TestMaskTextAssignmentWinsOverShape(t *testing.T) {
	in := "AWS_SECRET=" + "AKIAIOSFODNN7EXAMPLE"
	out, n := maskText(in)
	if n != 1 || !strings.Contains(out, "AWS_SECRET=«REDACTED:assignment:") || strings.Contains(out, "aws_ak") {
		t.Fatalf("overlap %q n=%d", out, n)
	}
}

func TestMaskTextPEM(t *testing.T) {
	pem := "-----BEGIN RSA PRIVATE KEY-----\nMIIE\n-----END RSA PRIVATE KEY-----"
	out, n := maskText("before\n" + pem + "\nafter")
	if n != 1 || strings.Contains(out, "MIIE") || !strings.Contains(out, "«REDACTED:pem:") || !strings.Contains(out, "before") {
		t.Fatalf("pem %q n=%d", out, n)
	}
	half := "-----BEGIN RSA PRIVATE KEY-----\nMIIE"
	out, n = maskText(half)
	if n != 0 || out != half {
		t.Fatalf("half pem %q n=%d", out, n)
	}
}

func TestMaskJSONSkipsImageData(t *testing.T) {
	raw := json.RawMessage(`{"type":"image","data":"AKIAIOSFODNN7EXAMPLE","mimeType":"image/png"}`)
	out, n := maskJSON(raw)
	if n != 0 || !strings.Contains(string(out), "AKIAIOSFODNN7EXAMPLE") {
		t.Fatalf("image %s n=%d", out, n)
	}
}

func TestMaskJSONWalksTextAndInvalid(t *testing.T) {
	raw := json.RawMessage(`{"content":[{"type":"text","text":"API_KEY=sk-1234"}]}`)
	out, n := maskJSON(raw)
	if n != 1 || strings.Contains(string(out), "sk-1234") || !strings.Contains(string(out), "API_KEY=") {
		t.Fatalf("json %s n=%d", out, n)
	}
	plain, n := maskJSON(json.RawMessage(`API_KEY=sk-1234 not-json`))
	if n != 1 || strings.Contains(string(plain), "sk-1234") {
		t.Fatalf("invalid %s n=%d", plain, n)
	}
}

func TestRewriteOutputDropsPathAndFooter(t *testing.T) {
	raw := json.RawMessage(`{"content":[{"type":"text","text":"API_KEY=sk-1234"}],"details":{"fullOutputPath":"/tmp/out"}}`)
	out := rewriteOutput(raw)
	if strings.Contains(string(out), "sk-1234") || strings.Contains(string(out), "fullOutputPath") || strings.Contains(string(out), "/tmp/out") {
		t.Fatalf("rewrite %s", out)
	}
	if !strings.Contains(string(out), "[redact] masked 1 secrets") {
		t.Fatalf("missing footer %s", out)
	}
	clean := rewriteOutput(json.RawMessage(`{"content":[{"type":"text","text":"ok"}],"details":{"fullOutputPath":"/tmp/out"}}`))
	if strings.Contains(string(clean), "fullOutputPath") || strings.Contains(string(clean), "[redact]") {
		t.Fatalf("path-only %s", clean)
	}
}

func TestPluginHooks(t *testing.T) {
	p := &plugin{}
	in, err := p.OnAgentInput(context.Background(), sdk.AgentInput{Content: "use AKIAIOSFODNN7EXAMPLE"})
	if err != nil || strings.Contains(in.Content, "AKIA") || !strings.Contains(in.Content, "«REDACTED:aws_ak:") {
		t.Fatalf("input %+v err=%v", in, err)
	}

	req, err := p.OnAgentRequest(context.Background(), sdk.AgentRequest{
		SystemPrompt: "key AKIAIOSFODNN7EXAMPLE",
		Messages: []pkgagent.Message{{
			Content: json.RawMessage(`{"text":"API_KEY=sk-1234"}`),
			ToolCalls: []tool.Call{{
				Arguments: json.RawMessage(`{"cmd":"echo AKIAIOSFODNN7EXAMPLE"}`),
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(req.SystemPrompt, "AKIA") {
		t.Fatalf("prompt %q", req.SystemPrompt)
	}
	if strings.Contains(string(req.Messages[0].Content), "sk-1234") {
		t.Fatalf("msg %s", req.Messages[0].Content)
	}
	if strings.Contains(string(req.Messages[0].ToolCalls[0].Arguments), "AKIAIOSFODNN7EXAMPLE") {
		t.Fatalf("args %s", req.Messages[0].ToolCalls[0].Arguments)
	}

	post, err := p.OnToolPostExecute(context.Background(), sdk.ToolPostExecute{
		Result: tool.Result{
			Success: true,
			Output:  json.RawMessage(`{"content":[{"type":"text","text":"API_KEY=sk-1234"}]}`),
			Error:   "boom AKIAIOSFODNN7EXAMPLE",
		},
	})
	if err != nil || !post.Result.Success {
		t.Fatalf("post %+v err=%v", post, err)
	}
	if strings.Contains(string(post.Result.Output), "sk-1234") || strings.Contains(post.Result.Error, "AKIA") {
		t.Fatalf("post leaked %+v", post.Result)
	}
}
