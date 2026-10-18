package agent

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRunBash_reportsANonZeroExit(t *testing.T) {
	t.Parallel()

	out := runBash("echo hi; exit 3")

	if !strings.Contains(out, "hi") || !strings.Contains(out, "exit status 3") {
		t.Errorf("out = %q, want output and exit status", out)
	}
}

func TestRunBash_truncatesLongOutputKeepingBothEnds(t *testing.T) {
	t.Parallel()

	out := runBash("printf 'A%.0s' $(seq 6000); printf 'B%.0s' $(seq 6000)")

	if !strings.HasPrefix(out, "AAAA") || !strings.HasSuffix(out, "BBBB") || !strings.Contains(out, "[...truncated...]") {
		t.Errorf("truncated output lost an end or the marker: %d bytes", len(out))
	}
}

func TestTruncate_neverSplitsAMultibyteCharacter(t *testing.T) {
	t.Parallel()

	out := truncate(strings.Repeat("é", 6000))

	if !utf8.ValidString(out) {
		t.Error("truncate produced invalid UTF-8")
	}
}
