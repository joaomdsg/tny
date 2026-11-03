package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// History stays in Messages API shape. These translate it to and from the
// OpenAI Chat Completions format at the wire.

type oaMessage struct {
	Role       string       `json:"role"`
	Content    *string      `json:"content"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

type oaToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaResponse struct {
	Choices []struct {
		Message oaMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		Prompt     int `json:"prompt_tokens"`
		Completion int `json:"completion_tokens"`
		Details    struct {
			Cached int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (a *Agent) openAIRequest() map[string]any {
	var msgs []oaMessage
	for _, m := range a.history {
		msgs = append(msgs, toOpenAI(m)...)
	}
	return map[string]any{
		"model":      a.model,
		"max_tokens": maxTokens,
		"messages":   msgs,
		"tools": []any{map[string]any{"type": "function", "function": map[string]any{
			"name":        bashTool["name"],
			"description": bashTool["description"],
			"parameters":  bashTool["input_schema"],
		}}},
	}
}

// toOpenAI turns one history message into OpenAI messages. A user message
// of tool results becomes one "tool" message per result.
func toOpenAI(m message) []oaMessage {
	var text strings.Builder
	var calls []oaToolCall
	var results []oaMessage
	for _, raw := range m.Content {
		var b struct {
			block
			ToolUseID string `json:"tool_use_id"`
			Content   string `json:"content"`
		}
		if json.Unmarshal(raw, &b) != nil {
			continue
		}
		switch b.Type {
		case "text":
			text.WriteString(b.Text)
		case "tool_use":
			c := oaToolCall{ID: b.ID, Type: "function"}
			c.Function.Name, c.Function.Arguments = b.Name, string(b.Input)
			calls = append(calls, c)
		case "tool_result":
			content := b.Content
			results = append(results, oaMessage{Role: "tool", Content: &content, ToolCallID: b.ToolUseID})
		}
	}
	if len(results) > 0 {
		return results
	}
	s := text.String()
	msg := oaMessage{Role: m.Role, Content: &s, ToolCalls: calls}
	if s == "" && len(calls) > 0 {
		msg.Content = nil
	}
	return []oaMessage{msg}
}

// fromOpenAI turns a Chat Completions reply into a Messages API response.
func fromOpenAI(body []byte) (response, error) {
	var r oaResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return response{}, err
	}
	out := response{Usage: apiUsage{
		In:     r.Usage.Prompt - r.Usage.Details.Cached,
		Out:    r.Usage.Completion,
		CacheR: r.Usage.Details.Cached,
	}}
	if r.Error != nil {
		out.Error = &struct {
			Message string `json:"message"`
		}{r.Error.Message}
		return out, nil
	}
	if len(r.Choices) == 0 {
		return response{}, fmt.Errorf("no choices in reply")
	}
	msg := r.Choices[0].Message
	if msg.Content != nil && *msg.Content != "" {
		out.Content = append(out.Content, mustJSON(map[string]string{"type": "text", "text": *msg.Content}))
	}
	for i, c := range msg.ToolCalls {
		id := c.ID
		if id == "" {
			// Some local servers omit ids; the result still needs one to pair with.
			id = fmt.Sprintf("call_%d", i)
		}
		input := json.RawMessage(c.Function.Arguments)
		if !json.Valid(input) {
			// Keep history encodable; runTools then answers with an error result.
			input = mustJSON(c.Function.Arguments)
		}
		out.Content = append(out.Content, mustJSON(map[string]any{
			"type": "tool_use", "id": id, "name": c.Function.Name, "input": input,
		}))
	}
	// Servers disagree on finish_reason for tool calls, so go by the calls.
	out.StopReason = "end_turn"
	if len(msg.ToolCalls) > 0 {
		out.StopReason = "tool_use"
	}
	return out, nil
}
