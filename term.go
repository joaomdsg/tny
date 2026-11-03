package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// SGR codes for style.
const (
	dim    = "2"
	red    = "1;31"
	green  = "1;32"
	yellow = "33"
	cyan   = "1;36"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// term styles stdout. During Ask the spinner runs in its own goroutine, so
// it and the tool log share mu to keep their writes from interleaving.
type term struct {
	tty   bool // spinner only on a terminal; piped output stays clean
	color bool
	wrote bool // the tool log printed during the current turn

	mu       sync.Mutex
	frameOn  bool // a spinner frame is on the current line
	stop     chan struct{}
	finished chan struct{}
}

func newTerm() *term {
	fi, err := os.Stdout.Stat()
	tty := err == nil && fi.Mode()&os.ModeCharDevice != 0
	// no-color.org: any non-empty NO_COLOR disables color.
	return &term{tty: tty, color: tty && os.Getenv("NO_COLOR") == ""}
}

func (t *term) style(code, s string) string {
	if !t.color {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

func (t *term) startSpinner() {
	t.wrote = false
	if !t.tty {
		return
	}
	t.stop, t.finished = make(chan struct{}), make(chan struct{})
	go func() {
		defer close(t.finished)
		tick := time.NewTicker(80 * time.Millisecond)
		defer tick.Stop()
		for i := 0; ; i++ {
			t.mu.Lock()
			fmt.Print("\r" + t.style(dim, "  "+spinnerFrames[i%len(spinnerFrames)]+" thinking"))
			t.frameOn = true
			t.mu.Unlock()
			select {
			case <-t.stop:
				return
			case <-tick.C:
			}
		}
	}()
}

func (t *term) stopSpinner() {
	if t.stop == nil {
		return
	}
	close(t.stop)
	<-t.finished
	t.stop = nil
	t.mu.Lock()
	t.clearFrame()
	t.mu.Unlock()
}

// clearFrame must be called with mu held.
func (t *term) clearFrame() {
	if t.frameOn {
		fmt.Print("\r\033[K")
		t.frameOn = false
	}
}

// Write is the tool log, dimmed. It clears the spinner frame first.
func (t *term) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.clearFrame()
	t.wrote = true
	body, nl := strings.CutSuffix(string(p), "\n")
	out := t.style(dim, body)
	if nl {
		out += "\n"
	}
	_, err := fmt.Print(out)
	return len(p), err
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}
