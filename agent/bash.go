package agent

import (
	"errors"
	"fmt"
	"os/exec"
	"unicode/utf8"
)

const (
	bashToolName   = "bash"
	bashTimeoutSec = 120
	// timeout(1) exits 124 when it had to kill the command.
	timeoutExitCode = 124
	// Output past maxOutput bytes keeps its first and last keepEach bytes.
	maxOutput = 10000
	keepEach  = 5000
)

var bashTool = map[string]any{
	"name":        bashToolName,
	"description": "run a bash command on the user's machine and return its combined output",
	"input_schema": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string", "description": "the bash command to run"},
		},
		"required": []string{"command"},
	},
}

// runBash runs command with a timeout and returns its combined output,
// truncated, followed by how it exited if it failed.
func runBash(command string) string {
	b, err := exec.Command("timeout", fmt.Sprint(bashTimeoutSec), "bash", "-c", command).CombinedOutput()
	out := truncate(string(b))
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr) && exitErr.ExitCode() == timeoutExitCode:
		out += fmt.Sprintf("\nexit: killed after %ds timeout", bashTimeoutSec)
	case err != nil:
		out += "\nexit: " + err.Error()
	}
	return out
}

// truncate keeps both ends of long output, where the command and its
// result usually are. Cuts land on character boundaries so the result is
// valid UTF-8.
func truncate(s string) string {
	if len(s) <= maxOutput {
		return s
	}
	head, tail := keepEach, len(s)-keepEach
	for head > 0 && !utf8.RuneStart(s[head]) {
		head--
	}
	for tail < len(s) && !utf8.RuneStart(s[tail]) {
		tail++
	}
	return s[:head] + "\n[...truncated...]\n" + s[tail:]
}
