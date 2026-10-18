package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	apiVersion = "2023-06-01"
	maxTokens  = 20000
)

// message is one conversation turn. Content blocks stay raw so whatever
// Claude sent (including fields this package doesn't know about) goes back
// to the API exactly as received.
type message struct {
	Role    string            `json:"role"`
	Content []json.RawMessage `json:"content"`
}

// block is the subset of a content block this package reads.
type block struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type response struct {
	Content    []json.RawMessage `json:"content"`
	StopReason string            `json:"stop_reason"`
	Usage      apiUsage          `json:"usage"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// send posts the conversation so far and returns Claude's next message.
func (a *Agent) send() (response, error) {
	body, err := json.Marshal(map[string]any{
		"model":      a.model,
		"max_tokens": maxTokens,
		"messages":   withCache(a.history, a.cacheTTL),
		"tools":      []any{bashTool},
	})
	if err != nil {
		return response{}, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, a.endpoint, bytes.NewReader(body))
	if err != nil {
		return response{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("anthropic-version", apiVersion)
	req.Header.Set("x-api-key", a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return response{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var out response
	decodeErr := json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusOK || out.Error != nil {
		msg := ""
		if out.Error != nil {
			msg = out.Error.Message
		}
		return response{}, fmt.Errorf("%s: %s", resp.Status, msg)
	}
	if decodeErr != nil {
		return response{}, fmt.Errorf("unreadable response: %w", decodeErr)
	}
	return out, nil
}

// withCache returns a copy of history whose last block carries the cache
// breakpoint, so each request reads everything before it from cache and
// writes itself for the next one. history itself is never marked: a
// breakpoint left on an old block would count toward the API's limit.
func withCache(history []message, ttl string) []message {
	if len(history) == 0 {
		return history
	}
	out := append([]message(nil), history...)
	last := out[len(out)-1]
	content := append([]json.RawMessage(nil), last.Content...)
	if len(content) == 0 {
		return out
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(content[len(content)-1], &fields); err != nil {
		return out
	}
	fields["cache_control"], _ = json.Marshal(map[string]string{"type": "ephemeral", "ttl": ttl})
	marked, err := json.Marshal(fields)
	if err != nil {
		return out
	}
	content[len(content)-1] = marked
	out[len(out)-1] = message{Role: last.Role, Content: content}
	return out
}

func userText(prompt string) message {
	return message{Role: "user", Content: []json.RawMessage{mustJSON(map[string]string{
		"type": "text",
		"text": prompt,
	})}}
}

// replyText joins every text block in content.
func replyText(content []json.RawMessage) string {
	var sb strings.Builder
	for _, raw := range content {
		var b block
		if json.Unmarshal(raw, &b) == nil && b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}

// runTools executes every tool call in content and returns the user
// message carrying their results. An unknown tool is answered with an
// error result rather than dropped, since the API rejects a tool call
// left without a result.
func (a *Agent) runTools(content []json.RawMessage) message {
	var results []json.RawMessage
	for _, raw := range content {
		var b block
		if json.Unmarshal(raw, &b) != nil || b.Type != "tool_use" {
			continue
		}
		result := map[string]any{"type": "tool_result", "tool_use_id": b.ID}
		var input struct {
			Command string `json:"command"`
		}
		if b.Name != bashToolName || json.Unmarshal(b.Input, &input) != nil {
			result["content"] = fmt.Sprintf("unknown tool call %q", b.Name)
			result["is_error"] = true
		} else {
			fmt.Fprintf(a.toolLog, "  [bash] $ %s\n", input.Command)
			output := runBash(input.Command)
			fmt.Fprintf(a.toolLog, "  -> %s\n", output)
			result["content"] = output
		}
		results = append(results, mustJSON(result))
	}
	return message{Role: "user", Content: results}
}

// mustJSON marshals values built here from strings and maps, which cannot
// fail to encode.
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("agent: " + err.Error())
	}
	return b
}
