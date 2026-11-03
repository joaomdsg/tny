package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/joaomdsg/tny/agent"
)

// status is the line after each turn: labels dim, session cost yellow, and
// parts that would read zero left out (no cache segment without caching,
// no session cost for free local models).
func (t *term) status(turn, session agent.Usage, elapsed time.Duration) string {
	label := func(s string) string { return t.style(dim, s) }
	in, out, cacheR, cacheW := turn.Tokens()
	total := in + cacheR + cacheW

	parts := []string{fmt.Sprintf("%.1fs", elapsed.Seconds())}
	if n := turn.Calls(); n > 1 {
		parts = append(parts, fmt.Sprintf("%d%s", n, label(" calls")))
	}
	parts = append(parts, compact(total)+label(" in → ")+compact(out)+label(" out"))
	if cacheR+cacheW > 0 {
		parts = append(parts, fmt.Sprintf("%d%%%s", 100*cacheR/total, label(" cached")))
	}
	if cost := session.Cost(); cost > 0 {
		parts = append(parts, label("session ")+t.style(yellow, money(cost)))
	}
	return "  " + strings.Join(parts, label(" · "))
}

func compact(n int) string {
	switch {
	case n < 1_000:
		return fmt.Sprint(n)
	case n < 10_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	case n < 1_000_000:
		return fmt.Sprintf("%dk", n/1_000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	}
}

func money(x float64) string {
	if x < 1 {
		return fmt.Sprintf("$%.4f", x)
	}
	return fmt.Sprintf("$%.2f", x)
}
