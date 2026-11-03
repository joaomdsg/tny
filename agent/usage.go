package agent

import "fmt"

// Usage is the tokens a turn or session used and what they cost.
type Usage struct {
	in, out, cacheW, cacheR int
	w5m, w1h                int
	calls                   int
	priceIn, priceOut       float64
}

type apiUsage struct {
	In     int      `json:"input_tokens"`
	Out    int      `json:"output_tokens"`
	CacheW int      `json:"cache_creation_input_tokens"`
	CacheR int      `json:"cache_read_input_tokens"`
	TTL    ttlUsage `json:"cache_creation"`
}

type ttlUsage struct {
	W5m int `json:"ephemeral_5m_input_tokens"`
	W1h int `json:"ephemeral_1h_input_tokens"`
}

func (a *Agent) newUsage() Usage {
	return Usage{priceIn: a.priceIn, priceOut: a.priceOut}
}

func (u *Usage) add(o apiUsage) {
	if o.TTL.W5m+o.TTL.W1h == 0 {
		// No TTL breakdown in the response: price writes as 5m.
		o.TTL.W5m = o.CacheW
	}
	u.in += o.In
	u.out += o.Out
	u.cacheW += o.CacheW
	u.cacheR += o.CacheR
	u.w5m += o.TTL.W5m
	u.w1h += o.TTL.W1h
	u.calls++
}

func (u *Usage) plus(o Usage) {
	u.in += o.in
	u.out += o.out
	u.cacheW += o.cacheW
	u.cacheR += o.cacheR
	u.w5m += o.w5m
	u.w1h += o.w1h
	u.calls += o.calls
}

// Cost is in dollars, with the standard cache multipliers: writes 1.25x
// input (5m) or 2x (1h), reads 0.1x input.
func (u Usage) Cost() float64 {
	in := float64(u.in) + 1.25*float64(u.w5m) + 2*float64(u.w1h) + 0.1*float64(u.cacheR)
	return in*u.priceIn/1e6 + float64(u.out)*u.priceOut/1e6
}

// Saved is what the same tokens would have cost uncached, minus Cost.
// Negative means caching cost more than it saved.
func (u Usage) Saved() float64 {
	uncached := float64(u.in+u.cacheW+u.cacheR)*u.priceIn/1e6 + float64(u.out)*u.priceOut/1e6
	return uncached - u.Cost()
}

// String is the one-line summary the REPL prints after each turn.
func (u Usage) String() string {
	hit := 0.0
	if t := u.in + u.cacheW + u.cacheR; t > 0 {
		hit = 100 * float64(u.cacheR) / float64(t)
	}
	return fmt.Sprintf("in %d | out %d | cache w %d (5m %d, 1h %d) r %d (%.0f%% hit) | %d calls | $%.4f (saved $%.4f)",
		u.in, u.out, u.cacheW, u.w5m, u.w1h, u.cacheR, hit, u.calls, u.Cost(), u.Saved())
}
