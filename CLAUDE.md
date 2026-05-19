# anthroproxy

Local HTTP reverse proxy for `https://api.anthropic.com`. Round-robins requests across a pool of bearer tokens; on a 429 it puts that token in cooldown and retries with the next available one — transparent to the caller.

## Build & run

```bash
go build ./cmd/anthroproxy
./anthroproxy add <label>       # prompts for bearer token, saves to config
./anthroproxy serve             # listens on 0.0.0.0:8080 by default
./anthroproxy list              # show tokens (masked)
./anthroproxy remove <id|label>
./anthroproxy status            # config info; live cooldown state is in server logs
```

No external dependencies — stdlib only, `go 1.21`.

## Project structure

```
cmd/anthroproxy/main.go     CLI: subcommand dispatch and token management
internal/config/config.go   Config type, Load/Save, token masking
internal/pool/pool.go       Round-robin token pool with in-memory cooldown state
internal/proxy/proxy.go     http.Handler — proxies to api.anthropic.com with retry
```

## Architecture

```
request → proxy.ServeHTTP
    → pool.Next()           round-robin, skips tokens in cooldown
    → doRequest()           rewrites Authorization header, forwards
        → 429?  MarkCooldown + NextAfter → loop with next token
        → all exhausted?    return 429 to client
```

- **Config** — `~/.config/anthroproxy/config.json` (respects `XDG_CONFIG_HOME`). Fields: `tokens[]`, `listen`, `cooldown_minutes` (default 30). Writes are atomic (tmp + rename) with `flock`.
- **Pool** — in-memory only; cooldown state resets on server restart. `RequestCount` is `atomic.Int64` per token.
- **Proxy** — buffers request body once to replay on retry. Responses stream directly to client except for retriable 429s (body discarded).
- **CLI** — management commands (`add`, `list`, `remove`, `status`) only touch the config file; they do not communicate with a running server.

## Conventions

- Keep `go.mod` dependency-free.
- Locking: `sync.Mutex` for pool cursor and cooldown timestamps; `atomic.Int64` for counters.
- Config writes must go through `config.Save` — never write the file directly.
- Log format: `[RFC3339] token="label" METHOD /path -> STATUS (latency)`.
- New subcommands: add a `cmdXxx()` in `main.go`, register in the `switch` and `usage()`.
- `httputil.ReverseProxy.Director` is a no-op; all request construction is in `doRequest`. Don't add logic to `Director`.
- `WriteTimeout` is 10 min to support streaming — don't lower it without testing.

## Testing

No automated tests yet. To verify:

```bash
go build ./cmd/anthroproxy && go vet ./...
./anthroproxy add test && ./anthroproxy serve &
curl -s http://localhost:8080/v1/models -H "anthropic-version: 2023-06-01"
```

`internal/pool` has no I/O and is the best place to add unit tests first.
