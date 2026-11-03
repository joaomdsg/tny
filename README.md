# tny

Chat with Claude, or any OpenAI-compatible model, in your terminal.
It runs bash for you. One Go binary, stdlib only.

**Warning:** commands run as you, unsandboxed, no confirmation.
Use a VM or container.

## Run

```
go install github.com/joaomdsg/tny@latest
export QUICK_CHECK_ANTHROPIC_API_KEY=sk-ant-...
tny
```

```
you: free disk space on /?

  [bash] $ df -h /
  │ /dev/nvme0n1p2  468G  201G  265G  44% /

claude:
  265G free (44% used).

  1.8s · 2 calls · 120 in → 48 out · session $0.0018
```

`/clear` → new chat. `exit` → quit.

## Config (env vars)

- `TNY_MODEL`: default `claude-opus-5-5`
- `TNY_PRICE`: `in,out` $ per M tokens, default `5,25`.
  Set it when you change model.
- `TNY_CACHE_TTL`: `5m` (default) or `1h`
- `NO_COLOR`: set → no colors

## Local or OpenAI-compatible models

```
export TNY_API=openai TNY_PRICE=0,0
export TNY_ENDPOINT=http://127.0.0.1:8888/v1/chat/completions
export TNY_MODEL=<id from /v1/models> OPENAI_API_KEY=<key, if any>
tny
```

Works w/ llama.cpp, Unsloth, vLLM, Ollama, OpenAI.

## As a library

```go
a := agent.New(key)
reply, usage, err := a.Ask("list the files here")
```

## Dev

`go test -race ./...` (no API key needed).
Read [CONVENTIONS.md](./CONVENTIONS.md) first.
