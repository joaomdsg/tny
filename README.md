# tny

A minimal terminal chat with Claude that can run bash on your machine. One
binary, standard library only. You type a prompt; Claude answers, running
as many bash commands as it needs along the way. Every turn prints what it
cost.

## Read this first: bash runs unsandboxed

Every command Claude runs goes to `bash -c` **on your machine, as you**,
with no confirmation prompt, no sandbox, and no network restriction. The
only guard is a 120-second timeout per command. Run tny where that is
acceptable: a throwaway VM, a container, or a directory you don't mind
losing.

## Vocabulary

- **turn**: one prompt you type and everything that follows until Claude
  replies with text. A turn can make several API calls.
- **call**: one request to the Messages API. Each tool round-trip is one
  call.
- **tool call**: a bash command Claude asks to run. Its output (stdout and
  stderr combined) goes back to Claude in the next call.
- **session**: every turn since tny started. `/clear` starts a new
  conversation but keeps counting session usage.

## Prerequisites

- Go (the version in `go.mod`) to build.
- `bash` and `timeout` (coreutils) on `PATH`.
- An Anthropic API key in `QUICK_CHECK_ANTHROPIC_API_KEY`.

## Quickstart

```
$ go build -o tny .
$ export QUICK_CHECK_ANTHROPIC_API_KEY=sk-ant-...
$ ./tny
you: how much free disk space is on /?
  [bash] $ df -h /
  -> Filesystem      Size  Used Avail Use% Mounted on
/dev/nvme0n1p2  468G  201G  265G  44% /

claude: You have 265G free on / (44% used).
[in 12 | out 48 | cache w 1830 (5m 1830, 1h 0) r 1702 (49% hit) | 2 calls
 | $0.0050 (saved $0.0011) | session $0.0050 (saved $0.0011)]
```

The `[bash]` lines echo each command and its output as it runs. The status
line prints as one line; it is wrapped above to fit.

## Configuration

All configuration is through environment variables:

- `QUICK_CHECK_ANTHROPIC_API_KEY` (required): the API key. tny exits
  immediately if it isn't set.
- `TNY_MODEL`: model name. Default `claude-opus-5-5`.
- `TNY_CACHE_TTL`: prompt-cache lifetime, `5m` (default) or `1h`. `1h`
  holds up across long pauses between prompts, but cache writes cost 2x
  input instead of 1.25x.
- `TNY_PRICE`: `in,out` dollars per million tokens, e.g. `TNY_PRICE=3,15`.
  Default `5,25`. Set this when you change `TNY_MODEL`. It only changes
  the cost figures tny reports, not what you are billed.

## Commands at the prompt

- `/clear`: forget the conversation and start a fresh one. Session cost
  keeps accumulating.
- `exit` (or EOF, `Ctrl-D`): quit.
- An empty line is ignored. Anything else is sent to Claude.

## Reading the status line

Every turn ends with a status line, printed even if the turn failed:

- `in` / `out`: uncached input tokens and output tokens.
- `cache w`: tokens written to the prompt cache, split by TTL.
- `r`: tokens read from cache. The `% hit` is reads over all input.
- `calls`: API requests made during the turn.
- `$`: cost of the turn at `TNY_PRICE`, with cache multipliers applied
  (writes 1.25x input for 5m and 2x for 1h, reads 0.1x).
- `saved`: what the same tokens would have cost uncached, minus the actual
  cost. Negative means caching cost more than it saved, which is normal
  for a short session's first turn.
- `session`: the same cost and savings, totaled across every turn.

## How it works

- **Caching.** Every request moves a single cache breakpoint to the last
  block of the conversation. Each call then reads the earlier conversation
  from cache and writes only the new part.
- **Tool loop.** While Claude stops with `tool_use`, tny runs each bash
  call, sends the results back, and calls again. A tool tny doesn't know
  gets an error result rather than being dropped, because the API rejects
  a tool call that has no result.
- **Failed turns roll back.** If any call in a turn fails (network, non-200,
  unreadable body), the prompt and the partial exchange are removed from
  history, and the next prompt continues from the last successful turn.
  Calls billed before the failure still count in the status line.
- **Output limits.** Command output past 10,000 bytes keeps its first and
  last 5,000 bytes, cut on UTF-8 boundaries. A command killed by the
  timeout says so in its output. Replies are capped at 20,000 tokens.

## Using the agent package

`main.go` contains only the REPL. The conversation itself is the `agent`
package, and you can use it on its own:

```go
a := agent.New(key, agent.WithModel("claude-opus-5-5"))
reply, turn, err := a.Ask("list the files here")
fmt.Println(reply, turn)  // turn is this Ask's Usage
a.Reset()                 // forget the conversation
fmt.Println(a.Session().Cost())
```

Options: `WithModel`, `WithCacheTTL`, `WithPrice`, `WithToolLog(w)` (echo
bash commands to `w`; silent by default), and `WithEndpoint(url)` (a
proxy, or an `httptest` server in tests). An `Agent` is not safe for
concurrent use.

## Development

```
$ go test -race ./...
```

Tests run against an `httptest` fake of the Messages API, so they need no
API key or network. They do run real bash. Read
[CONVENTIONS.md](./CONVENTIONS.md) before changing code.

## Troubleshooting

**`QUICK_CHECK_ANTHROPIC_API_KEY not set`**: export the key in the shell
that runs tny. No other variable is read for it.

**`error: 401 Unauthorized: ...` (or any status line)**: the API rejected
the call. The message after the colon is the API's own. The turn was
rolled back, so fix the cause and send the prompt again.

**The costs look wrong.** They are computed from `TNY_PRICE`, which
defaults to 5/25 regardless of `TNY_MODEL`. Set it to your model's
price.

**A command shows `exit: killed after 120s timeout`.** Claude ran
something that didn't finish (a server, a `watch`, a prompt waiting on
stdin). Ask it to run the command in the background or with a bounded
duration.
