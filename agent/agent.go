// Package agent is a conversation with a model that can run bash on this
// machine. Claude by default; any OpenAI-compatible server with WithOpenAI.
//
// Ask sends a prompt and returns the model's final text reply. Everything
// in between is hidden: the wire format, prompt-cache breakpoints, running
// each bash call the model makes and feeding its output back, rolling a
// failed turn out of the history so the conversation stays valid, and
// pricing what it all cost.
package agent

import (
	"io"
	"net/http"
)

const (
	defaultModel    = "claude-opus-5-5"
	defaultCacheTTL = "5m"
	defaultEndpoint = "https://api.anthropic.com/v1/messages"
	openAIEndpoint  = "https://api.openai.com/v1/chat/completions"
	defaultPriceIn  = 5.0
	defaultPriceOut = 25.0
)

// Agent holds one conversation. It is not safe for concurrent use.
type Agent struct {
	apiKey   string
	endpoint string
	openAI   bool
	model    string
	cacheTTL string
	priceIn  float64
	priceOut float64
	toolLog  io.Writer
	client   *http.Client

	history []message
	session Usage
}

// Option configures New.
type Option func(*Agent)

// WithModel selects the model (default claude-opus-5-5).
func WithModel(model string) Option { return func(a *Agent) { a.model = model } }

// WithCacheTTL sets the prompt-cache lifetime, "5m" (default) or "1h". 1h
// survives long pauses, but its cache writes cost 2x input instead of 1.25x.
func WithCacheTTL(ttl string) Option { return func(a *Agent) { a.cacheTTL = ttl } }

// WithPrice sets dollars per million input and output tokens (default 5,
// 25), which every Usage is priced at.
func WithPrice(in, out float64) Option {
	return func(a *Agent) { a.priceIn, a.priceOut = in, out }
}

// WithToolLog echoes each bash command to w before it runs, and its
// output after. By default nothing is echoed.
func WithToolLog(w io.Writer) Option { return func(a *Agent) { a.toolLog = w } }

// WithEndpoint sends requests to url instead of the public API, e.g. a
// proxy, a local model server, or a test server.
func WithEndpoint(url string) Option { return func(a *Agent) { a.endpoint = url } }

// WithOpenAI speaks the OpenAI Chat Completions format instead of the
// Messages API, for OpenAI and compatible servers (llama.cpp, vLLM,
// Ollama). The key is sent as a Bearer token, or not at all if empty.
// Without WithEndpoint, requests go to api.openai.com. There is no
// prompt-cache control; cached prompt tokens the server reports are priced
// as cache reads.
func WithOpenAI() Option { return func(a *Agent) { a.openAI = true } }

// New starts an empty conversation authenticated with apiKey.
func New(apiKey string, opts ...Option) *Agent {
	a := &Agent{
		apiKey:   apiKey,
		model:    defaultModel,
		cacheTTL: defaultCacheTTL,
		priceIn:  defaultPriceIn,
		priceOut: defaultPriceOut,
		toolLog:  io.Discard,
		client:   http.DefaultClient,
	}
	for _, opt := range opts {
		opt(a)
	}
	if a.endpoint == "" {
		a.endpoint = defaultEndpoint
		if a.openAI {
			a.endpoint = openAIEndpoint
		}
	}
	a.session = a.newUsage()
	return a
}

// Ask sends prompt and returns the model's text reply once it stops calling
// tools, with the Usage of this turn alone. On error the prompt and any
// partial exchange are dropped from the conversation, so the next Ask
// starts from where the last successful one ended; the Usage returned
// still counts the calls that were billed before the failure.
func (a *Agent) Ask(prompt string) (string, Usage, error) {
	turn := a.newUsage()
	defer func() { a.session.plus(turn) }()

	start := len(a.history)
	a.history = append(a.history, userText(prompt))
	for {
		resp, err := a.send()
		if err != nil {
			a.history = a.history[:start]
			return "", turn, err
		}
		turn.add(resp.Usage)
		a.history = append(a.history, message{Role: "assistant", Content: resp.Content})

		if resp.StopReason != "tool_use" {
			return replyText(resp.Content), turn, nil
		}
		a.history = append(a.history, a.runTools(resp.Content))
	}
}

// Reset forgets the conversation. Session usage is kept.
func (a *Agent) Reset() { a.history = nil }

// Session is the Usage of every Ask since New.
func (a *Agent) Session() Usage { return a.session }
