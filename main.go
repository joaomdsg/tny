// Command tny is a minimal terminal chat with Claude that can run bash.
//
// Environment:
//
//	QUICK_CHECK_ANTHROPIC_API_KEY  required
//	TNY_MODEL                      model name (default claude-opus-5-5)
//	TNY_CACHE_TTL                  prompt-cache lifetime, 5m or 1h
//	TNY_PRICE                      "in,out" dollars per million tokens
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
	a := agent.New(key, options()...)

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for {
		fmt.Print("you: ")
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
		default:
			reply, turn, err := a.Ask(prompt)
			if err != nil {
				fmt.Println("error: " + err.Error())
			} else {
				fmt.Println("claude: " + reply)
			}
			s := a.Session()
			fmt.Printf("[%s | session $%.4f (saved $%.4f)]\n", turn, s.Cost(), s.Saved())
		}
	}
	if err := in.Err(); err != nil {
		fmt.Println("stdin read failed: " + err.Error())
		os.Exit(1)
	}
}

func options() []agent.Option {
	opts := []agent.Option{agent.WithToolLog(os.Stdout)}
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
