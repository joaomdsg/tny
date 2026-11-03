# tny

Chat with Claude in your terminal. It runs bash for you. One Go binary, zero deps.

**Warning:** commands run as you, unsandboxed, no confirmation. Use a VM or container.

## Run

```
go install github.com/joaomdsg/tny@latest
export QUICK_CHECK_ANTHROPIC_API_KEY=sk-ant-...
tny
```

```
you: free disk space on /?
  [bash] $ df -h /
claude: 265G free (44% used).
[in 12 | out 48 | ... | $0.0050 | session $0.0050]
```

`/clear` → new chat. `exit` → quit.

## Config (env vars)

- `TNY_MODEL`: default `claude-opus-5-5`
- `TNY_PRICE`: `in,out` $ per M tokens, default `5,25`. Set it when you change model.
- `TNY_CACHE_TTL`: `5m` (default) or `1h`

## As a library

```go
a := agent.New(key)
reply, usage, err := a.Ask("list the files here")
```

## Dev

`go test -race ./...` (no API key needed). Read [CONVENTIONS.md](./CONVENTIONS.md) first.
