// Command tny is a minimal terminal chat with a model that can run bash:
// Claude, or any OpenAI-compatible server.
//
// Environment:
//
//	QUICK_CHECK_ANTHROPIC_API_KEY  required, unless TNY_API=openai
//	TNY_API                        "openai" for OpenAI-compatible servers
//	OPENAI_API_KEY                 key for TNY_API=openai; optional
//	TNY_ENDPOINT                   API URL, e.g. a local model server
//	TNY_MODEL                      model name (default claude-opus-5-5)
//	TNY_CACHE_TTL                  prompt-cache lifetime, 5m or 1h
//	TNY_PRICE                      "in,out" dollars per million tokens
//	NO_COLOR                       any non-empty value disables color
//
// Type /clear to start a fresh conversation, exit to quit.
package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/joaomdsg/tny/agent"
)

func main() {
	openAI := os.Getenv("TNY_API") == "openai"
	key := os.Getenv("QUICK_CHECK_ANTHROPIC_API_KEY")
	if openAI {
		// Local servers often need no key, so an empty one is fine.
		key = os.Getenv("OPENAI_API_KEY")
	} else if key == "" {
		fmt.Println("QUICK_CHECK_ANTHROPIC_API_KEY not set")
		os.Exit(1)
	}
	name := "claude"
	if openAI {
		name = "model"
	}
	t := newTerm()
	a := agent.New(key, options(t, openAI)...)

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for {
		fmt.Print(t.style(cyan, "you: "))
		if !in.Scan() {
			break
		}
		switch prompt := in.Text(); prompt {
		case "":
			continue
		case "exit":
			return
		case "/clear":
			a.Reset()
			fmt.Println(t.style(dim, "  new conversation") + "\n")
		default:
			fmt.Println()
			t.startSpinner()
			start := time.Now()
			reply, turn, err := a.Ask(prompt)
			elapsed := time.Since(start)
			t.stopSpinner()
			if t.wrote {
				fmt.Println()
			}
			if err != nil {
				fmt.Println(t.style(red, "error: ") + err.Error())
			} else {
				fmt.Println(t.style(green, name+":"))
				fmt.Println(indent(reply))
			}
			fmt.Print("\n" + t.status(turn, a.Session(), elapsed) + "\n\n")
		}
	}
	if err := in.Err(); err != nil {
		fmt.Println("stdin read failed: " + err.Error())
		os.Exit(1)
	}
}

func options(t *term, openAI bool) []agent.Option {
	opts := []agent.Option{agent.WithToolLog(t)}
	if openAI {
		opts = append(opts, agent.WithOpenAI())
	}
	if url := os.Getenv("TNY_ENDPOINT"); url != "" {
		opts = append(opts, agent.WithEndpoint(url))
	}
	if m := os.Getenv("TNY_MODEL"); m != "" {
		opts = append(opts, agent.WithModel(m))
	}
	if ttl := os.Getenv("TNY_CACHE_TTL"); ttl != "" {
		opts = append(opts, agent.WithCacheTTL(ttl))
	}
	var in, out float64
	if n, _ := fmt.Sscanf(os.Getenv("TNY_PRICE"), "%g,%g", &in, &out); n == 2 {
		opts = append(opts, agent.WithPrice(in, out))
	}
	return opts
}
