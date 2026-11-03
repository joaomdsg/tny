// Command tny is a minimal terminal chat with Claude that can run bash.
//
// Environment:
//
//	QUICK_CHECK_ANTHROPIC_API_KEY  required
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

	"github.com/joaomdsg/tny/agent"
)

func main() {
	key := os.Getenv("QUICK_CHECK_ANTHROPIC_API_KEY")
	if key == "" {
		fmt.Println("QUICK_CHECK_ANTHROPIC_API_KEY not set")
		os.Exit(1)
	}
	t := newTerm()
	a := agent.New(key, options(t)...)

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
			reply, turn, err := a.Ask(prompt)
			t.stopSpinner()
			if t.wrote {
				fmt.Println()
			}
			if err != nil {
				fmt.Println(t.style(red, "error: ") + err.Error())
			} else {
				fmt.Println(t.style(green, "claude:"))
				fmt.Println(indent(reply))
			}
			s := a.Session()
			status := fmt.Sprintf("  [%s | session $%.4f (saved $%.4f)]", turn, s.Cost(), s.Saved())
			fmt.Print("\n" + t.style(dim, status) + "\n\n")
		}
	}
	if err := in.Err(); err != nil {
		fmt.Println("stdin read failed: " + err.Error())
		os.Exit(1)
	}
}

func options(t *term) []agent.Option {
	opts := []agent.Option{agent.WithToolLog(t)}
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
