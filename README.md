```
 █████╗  ███╗   ██╗ ████████╗ ██╗  ██╗ ██████╗   ██████╗  ██████╗  ██████╗   ██████╗  ██╗  ██╗ ██╗   ██╗
██╔══██╗ ████╗  ██║ ╚══██╔══╝ ██║  ██║ ██╔══██╗ ██╔═══██╗ ██╔══██╗ ██╔══██╗ ██╔═══██╗ ╚██╗██╔╝ ╚██╗ ██╔╝
███████║ ██╔██╗ ██║    ██║    ███████║ ██████╔╝ ██║   ██║ ██████╔╝ ██████╔╝ ██║   ██║  ╚███╔╝   ╚████╔╝ 
██╔══██║ ██║╚██╗██║    ██║    ██╔══██║ ██╔══██╗ ██║   ██║ ██╔═══╝  ██╔══██╗ ██║   ██║  ██╔██╗    ╚██╔╝  
██║  ██║ ██║ ╚████║    ██║    ██║  ██║ ██║  ██║ ╚██████╔╝ ██║      ██║  ██║ ╚██████╔╝ ██╔╝ ██╗    ██║   
╚═╝  ╚═╝ ╚═╝  ╚═══╝    ╚═╝    ╚═╝  ╚═╝ ╚═╝  ╚═╝  ╚═════╝  ╚═╝      ╚═╝  ╚═╝  ╚═════╝  ╚═╝  ╚═╝    ╚═╝
```

<p align="center">
  Team proxy that pools Claude Pro/Max seats — auto-rotates on rate limits.
</p>

<p align="center">
  <a href="https://github.com/Dave-Nguyen-PM/anthroproxy/releases/latest">
    <img alt="latest release"
         src="https://img.shields.io/github/v/release/Dave-Nguyen-PM/anthroproxy?style=for-the-badge&label=Release&color=c8763a&labelColor=0e1116&logo=github&logoColor=f0ead6">
  </a>
</p>

---

**`anthroproxy`** is a Go reverse proxy that pools your team's Claude Pro/Max OAuth tokens. Heavy users draw from the whole pool instead of blocking on their own seat — rate limits are handled transparently with automatic token rotation. No logout, no reconfiguration, no lost work.

```text
$ anthroproxy serve
anthroproxy: rate limit on alice@example.com → rotated to bob@example.com, retrying…
[2026-05-20T09:00:05Z] token="bob" POST /v1/messages -> 200 (0.99s)
```

---

## Install

Three options — all produce the same `anthroproxy` binary.

### Option 1 — pre-built binary (recommended)

Download the binary for your platform from the [releases page](https://github.com/Dave-Nguyen-PM/anthroproxy/releases/latest):

| Platform | File |
|---|---|
| macOS (Apple Silicon) | `anthroproxy-darwin-arm64` |
| macOS (Intel) | `anthroproxy-darwin-amd64` |
| Linux (amd64) | `anthroproxy-linux-amd64` |
| Linux (arm64) | `anthroproxy-linux-arm64` |
| Windows (amd64) | `anthroproxy-windows-amd64.exe` |

**macOS / Linux:**
```bash
# Replace <version> and <platform> as appropriate
curl -L https://github.com/Dave-Nguyen-PM/anthroproxy/releases/latest/download/anthroproxy-darwin-arm64 -o anthroproxy
chmod +x anthroproxy
sudo mv anthroproxy /usr/local/bin/
```

**Windows:** download the `.exe`, rename to `anthroproxy.exe`, and move it to any directory on your `Path`.

### Option 2 — go install

Requires Go 1.21+.

```bash
go install github.com/Dave-Nguyen-PM/anthroproxy/cmd/anthroproxy@latest
```

### Option 3 — build from source

```bash
git clone https://github.com/Dave-Nguyen-PM/anthroproxy
cd anthroproxy
go build -o anthroproxy ./cmd/anthroproxy
sudo mv anthroproxy /usr/local/bin/
```

---

## Contents

- [Install](#install)
- [Admin guide](#admin-guide)
  - [Collect tokens from team members](#collect-tokens-from-team-members)
  - [Configure and start the proxy](#configure-and-start-the-proxy)
  - [Keep the proxy running](#keep-the-proxy-running)
  - [Maintain the token pool](#maintain-the-token-pool)
- [Team member guide](#team-member-guide)
  - [Find your bearer token](#find-your-bearer-token)
  - [Configure Claude Code CLI](#configure-claude-code-cli)
  - [Configure Claude Code in VS Code](#configure-claude-code-in-vs-code)
  - [Verify it is working](#verify-it-is-working)
  - [Revert to direct access](#revert-to-direct-access)
- [Reference](#reference)
  - [CLI commands](#cli-commands)
  - [Config file](#config-file)
  - [Log format](#log-format)

---

## Admin guide

### Build and install

Requires Go 1.21 or newer.

```bash
git clone <repo-url> anthroproxy
cd anthroproxy
go build -o anthroproxy ./cmd/anthroproxy

# Move the binary somewhere on your PATH
sudo mv anthroproxy /usr/local/bin/
```

Verify:

```bash
anthroproxy --help
```

---

### Collect tokens from team members

Each team member must extract their **Bearer token** from the machine where they have Claude Code installed and send it to you securely (password manager share, 1Password, etc. — not Slack or email).

Tell each team member to run **one** of the following on their own machine:

**macOS (reads from Keychain — most reliable):**
```bash
security find-generic-password -a "claude.ai" -s "claude.ai" -w 2>/dev/null \
  || security find-internet-password -s "claude.ai" -w 2>/dev/null \
  || security find-generic-password -s "Claude" -w
```

**macOS / Linux (reads credentials file):**
```bash
cat ~/.claude/.credentials.json \
  | python3 -c "import json,sys; d=json.load(sys.stdin); print(d['claudeAiOauth']['accessToken'])"
```

**Any platform — grab from a live network request:**
1. Open [claude.ai](https://claude.ai) in Chrome.
2. Open DevTools → **Network** tab.
3. Send any message.
4. Click any `/api/` request → **Headers** → copy the value after `Authorization: Bearer `.

The token looks like `sk-ant-oaut...` and is typically several hundred characters long.

> **Important:** Tokens expire (usually within 24 hours or on next login). Each team member should re-extract and re-send their token whenever you see `401` errors in the proxy logs for their label.

---

### Configure and start the proxy

**Add each token to the pool:**

```bash
anthroproxy add alice
# Paste alice's token at the prompt and press Enter

anthroproxy add bob
anthroproxy add carol
```

**Confirm the pool looks right:**

```bash
anthroproxy list
```

```
ID                       LABEL    TOKEN        STATUS
-------------------------------------------------------
a1b2c3d4e5f6a7b8         alice    ****ab12     ready
deadbeefcafebabe         bob      ****cd34     ready
1234567890abcdef         carol    ****ef56     ready
```

**Start the proxy:**

```bash
anthroproxy serve
```

The proxy listens on `0.0.0.0:8080` by default. It logs every request to stdout:

```
[2026-05-20T09:00:01Z] token="alice" POST /v1/messages -> 200 (1.23s)
[2026-05-20T09:00:05Z] token="alice" rate-limited, entering 30m cooldown
[2026-05-20T09:00:05Z] token="bob"   POST /v1/messages -> 200 (0.99s)
```

**Change the listen address or cooldown duration** by editing `~/.config/anthroproxy/config.json`:

```json
{
  "listen": "0.0.0.0:9000",
  "cooldown_minutes": 60
}
```

Then restart the proxy.

---

### Keep the proxy running

**macOS — launchd**

Create `~/Library/LaunchAgents/com.anthroproxy.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.anthroproxy</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/anthroproxy</string>
    <string>serve</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>/tmp/anthroproxy.log</string>
  <key>StandardErrorPath</key>
  <string>/tmp/anthroproxy.log</string>
</dict>
</plist>
```

```bash
launchctl load ~/Library/LaunchAgents/com.anthroproxy.plist
launchctl start com.anthroproxy
```

**Linux — systemd**

Create `/etc/systemd/system/anthroproxy.service`:

```ini
[Unit]
Description=anthroproxy
After=network.target

[Service]
ExecStart=/usr/local/bin/anthroproxy serve
Restart=always
User=YOUR_USER

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now anthroproxy
sudo journalctl -fu anthroproxy   # follow logs
```

---

### Maintain the token pool

**Remove a token** (e.g. when someone leaves the team):

```bash
anthroproxy remove alice
# or by ID:
anthroproxy remove a1b2c3d4e5f6a7b8
```

**Update a token** (when a token expires — you will see `401` in the logs for that label):

```bash
anthroproxy remove alice
anthroproxy add alice   # paste the fresh token
```

Then restart the proxy (or send `SIGHUP` if you add graceful-reload support later).

---

## Team member guide

You have two things to do:

1. **Extract your Bearer token** and send it securely to your admin.
2. **Point Claude Code at the proxy** on your machine.

---

### Find your bearer token

Run **one** of the following on your machine and send the output to your admin via a secure channel.

**macOS — Keychain (try this first):**
```bash
security find-generic-password -a "claude.ai" -s "claude.ai" -w 2>/dev/null \
  || security find-internet-password -s "claude.ai" -w 2>/dev/null \
  || security find-generic-password -s "Claude" -w
```

**macOS / Linux — credentials file:**
```bash
cat ~/.claude/.credentials.json \
  | python3 -c "import json,sys; d=json.load(sys.stdin); print(d['claudeAiOauth']['accessToken'])"
```

**Windows — credentials file:**
```powershell
(Get-Content "$env:APPDATA\Claude\.credentials.json" | ConvertFrom-Json).claudeAiOauth.accessToken
```

If none of those work, grab it from a network request:
1. Open [claude.ai](https://claude.ai) in Chrome.
2. DevTools → **Network** tab → send any message.
3. Click any `/api/` request → **Headers** → copy the value after `Authorization: Bearer `.

> Tokens expire periodically. If the admin tells you your token is returning 401 errors, just re-run the command above and send the new value.

---

### Configure Claude Code CLI

Ask your admin for the proxy address (e.g. `http://192.168.1.50:8080`).

**Option A — environment variable (temporary, per-shell)**

```bash
export ANTHROPIC_BASE_URL=http://<proxy-host>:<port>
claude   # or 'claude code', etc.
```

**Option B — persist in Claude Code settings (recommended)**

Open (or create) `~/.claude/settings.json` and add:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://<proxy-host>:<port>"
  }
}
```

From now on every `claude` invocation will route through the proxy automatically, with no environment variable needed.

---

### Configure Claude Code in VS Code

The Claude Code VS Code extension reads the same `~/.claude/settings.json` as the CLI, so **Option B above also covers VS Code** — no additional steps needed after editing that file.

If you prefer to set it only for VS Code:

1. Open VS Code → **Settings** (`Cmd+,` / `Ctrl+,`).
2. Search for `claude`.
3. Find **Claude: Env** (or **Claude › Api: Base Url** depending on extension version).
4. Set `ANTHROPIC_BASE_URL` to `http://<proxy-host>:<port>`.

Or add it directly to your VS Code `settings.json`:

```json
{
  "claude.env": {
    "ANTHROPIC_BASE_URL": "http://<proxy-host>:<port>"
  }
}
```

---

### Verify it is working

**CLI:**
```bash
claude -p "say hello"
```

You should get a normal response. The admin can confirm your requests are appearing in the proxy logs.

**VS Code:**
Open the Claude Code panel and send any message. If it responds, the proxy is working.

---

### Revert to direct access

Remove or comment out the `ANTHROPIC_BASE_URL` line from `~/.claude/settings.json` (and VS Code settings if set separately). Claude Code will revert to connecting directly to `api.anthropic.com` with your own credentials.

---

## Reference

### CLI commands

| Command | Description |
|---|---|
| `anthroproxy serve` | Start the proxy server |
| `anthroproxy add <label>` | Add a token to the pool (prompts for the token value) |
| `anthroproxy list` | List all tokens and their current status |
| `anthroproxy remove <label\|id>` | Remove a token from the pool |
| `anthroproxy status` | Show pool summary (token count, listen address, cooldown setting) |

### Config file

Location: `~/.config/anthroproxy/config.json` (respects `$XDG_CONFIG_HOME`).

| Field | Default | Description |
|---|---|---|
| `listen` | `"0.0.0.0:8080"` | Address and port to listen on |
| `cooldown_minutes` | `30` | How long a rate-limited token is skipped before being retried |
| `tokens` | `[]` | Array of `{id, label, bearer_token}` — managed by CLI commands |

### Log format

```
[<timestamp>] token="<label>" <METHOD> <path> -> <status> (<latency>)
[<timestamp>] token="<label>" rate-limited, entering <N>m cooldown
[<timestamp>] token="<label>" all tokens exhausted, returning 429 to client
```

Watch for:
- **`401`** — that token has expired; remove and re-add it with a fresh value.
- **`all tokens exhausted`** — every token is in cooldown simultaneously; increase the pool size or reduce `cooldown_minutes`.
