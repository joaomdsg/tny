package main

import "testing"

func TestCompact_shortensLargeCounts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		n    int
		want string
	}{
		{"under a thousand", 627, "627"},
		{"one decimal under ten thousand", 1495, "1.5k"},
		{"whole thousands above ten thousand", 12345, "12k"},
		{"millions", 1_250_000, "1.2M"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := compact(tt.n); got != tt.want {
				t.Errorf("compact(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}
