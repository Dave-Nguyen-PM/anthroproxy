# anthropool-proxy

A Go reverse proxy that pools multiple Claude OAuth (Bearer) tokens across a team, distributing requests round-robin and automatically rotating to the next token on a 429 rate-limit response.

## How it works

1. Team members point their Claude Code CLI at the proxy instead of `api.anthropic.com`.
2. The proxy picks the next available token from the pool (round-robin, skipping tokens in cooldown).
3. If the upstream returns `429 Too Many Requests`, the proxy marks that token in cooldown for a configurable duration (default 30 minutes) and transparently retries the request with the next available token.
4. All other headers are forwarded unchanged; only `Authorization` is replaced.

## Prerequisites

- Go 1.21+ (to build from source)
- One or more Claude Pro/Max Bearer tokens

## Build

```bash
git clone <repo>
cd anthropool-proxy
go build -o anthropool-proxy ./cmd/anthropool-proxy
```

Or install directly:

```bash
go install github.com/dtnguyen/anthropool-proxy/cmd/anthropool-proxy@latest
```

## Configuration

Config is stored at `~/.config/anthropool-proxy/config.json` (respects `$XDG_CONFIG_HOME`).

```json
{
  "tokens": [
    {
      "id": "a1b2c3d4e5f6a7b8",
      "label": "alice",
      "bearer_token": "sk-ant-..."
    },
    {
      "id": "deadbeefcafebabe",
      "label": "bob",
      "bearer_token": "sk-ant-..."
    }
  ],
  "listen": "0.0.0.0:8080",
  "cooldown_minutes": 30
}
```

You can edit this file directly or use the CLI commands.

## Commands

### Add a token

```bash
anthropool-proxy add alice
# Prompts: Enter bearer token for "alice":
```

The token is stored in config. Only the last 4 characters are shown in any output.

### List tokens

```bash
anthropool-proxy list
```

```
ID                       LABEL                TOKEN        STATUS
----------------------------------------------------------------------
a1b2c3d4e5f6a7b8         alice                ****ab12     ready
deadbeefcafebabe         bob                  ****cd34     ready
```

### Remove a token

```bash
anthropool-proxy remove alice
# or by ID:
anthropool-proxy remove a1b2c3d4e5f6a7b8
```

### Start the proxy

```bash
anthropool-proxy serve
```

### Show status

```bash
anthropool-proxy status
```

Note: live cooldown state is tracked in the running server process. Check server logs for real-time per-token status.

## Team setup

### Server side

1. Install and configure the proxy on a shared host (or each developer's machine).
2. Add each team member's Bearer token:

```bash
anthropool-proxy add alice
anthropool-proxy add bob
anthropool-proxy add carol
```

3. Start the proxy:

```bash
anthropool-proxy serve
```

### Client side

Each team member sets `ANTHROPIC_BASE_URL` in their Claude Code config:

```bash
# In ~/.claude/settings.json or via environment:
export ANTHROPIC_BASE_URL=http://proxy-host:8080
```

Or add to `~/.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://proxy-host:8080"
  }
}
```

Claude Code will now route all API calls through the proxy, which distributes them across the token pool.

## Getting Bearer tokens

Bearer tokens are stored in `~/.claude/.credentials.json` after logging in to Claude Code. The relevant field is `claudeAiOauth.accessToken`. Each team member runs Claude Code on their own machine, authenticates, and shares their access token with whoever manages the proxy config.

Example extraction:

```bash
cat ~/.claude/.credentials.json | python3 -c "import json,sys; d=json.load(sys.stdin); print(d['claudeAiOauth']['accessToken'])"
```

> **Note:** Access tokens expire and rotate. You may need to update the pool periodically when tokens expire (typically after a session or 24 hours). Watch the server logs for `401` responses as a signal that a token needs refreshing.

## Logging

Each request is logged to stdout:

```
[2024-01-15T10:23:45Z] token="alice" POST /v1/messages -> 200 (1.234s)
[2024-01-15T10:23:50Z] token="alice" rate-limited, marking cooldown
[2024-01-15T10:23:50Z] token="bob" POST /v1/messages -> 200 (0.987s)
```

## Running as a service

### systemd (Linux)

```ini
[Unit]
Description=anthropool-proxy
After=network.target

[Service]
ExecStart=/usr/local/bin/anthropool-proxy serve
Restart=always
User=anthropool

[Install]
WantedBy=multi-user.target
```

### launchd (macOS)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.anthropool.proxy</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/anthropool-proxy</string>
    <string>serve</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
</dict>
</plist>
```

Save to `~/Library/LaunchAgents/com.anthropool.proxy.plist` and load with:

```bash
launchctl load ~/Library/LaunchAgents/com.anthropool.proxy.plist
```

## Architecture

```
team member CLI
      │
      │ ANTHROPIC_BASE_URL=http://proxy:8080
      ▼
anthropool-proxy (Go reverse proxy)
      │
      │ picks next token from pool
      │ injects Authorization: Bearer <token>
      │
      ▼ on 429: mark cooldown, retry with next token
api.anthropic.com
```

- **No external dependencies** — stdlib only (`net/http/httputil`, `encoding/json`, etc.)
- **Atomic config writes** — tmp file + rename + flock for safe concurrent updates
- **Thread-safe pool** — sync.Mutex + atomic counters; safe for concurrent requests
