package agent_test

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/joaomdsg/tny/agent"
)

func oaReply(msg map[string]any, finish string, usage map[string]any) fakeReply {
	b, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"message": msg, "finish_reason": finish}},
		"usage":   usage,
	})
	return fakeReply{http.StatusOK, string(b)}
}

func oaText(s string) fakeReply {
	return oaReply(map[string]any{"role": "assistant", "content": s}, "stop",
		map[string]any{"prompt_tokens": 10, "completion_tokens": 5})
}

func oaBash(id, command string) fakeReply {
	args, _ := json.Marshal(map[string]string{"command": command})
	return oaReply(map[string]any{
		"role":    "assistant",
		"content": nil,
		"tool_calls": []any{map[string]any{
			"id": id, "type": "function",
			"function": map[string]any{"name": "bash", "arguments": string(args)},
		}},
	}, "tool_calls", map[string]any{"prompt_tokens": 100, "completion_tokens": 20})
}

func newOpenAIAgent(url string, opts ...agent.Option) *agent.Agent {
	return newAgent(url, append([]agent.Option{agent.WithOpenAI()}, opts...)...)
}

func TestWithOpenAI_sendsAChatCompletionsRequestWithBearerAuth(t *testing.T) {
	t.Parallel()
	api, url := newFakeAPI(t, oaText("hello"))

	reply, _, err := newOpenAIAgent(url, agent.WithModel("m-1")).Ask("hi")

	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if reply != "hello" {
		t.Errorf("reply = %q, want %q", reply, "hello")
	}
	if got := api.headers[0].Get("Authorization"); got != "Bearer test-key" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer test-key")
	}
	req := api.requests[0]
	if req["model"] != "m-1" {
		t.Errorf("model = %v, want m-1", req["model"])
	}
	msg := api.messages(0)[0].(map[string]any)
	if msg["role"] != "user" || msg["content"] != "hi" {
		t.Errorf("first message = %v, want user text hi", msg)
	}
	tool := req["tools"].([]any)[0].(map[string]any)
	if tool["type"] != "function" || tool["function"].(map[string]any)["name"] != "bash" {
		t.Errorf("tool = %v, want function bash", tool)
	}
}

func TestWithOpenAI_sendsNoAuthHeaderWithoutAKey(t *testing.T) {
	t.Parallel()
	api, url := newFakeAPI(t, oaText("hello"))

	if _, _, err := agent.New("", agent.WithEndpoint(url), agent.WithOpenAI()).Ask("hi"); err != nil {
		t.Fatalf("Ask: %v", err)
	}

	if got := api.headers[0].Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want none", got)
	}
}

func TestWithOpenAI_runsToolCallsAndSendsResultsAsToolMessages(t *testing.T) {
	t.Parallel()
	api, url := newFakeAPI(t, oaBash("call_1", "echo from-bash"), oaText("done"))

	reply, _, err := newOpenAIAgent(url).Ask("run it")

	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if reply != "done" {
		t.Errorf("reply = %q, want done", reply)
	}
	msgs := api.messages(1)
	if len(msgs) != 3 {
		t.Fatalf("second request has %d messages, want user, assistant, tool: %v", len(msgs), msgs)
	}
	call := msgs[1].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	if call["id"] != "call_1" || !strings.Contains(call["function"].(map[string]any)["arguments"].(string), "echo from-bash") {
		t.Errorf("assistant tool call = %v, want call_1 echoing from-bash", call)
	}
	result := msgs[2].(map[string]any)
	if result["role"] != "tool" || result["tool_call_id"] != "call_1" || !strings.Contains(result["content"].(string), "from-bash") {
		t.Errorf("tool message = %v, want call_1's bash output", result)
	}
}

func TestWithOpenAI_pricesCachedPromptTokensAsCacheReads(t *testing.T) {
	t.Parallel()
	_, url := newFakeAPI(t, oaReply(map[string]any{"role": "assistant", "content": "ok"}, "stop",
		map[string]any{"prompt_tokens": 1000, "completion_tokens": 10,
			"prompt_tokens_details": map[string]any{"cached_tokens": 800}}))

	_, turn, err := newOpenAIAgent(url, agent.WithPrice(1, 1)).Ask("hi")

	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	// in 200 uncached + 0.1*800 cached = 280, out 10 → 290 tokens at $1/M
	if got := turn.Cost(); math.Abs(got-290e-6) > 1e-12 {
		t.Errorf("Cost = %v, want 290e-6", got)
	}
}

func TestWithOpenAI_returnsTheServersErrorMessage(t *testing.T) {
	t.Parallel()
	_, url := newFakeAPI(t, fakeReply{http.StatusBadRequest, `{"error":{"message":"no such model"}}`})

	_, _, err := newOpenAIAgent(url).Ask("hi")

	if err == nil || !strings.Contains(err.Error(), "no such model") {
		t.Errorf("err = %v, want the server's message", err)
	}
}
