package agent_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/joaomdsg/tny/agent"
)

// fakeAPI stands in for the Messages API: it records every request body
// and answers with the next scripted response.
type fakeAPI struct {
	t        *testing.T
	mu       sync.Mutex
	replies  []fakeReply
	requests []map[string]any
	headers  []http.Header
}

type fakeReply struct {
	status int
	body   string
}

func newFakeAPI(t *testing.T, replies ...fakeReply) (*fakeAPI, string) {
	t.Helper()
	f := &fakeAPI{t: t, replies: replies}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv.URL
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		f.t.Errorf("request body is not JSON: %v", err)
	}
	f.requests = append(f.requests, body)
	f.headers = append(f.headers, r.Header.Clone())
	if len(f.replies) == 0 {
		f.t.Errorf("unexpected request %d", len(f.requests))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	next := f.replies[0]
	f.replies = f.replies[1:]
	w.WriteHeader(next.status)
	io.WriteString(w, next.body)
}

func (f *fakeAPI) messages(i int) []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[i]["messages"].([]any)
}

func text(s string) fakeReply {
	b, _ := json.Marshal(map[string]any{
		"content":     []any{map[string]any{"type": "text", "text": s}},
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
	})
	return fakeReply{http.StatusOK, string(b)}
}

func bash(id, command string) fakeReply {
	b, _ := json.Marshal(map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": "running it"},
			map[string]any{"type": "tool_use", "id": id, "name": "bash", "input": map[string]any{"command": command}},
		},
		"stop_reason": "tool_use",
		"usage":       map[string]any{"input_tokens": 100, "output_tokens": 20},
	})
	return fakeReply{http.StatusOK, string(b)}
}

func newAgent(url string, opts ...agent.Option) *agent.Agent {
	return agent.New("test-key", append([]agent.Option{agent.WithEndpoint(url)}, opts...)...)
}

func TestAsk_returnsTheTextReply(t *testing.T) {
	t.Parallel()
	api, url := newFakeAPI(t, text("hello"))

	reply, _, err := newAgent(url, agent.WithModel("m-1")).Ask("hi")

	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if reply != "hello" {
		t.Errorf("reply = %q, want %q", reply, "hello")
	}
	if got := api.requests[0]["model"]; got != "m-1" {
		t.Errorf("model = %v, want m-1", got)
	}
	h := api.headers[0]
	if h.Get("x-api-key") != "test-key" || h.Get("anthropic-version") == "" {
		t.Errorf("missing auth/version headers: %v", h)
	}
}

func TestAsk_runsToolCallsAndSendsTheirOutputBack(t *testing.T) {
	t.Parallel()
	api, url := newFakeAPI(t, bash("tu_1", "echo from-bash"), text("done"))
	var log bytes.Buffer

	reply, _, err := newAgent(url, agent.WithToolLog(&log)).Ask("run it")

	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if reply != "done" {
		t.Errorf("reply = %q, want done", reply)
	}
	msgs := api.messages(1)
	result := msgs[len(msgs)-1].(map[string]any)["content"].([]any)[0].(map[string]any)
	if result["type"] != "tool_result" || result["tool_use_id"] != "tu_1" {
		t.Fatalf("last message is not the tool result: %v", result)
	}
	if !strings.Contains(result["content"].(string), "from-bash") {
		t.Errorf("tool result = %q, want bash output", result["content"])
	}
	if !strings.Contains(log.String(), "[bash] $ echo from-bash") {
		t.Errorf("tool log = %q, want the command echoed", log.String())
	}
}

func TestAsk_indentsEveryLineOfToolOutputInTheLog(t *testing.T) {
	t.Parallel()
	_, url := newFakeAPI(t, bash("tu_1", "printf 'a\\nb\\n'"), text("done"))
	var log bytes.Buffer

	if _, _, err := newAgent(url, agent.WithToolLog(&log)).Ask("run it"); err != nil {
		t.Fatalf("Ask: %v", err)
	}

	if want := "  │ a\n  │ b\n"; !strings.HasSuffix(log.String(), want) {
		t.Errorf("tool log = %q, want it to end with %q", log.String(), want)
	}
}

func TestAsk_putsOneCacheBreakpointOnTheLastBlock(t *testing.T) {
	t.Parallel()
	api, url := newFakeAPI(t, bash("tu_1", "true"), text("done"))

	if _, _, err := newAgent(url, agent.WithCacheTTL("1h")).Ask("go"); err != nil {
		t.Fatalf("Ask: %v", err)
	}

	for i := range 2 {
		marked := 0
		for m, msg := range api.messages(i) {
			for b, block := range msg.(map[string]any)["content"].([]any) {
				if cc, ok := block.(map[string]any)["cache_control"]; ok {
					marked++
					if cc.(map[string]any)["ttl"] != "1h" {
						t.Errorf("request %d: ttl = %v, want 1h", i, cc)
					}
					msgs := api.messages(i)
					last := msgs[len(msgs)-1].(map[string]any)["content"].([]any)
					if m != len(msgs)-1 || b != len(last)-1 {
						t.Errorf("request %d: breakpoint on message %d block %d, want the last block", i, m, b)
					}
				}
			}
		}
		if marked != 1 {
			t.Errorf("request %d: %d cache breakpoints, want exactly 1", i, marked)
		}
	}
}

func TestAsk_rollsBackAFailedTurnSoTheNextOneStartsClean(t *testing.T) {
	t.Parallel()
	api, url := newFakeAPI(t,
		fakeReply{http.StatusBadRequest, `{"error":{"message":"bad thing"}}`},
		text("ok"),
	)
	a := newAgent(url)

	_, _, err := a.Ask("first")
	if err == nil || !strings.Contains(err.Error(), "bad thing") {
		t.Fatalf("err = %v, want the API's message", err)
	}
	if _, _, err := a.Ask("second"); err != nil {
		t.Fatalf("second Ask: %v", err)
	}

	msgs := api.messages(1)
	if len(msgs) != 1 {
		t.Fatalf("second request carries %d messages, want only the new prompt", len(msgs))
	}
}

func TestAsk_rejectsAnUnreadableResponseInsteadOfStoringIt(t *testing.T) {
	t.Parallel()
	api, url := newFakeAPI(t, fakeReply{http.StatusOK, `not json`}, text("ok"))
	a := newAgent(url)

	if _, _, err := a.Ask("first"); err == nil {
		t.Fatal("Ask accepted a response it could not decode")
	}
	if _, _, err := a.Ask("second"); err != nil {
		t.Fatalf("second Ask: %v", err)
	}
	if n := len(api.messages(1)); n != 1 {
		t.Errorf("second request carries %d messages, want 1", n)
	}
}

func TestAsk_keepsTheConversationAcrossTurns(t *testing.T) {
	t.Parallel()
	api, url := newFakeAPI(t, text("one"), text("two"))
	a := newAgent(url)

	a.Ask("first")
	a.Ask("second")

	if n := len(api.messages(1)); n != 3 {
		t.Errorf("second request carries %d messages, want 3 (user, assistant, user)", n)
	}
}

func TestReset_forgetsTheConversation(t *testing.T) {
	t.Parallel()
	api, url := newFakeAPI(t, text("one"), text("two"))
	a := newAgent(url)

	a.Ask("first")
	a.Reset()
	a.Ask("second")

	if n := len(api.messages(1)); n != 1 {
		t.Errorf("request after Reset carries %d messages, want 1", n)
	}
}

func TestSession_accumulatesUsageAcrossTurns(t *testing.T) {
	t.Parallel()
	_, url := newFakeAPI(t, bash("tu_1", "true"), text("done"), text("again"))
	a := newAgent(url, agent.WithPrice(1, 1))

	_, turn, _ := a.Ask("first")
	a.Ask("second")

	if !strings.Contains(turn.String(), "| 2 calls |") {
		t.Errorf("turn usage = %q, want 2 calls", turn)
	}
	if !strings.Contains(a.Session().String(), "| 3 calls |") {
		t.Errorf("session usage = %q, want 3 calls", a.Session())
	}
}
