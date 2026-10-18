package agent

import (
	"math"
	"testing"
)

func TestUsage_costAppliesCacheMultipliers(t *testing.T) {
	t.Parallel()
	u := Usage{priceIn: 1, priceOut: 10}
	u.add(apiUsage{In: 1_000_000, Out: 1_000_000, CacheW: 2_000_000, CacheR: 1_000_000,
		TTL: ttlUsage{W5m: 1_000_000, W1h: 1_000_000}})

	// in 1 + 1.25*1 (5m write) + 2*1 (1h write) + 0.1*1 (read) = 4.35, out 10
	if got := u.Cost(); math.Abs(got-14.35) > 1e-9 {
		t.Errorf("Cost = %v, want 14.35", got)
	}
	// uncached: (1+2+1)*1 + 10 = 14
	if got := u.Saved(); math.Abs(got-(14-14.35)) > 1e-9 {
		t.Errorf("Saved = %v, want -0.35", got)
	}
}

func TestUsage_treatsWritesWithoutATTLBreakdownAsFiveMinute(t *testing.T) {
	t.Parallel()
	u := Usage{priceIn: 1, priceOut: 1}
	u.add(apiUsage{CacheW: 1_000_000})

	if got := u.Cost(); math.Abs(got-1.25) > 1e-9 {
		t.Errorf("Cost = %v, want 1.25", got)
	}
}
