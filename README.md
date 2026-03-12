# Clawring

A proxy that sits between your AI agents and LLM providers (Anthropic, OpenAI). Agents authenticate with throwaway "phantom tokens" instead of real API keys. The proxy swaps in the actual credentials before forwarding upstream, so your agents never see real keys.

## How it works

```
Agent VM                      Clawring Proxy                   Upstream API
+----------+   phantom token   +--------------+   real key      +----------------+
| AI Agent | ───────────────> | Clawring     | ─────────────> | api.anthropic  |
|          | <─────────────── |              | <───────────── | api.openai     |
+----------+   response        +--------------+   response      +----------------+
```

1. Register an agent via the admin API. You get back a phantom token (shown once, save it).
2. Add your real API keys. They're encrypted with AES-256-GCM before hitting SQLite.
3. Agent sends requests to Clawring using its phantom token.
4. Clawring checks the token, checks the vendor allowlist, swaps in the real key, forwards upstream.
5. The agent never touches the real key.

## What's in the box

- Agents auth with proxy-issued phantom tokens. Real keys stay on the proxy.
- API keys encrypted at rest (AES-256-GCM, random nonces per encryption).
- Phantom tokens are SHA-256 hashed in the DB. If the database leaks, the tokens are useless.
- Admin auth uses constant-time comparison (`crypto/subtle`).
- Per-agent vendor allowlists. Agent A can talk to Anthropic but not OpenAI, etc.
- Token rotation without re-registering the agent.
- SSE streaming works, with per-chunk write deadlines.
- Rate limiting per IP (token bucket) on both admin and data servers.
- Audit log for all admin mutations (key/agent create, delete, rotate).
- Per-agent usage tracking with automatic 30-day pruning.
- Hardened systemd unit with 15+ security directives.
- One external dependency (`modernc.org/sqlite`). Everything else is stdlib.
- About 1,800 lines of Go.

## Supported providers

| Provider | Upstream Host | Auth Method |
|----------|---------------|-------------|
| `anthropic` | api.anthropic.com | `x-api-key` header |
| `openai` | api.openai.com | `Authorization: Bearer` |

Adding a new provider is a 5-line entry in `proxy/vendors.go`.

## Quick start

### Install

```bash
curl -fsSL https://raw.githubusercontent.com/Pfgoriaux/clawring/main/scripts/install.sh | bash
```

Downloads the binary to `~/.local/bin/`. No sudo, no config, no dependencies.

Or build from source:

```bash
git clone https://github.com/Pfgoriaux/clawring.git
cd clawring
go build -o clawring .
```

### Setup

Generate a master key and admin token, then run:

```bash
mkdir -p ~/.config/clawring
openssl rand -hex 32 > ~/.config/clawring/master_key
openssl rand -hex 32 > ~/.config/clawring/admin_token

MASTER_KEY_FILE=~/.config/clawring/master_key \
ADMIN_TOKEN_FILE=~/.config/clawring/admin_token \
DB_PATH=~/.config/clawring/proxy.db \
clawring
```

A sample systemd unit is in `scripts/openclaw-proxy.service` if you want to run it as a service.

### Add an API key

```bash
curl -X POST http://127.0.0.1:9100/admin/keys \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"vendor":"anthropic","secret":"sk-ant-your-real-key","label":"production"}'
```

### Register an agent

```bash
curl -X POST http://127.0.0.1:9100/admin/agents \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"hostname":"my-agent","allowed_vendors":["anthropic"]}'
```

Save the returned `token`. It's only shown once.

### Use from your agent

Point your agent at the proxy instead of the real API:

```bash
curl -X POST http://127.0.0.1:9101/anthropic/v1/messages \
  -H "x-api-key: <phantom-token>" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5-20250514","max_tokens":100,"messages":[{"role":"user","content":"Hello"}]}'
```

The proxy swaps the phantom token for the real key and forwards to `api.anthropic.com`.

## API reference

### Admin server (default: `127.0.0.1:9100`)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/admin/health` | None | Health check |
| `POST` | `/admin/keys` | Bearer | Add a vendor API key |
| `GET` | `/admin/keys` | Bearer | List keys (secrets not returned) |
| `DELETE` | `/admin/keys/{id}` | Bearer | Delete a key |
| `POST` | `/admin/agents` | Bearer | Register an agent (returns phantom token) |
| `GET` | `/admin/agents` | Bearer | List agents |
| `POST` | `/admin/agents/{id}/rotate` | Bearer | Rotate an agent's phantom token |
| `DELETE` | `/admin/agents/{id}` | Bearer | Delete an agent |

### Data server (default: `127.0.0.1:9101`)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `*` | `/{vendor}/*` | Phantom token | Proxied to upstream vendor API |

## Configuration

Environment variables (or systemd `Environment=`):

| Variable | Default | Description |
|----------|---------|-------------|
| `MASTER_KEY_FILE` | `/etc/openclaw-proxy/master_key` | Path to 32-byte hex-encoded master key |
| `ADMIN_TOKEN_FILE` | `/etc/openclaw-proxy/admin_token` | Path to admin bearer token |
| `BIND_ADDR` | `127.0.0.1` | Bind address (set to Tailscale IP for mesh access) |
| `ADMIN_PORT` | `9100` | Admin API port |
| `DATA_PORT` | `9101` | Data proxy port |
| `DB_PATH` | `/var/lib/openclaw-proxy/proxy.db` | SQLite database path |

### Binding to a Tailscale IP

```bash
tailscale ip -4

sudo systemctl edit openclaw-proxy
# Add: Environment=BIND_ADDR=100.x.x.x

sudo systemctl restart openclaw-proxy
```

## Security

| Layer | How |
|-------|-----|
| Encryption at rest | AES-256-GCM, random nonce per encryption |
| Token storage | SHA-256 hash (one-way) |
| Admin auth | Constant-time comparison (`crypto/subtle`) |
| Rate limiting | Token bucket per IP (admin: 100/min, data: 1000/min) |
| Request limits | Admin: 1 MB body, Proxy: 10 MB body |
| Timeouts | Read: 30s, Header: 10s, Idle: 120s, Upstream: 600s |
| Systemd | `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, `MemoryDenyWriteExecute`, syscall filtering |
| Network | Loopback only by default. You have to explicitly configure it for network access. |

### TLS

Clawring does not terminate TLS. Put it behind:
- Tailscale (recommended) for WireGuard encryption with zero config
- A reverse proxy (nginx, Caddy) for TLS termination
- Any encrypted network

Don't expose it on a plaintext network. Phantom tokens and API keys would transit in the clear.

### Token rotation

If a phantom token gets compromised:

```bash
curl -X POST http://127.0.0.1:9100/admin/agents/<agent-id>/rotate \
  -H "Authorization: Bearer <admin-token>"
```

The old token stops working immediately. Give the new one to the agent.

## Project layout

```
clawring/
├── main.go              # entry point, dual-server startup, graceful shutdown
├── admin/
│   ├── handler.go       # CRUD + audit logging
│   └── handler_test.go
├── proxy/
│   ├── handler.go       # token validation, vendor routing
│   ├── upstream.go      # request forwarding, SSE streaming
│   ├── vendors.go       # vendor registry
│   └── handler_test.go
├── crypto/
│   ├── crypto.go        # AES-256-GCM, SHA-256, token gen
│   └── crypto_test.go
├── config/
│   ├── config.go        # env/file-based config
│   └── config_test.go
├── db/
│   ├── db.go            # SQLite setup, migrations
│   ├── keys.go          # encrypted key storage
│   ├── agents.go        # agent registration, token rotation
│   ├── usage.go         # usage logging and pruning
│   └── db_test.go
├── middleware/
│   ├── ratelimit.go     # token bucket rate limiter
│   └── ratelimit_test.go
└── scripts/
    ├── install.sh       # installer (binary download or local build)
    └── openclaw-proxy.service
```

## Development

```bash
go test ./...           # run tests
go test -race ./...     # with race detector
go build -o clawring .  # build
govulncheck ./...       # check for vulnerabilities
```

## License

MIT
