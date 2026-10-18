package main

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

var API_KEY = os.Getenv("QUICK_CHECK_ANTHROPIC_API_KEY")
var model = cmp.Or(os.Getenv("TNY_MODEL"), "claude-opus-5-5")

var tools = []map[string]any{
	{
		"name":        "bash",
		"description": "run a bash command on the user's machine and return its combined output",
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "the bash command to run"},
			},
			"required": []string{"command"},
		},
	},
}

// cache breakpoint; TNY_CACHE_TTL=1h keeps the cache alive across long pauses (writes cost 2x vs 1.25x)
var eph = map[string]any{"type": "ephemeral", "ttl": cmp.Or(os.Getenv("TNY_CACHE_TTL"), "5m")}

// $ per million tokens (input, output); override with TNY_PRICE="in,out"
var priceIn, priceOut = 5.0, 25.0

func init() { fmt.Sscanf(os.Getenv("TNY_PRICE"), "%g,%g", &priceIn, &priceOut) }

type Usage struct {
	In     int `json:"input_tokens"`
	Out    int `json:"output_tokens"`
	CacheW int `json:"cache_creation_input_tokens"`
	CacheR int `json:"cache_read_input_tokens"`
	TTL    struct {
		W5m int `json:"ephemeral_5m_input_tokens"`
		W1h int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
	Calls int `json:"-"`
}

func (u *Usage) add(o Usage) {
	if o.TTL.W5m+o.TTL.W1h == 0 { // no TTL breakdown in response: treat writes as 5m
		o.TTL.W5m = o.CacheW
	}
	u.In, u.Out, u.CacheW, u.CacheR, u.Calls = u.In+o.In, u.Out+o.Out, u.CacheW+o.CacheW, u.CacheR+o.CacheR, u.Calls+o.Calls
	u.TTL.W5m, u.TTL.W1h = u.TTL.W5m+o.TTL.W5m, u.TTL.W1h+o.TTL.W1h
}

// cost uses standard multipliers: cache write 1.25x (5m) / 2x (1h) input, cache read 0.1x input
func (u Usage) cost() float64 {
	in := float64(u.In) + 1.25*float64(u.TTL.W5m) + 2*float64(u.TTL.W1h) + 0.1*float64(u.CacheR)
	return in*priceIn/1e6 + float64(u.Out)*priceOut/1e6
}

// saved is what the same tokens would cost uncached, minus actual cost (negative = caching cost more)
func (u Usage) saved() float64 {
	return float64(u.In+u.CacheW+u.CacheR)*priceIn/1e6 + float64(u.Out)*priceOut/1e6 - u.cost()
}

func (u Usage) String() string {
	hit := 0.0
	if t := u.In + u.CacheW + u.CacheR; t > 0 {
		hit = 100 * float64(u.CacheR) / float64(t)
	}
	return fmt.Sprintf("in %d | out %d | cache w %d (5m %d, 1h %d) r %d (%.0f%% hit) | %d calls | $%.4f (saved $%.4f)",
		u.In, u.Out, u.CacheW, u.TTL.W5m, u.TTL.W1h, u.CacheR, hit, u.Calls, u.cost(), u.saved())
}

type Response struct {
	Content    []map[string]any `json:"content"`
	StopReason string           `json:"stop_reason"`
	Usage      Usage            `json:"usage"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// runBash executes a shell command (120s timeout) and returns its combined output, truncated
func runBash(command string) string {
	b, err := exec.Command("timeout", "120", "bash", "-c", command).CombinedOutput()
	out := string(b)
	if len(out) > 10000 {
		out = out[:5000] + "\n[...truncated...]\n" + out[len(out)-5000:]
	}
	if err != nil {
		out += "\nexit: " + err.Error()
	}
	return out
}

// withCache returns a copy of history whose last block carries a cache breakpoint
func withCache(h []map[string]any) []map[string]any {
	h = append([]map[string]any{}, h...)
	last := h[len(h)-1]
	var blocks []map[string]any
	switch c := last["content"].(type) {
	case string:
		blocks = []map[string]any{{"type": "text", "text": c}}
	case []map[string]any:
		blocks = append([]map[string]any{}, c...)
	}
	b := map[string]any{"cache_control": eph}
	for k, v := range blocks[len(blocks)-1] {
		b[k] = v
	}
	blocks[len(blocks)-1] = b
	h[len(h)-1] = map[string]any{"role": last["role"], "content": blocks}
	return h
}

// turn sends one user prompt, running any tool calls until Claude replies with text.
// On error, history is rolled back to before the prompt so the session stays valid.
func turn(client *http.Client, history []map[string]any, prompt string, u *Usage) ([]map[string]any, string) {
	start := len(history)
	history = append(history, map[string]any{"role": "user", "content": prompt})

	for {
		b, _ := json.Marshal(map[string]any{
			"model":      model,
			"max_tokens": 20000,
			"messages":   withCache(history),
			"tools":      tools,
		})

		r, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(b))
		if err != nil {
			return history[:start], "Request build failed. err: " + err.Error()
		}
		r.Header.Set("content-type", "application/json")
		r.Header.Set("anthropic-version", "2023-06-01")
		r.Header.Set("x-api-key", API_KEY)

		resp, err := client.Do(r)
		if err != nil {
			return history[:start], "Request failed. err: " + err.Error()
		}

		out := Response{}
		json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			out.Usage.Calls = 1
			u.add(out.Usage)
		}
		if resp.StatusCode != http.StatusOK || out.Error != nil {
			msg := ""
			if out.Error != nil {
				msg = out.Error.Message
			}
			return history[:start], resp.Status + ": " + msg
		}

		history = append(history, map[string]any{"role": "assistant", "content": out.Content})

		if out.StopReason != "tool_use" {
			texts := []string{}
			for _, c := range out.Content {
				if c["type"] == "text" {
					texts = append(texts, c["text"].(string))
				}
			}
			return history, strings.Join(texts, "")
		}

		results := []map[string]any{}
		for _, c := range out.Content {
			if c["type"] != "tool_use" {
				continue
			}
			input, _ := c["input"].(map[string]any)
			cmd, _ := input["command"].(string)
			fmt.Printf("  [bash] $ %s\n", cmd)
			output := runBash(cmd)
			fmt.Printf("  -> %s\n", output)
			results = append(results, map[string]any{
				"type":        "tool_result",
				"tool_use_id": c["id"],
				"content":     output,
			})
		}
		history = append(history, map[string]any{"role": "user", "content": results})
	}
}

func main() {
	if API_KEY == "" {
		fmt.Println("QUICK_CHECK_ANTHROPIC_API_KEY not set")
		os.Exit(1)
	}

	client := http.DefaultClient
	history := []map[string]any{}
	session := Usage{}

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for {
		fmt.Print("you: ")
		if !in.Scan() {
			break
		}
		prompt := in.Text()
		if prompt == "" {
			continue
		}
		if prompt == "exit" {
			break
		}
		if prompt == "/clear" {
			history = nil
			continue
		}
		var reply string
		u := Usage{}
		history, reply = turn(client, history, prompt, &u)
		session.add(u)
		fmt.Println("claude: " + reply)
		fmt.Printf("[%s | session $%.4f (saved $%.4f)]\n", u, session.cost(), session.saved())
	}
	if err := in.Err(); err != nil {
		fmt.Println("stdin read failed. err: " + err.Error())
		os.Exit(1)
	}
}
