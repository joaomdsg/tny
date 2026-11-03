// Package agent is a conversation with Claude that can run bash on this
// machine.
//
// Ask sends a prompt and returns Claude's final text reply. Everything in
// between is hidden: the Messages API wire format, prompt-cache
// breakpoints, running each bash call Claude makes and feeding its output
// back, rolling a failed turn out of the history so the conversation stays
// valid, and pricing what it all cost.
package agent

import (
	"io"
	"net/http"
)

const (
	defaultModel    = "claude-opus-5-5"
	defaultCacheTTL = "5m"
	defaultEndpoint = "https://api.anthropic.com/v1/messages"
	defaultPriceIn  = 5.0
	defaultPriceOut = 25.0
)

// Agent holds one conversation. It is not safe for concurrent use.
type Agent struct {
	apiKey   string
	endpoint string
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

// WithModel selects the Claude model (default claude-opus-5-5).
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
// output after.
// By default nothing is echoed.
func WithToolLog(w io.Writer) Option { return func(a *Agent) { a.toolLog = w } }

// WithEndpoint sends requests to url instead of the public Messages API,
// e.g. a proxy or a test server.
func WithEndpoint(url string) Option { return func(a *Agent) { a.endpoint = url } }

// New starts an empty conversation authenticated with apiKey.
func New(apiKey string, opts ...Option) *Agent {
	a := &Agent{
		apiKey:   apiKey,
		endpoint: defaultEndpoint,
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
	a.session = a.newUsage()
	return a
}

// Ask sends prompt and returns Claude's text reply once it stops calling
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
