package pipeline

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sys "ubuntu-dev-assistant/mcp"
)

func TestTracerSnapshot(t *testing.T) {
	tr := NewTracer()
	tr.Add("route_decision", "fast", "ok")
	steps := tr.Snapshot()
	if len(steps) != 1 || steps[0].Kind != "route_decision" {
		t.Fatalf("%+v", steps)
	}
}

func TestPickModel(t *testing.T) {
	r := NewRouter(NewClient("x"), nil)
	if m := r.pickModel("hi"); m != defaultFastModel {
		t.Fatalf("want fast, got %s", m)
	}
	if m := r.pickModel("please debug the failing go test"); m != defaultHeavyModel {
		t.Fatalf("want heavy, got %s", m)
	}
}

func TestOpenAIToolsFromRegistry(t *testing.T) {
	tools := openAITools(sys.DefaultRegistry())
	if len(tools) == 0 || tools[0].Type != "function" {
		t.Fatalf("%+v", tools)
	}
}

func TestChatCompletionMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Fatal("missing auth")
		}
		_ = json.NewEncoder(w).Encode(CompletionResponse{
			Choices: []struct {
				Index   int         `json:"index"`
				Message ChatMessage `json:"message"`
			}{{Message: ChatMessage{Role: "assistant", Content: "hello"}}},
		})
	}))
	defer srv.Close()

	c := NewClient("test-key")
	c.baseURL = srv.URL
	res, err := c.ChatCompletion(context.Background(), CompletionRequest{
		Model:    "test",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Choices[0].Message.Content != "hello" {
		t.Fatalf("%+v", res)
	}
}

func TestRouterToolLoopMock(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var msg ChatMessage
		if calls == 1 {
			msg = ChatMessage{
				Role: "assistant",
				ToolCalls: []ToolCall{{
					ID:   "1",
					Type: "function",
					Function: FunctionCall{
						Name:      "git_status",
						Arguments: `{"dir":"."}`,
					},
				}},
			}
		} else {
			msg = ChatMessage{Role: "assistant", Content: "repo looks clean"}
		}
		_ = json.NewEncoder(w).Encode(CompletionResponse{
			Choices: []struct {
				Index   int         `json:"index"`
				Message ChatMessage `json:"message"`
			}{{Message: msg}},
		})
	}))
	defer srv.Close()

	c := NewClient("test-key")
	c.baseURL = srv.URL
	r := NewRouter(c, sys.DefaultRegistry())
	out, err := r.Run(context.Background(), "check git status please")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Reply, "clean") {
		t.Fatalf("reply=%q trace=%+v", out.Reply, out.Trace)
	}
	if len(out.Trace) < 3 {
		t.Fatalf("expected trace steps, got %+v", out.Trace)
	}
}

func TestAPIErrorCodeNumber(t *testing.T) {
	raw := []byte(`{"error":{"message":"User not found.","code":401}}`)
	var out CompletionResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Error == nil || out.Error.Message != "User not found." {
		t.Fatalf("%+v", out.Error)
	}
}
