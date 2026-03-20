# (Go) SMTP OAuth Relay — v1.5

[![Docker Image](https://github.com/Palasito/go-smtp/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/Palasito/go-smtp/actions/workflows/docker-publish.yml)

A high-performance, statically-linked Go port of the [Python SMTP-to-Microsoft-Graph relay](https://github.com/JustinIven/smtp-oauth-relay).

It accepts SMTP connections, authenticates clients via **OAuth 2.0 client credentials**, and delivers messages through the **Microsoft Graph API** (`sendMail`). It is a **drop-in replacement** for the Python version — all environment variables, the SMTP port, and the observable behaviour are identical.

## What's new in v1.5

### Security & Correctness
- **CacheKey hash collision fix** — token cache keys now use `\x00` null-byte field separators so distinct credential tuples can never produce the same SHA-256 hash.
- **OData injection fix** — single quotes in Azure Table lookup values are now escaped (`'` → `''`), preventing filter injection.
- **Backend config data race fix** — `Config` and `Whitelist` are stored in `atomic.Pointer`; each SMTP session snapshots them at creation so SIGHUP reloads never race with in-flight sessions.
- **Token cache margin race fix** — `tokenCacheMarginSecs` replaced with `atomic.Int32` + accessor, eliminating a data race on SIGHUP reload.

### Hardening
- **Non-root Docker container** — the scratch image now runs as `USER 65534:65534`.
- **Bounded webhook goroutines** — a semaphore (cap 16) limits concurrent webhook calls; excess notifications are dropped instead of spawning unbounded goroutines.
- **Port collision validation** — startup rejects `SMTP_PORT == HEALTH_PORT` with a clear error.

### Observability
- **Per-session UUID correlation IDs** — every structured log line within a session includes `"session": "<uuid>"`.
- **Build version injection** — `Version`, `Commit`, and `BuildDate` are set via `-ldflags` at build time and served at `GET /version`.
- **Version dashboard card** — the status dashboard now shows the running version, commit, and build date.
- **Startup metadata log** — version, Go runtime, PID, and key config values are logged at boot.
- **New Prometheus metrics** — `smtp_connections_total`, `smtp_whitelist_auth_total` (by result), `smtp_recipients_per_message`, `smtp_token_cache_size`.

### Cloud & Configuration
- **Sovereign cloud support** — new `AZURE_AUTHORITY_HOST` and `GRAPH_ENDPOINT` env vars allow targeting Azure Government, Azure China, etc.
- **Real readiness probe** — `/readyz` now TCP-dials the SMTP port instead of always returning 200.

### Code Quality
- **Shared retry package** — duplicated retry/backoff logic extracted into `internal/retry`.
- **Removed empty handler package.**
- **Fixed docker-compose volume** — `./certs:/certs` (was unreachable path in scratch image).
- **Fixed broken README doc links.**

## What's new in v1.4

- **Deterministic MIME header order** — `sanitizeHeaders` and `patchHeaders` now reconstruct headers in their original document order instead of random Go map iteration order, making logs reproducible and downstream consumers reliable.
- **Per-attempt context timeout on Graph API** — each `SendMail` HTTP attempt now uses its own `context.WithTimeout` tied to `HTTP_TIMEOUT`, preventing indefinite hangs when the Graph endpoint is unresponsive.
- **Azure Table lookup timeout** — `LookupUser` now applies a 30-second context timeout, preventing SMTP sessions from blocking indefinitely on slow Azure Tables responses.
- **Token cache garbage collection** — a background goroutine sweeps expired entries from the in-memory OAuth token cache every 5 minutes, bounding memory growth for deployments with many distinct credential pairs.
- **TLS safety auto-correction** — setting `REQUIRE_TLS=true` with `TLS_SOURCE=off` (a contradictory combination) now auto-corrects `TLS_SOURCE` to `auto` with a warning, instead of silently starting without TLS.
- **OAuth token retry with backoff** — `GetAccessToken` now retries up to 3 attempts on transient errors (429, 500–504) with exponential backoff and Retry-After header support.
- **Jitter in retry backoff** — both Graph API and OAuth retry delays now include ±20% random jitter to prevent thundering-herd retry storms under load.
- **Message-ID and Subject in logs** — delivery success/failure log lines now include `messageId` and `subject` fields for easier correlation with upstream MTA logs.
- **Session duration metric** — new `smtp_session_duration_seconds` Prometheus histogram tracks the lifetime of each SMTP session from connect to disconnect.
- **Sender domain allowlist** — new `ALLOWED_FROM_DOMAINS` env var restricts which sender domains can relay through the server; unlisted domains receive `553 5.7.1`.
- **SIGHUP-reloadable whitelist** — the IP whitelist is now rebuilt on `SIGHUP`, allowing operators to update `WHITELIST_IPS` and credentials without restarting.
- **Max recipients limit** — new `MAX_RECIPIENTS` env var caps the number of `RCPT TO` addresses per message; the SMTP server rejects additional recipients with `452 4.5.3`.

## What's new in v1.3

- **SMTP session timeouts** — `SMTP_READ_TIMEOUT` and `SMTP_WRITE_TIMEOUT` cap idle reads and writes at the TCP level, protecting against slow or stalled clients that could otherwise hold connections open indefinitely.
- **Health and readiness probes** — a lightweight HTTP server (default port `9090`) exposes `GET /healthz` (liveness) and `GET /readyz` (readiness) for use with Kubernetes, Docker, and load balancers. `GET /` serves an interactive HTML status dashboard with live auto-refresh.
- **Prometheus metrics** — `GET /metrics` on the same health port serves an interactive HTML dashboard with grouped metric families, search, human-readable value formatting (bytes → KB/MB/GB, seconds → ms/s, ratios → %), and 15 s auto-refresh. `GET /metrics?$output=text` returns the raw Prometheus text format for scraper ingestion. Metrics covered: SMTP active connections, auth totals, message delivery counters and size histogram, Graph API latency and attempt histograms, OAuth token cache hit/miss counters, and webhook notification counters.
- **SIGHUP config reload** — send `SIGHUP` to the process to hot-reload all reloadable fields (log level, timeouts, retry settings, webhook URL, whitelist, etc.) without restarting. Non-reloadable fields (`SMTP_PORT`, `HEALTH_PORT`, `TLS_SOURCE`) are detected and a warning is logged.

## What's new in v1.2

- **OAuth token caching** — access tokens are cached in memory (keyed by `SHA-256(tenantID+clientID+clientSecret)`) and reused until near-expiry, eliminating redundant Azure AD round-trips. Wrong credentials always miss the cache. Configurable safety margin via `TOKEN_CACHE_MARGIN`.
- **Header sanitization** — when `SANITIZE_HEADERS=true`, privacy-sensitive headers (`Received`, `X-Originating-IP`, `X-Mailer`, `User-Agent`, etc.) are stripped from the message before it is forwarded to Graph API.
- **Failure webhook** — set `FAILURE_WEBHOOK_URL` to receive a JSON `POST` whenever a message permanently fails delivery (after all retry attempts). Best-effort, fire-and-forget — never blocks or affects the SMTP response to the client.

---

## How it works

```
SMTP Client → smtp-oauth-relay:8025 → Microsoft Graph API → recipient mailbox
```

1. Client connects and authenticates with `AUTH PLAIN` or `AUTH LOGIN`
2. The username encodes the tenant ID and client ID (UUID or base64url, separated by a delimiter)
3. The password is the OAuth client secret
4. The relay exchanges the credentials for a Graph API access token
5. The raw MIME message is forwarded to `POST /v1.0/users/{from}/sendMail`

See the [Configuration](#configuration) section below for all environment variables.

---

## Prerequisites

- **Go 1.26+** — for building from source
- **Docker** — for container builds
- An **Azure AD app registration** with `Mail.Send` permission

---

## Building

### From source

```bash
cd go-smtp
go build -o smtp-relay ./cmd/smtp-relay
```

Or with full optimisations (same flags as the Dockerfile):

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o smtp-relay ./cmd/smtp-relay
```

### Docker (multi-stage, scratch-based)

Build from the **workspace root**:

```bash
docker build -t smtp-oauth-relay:go -f go-smtp/Dockerfile go-smtp/
```

Or from inside `go-smtp/`:

```bash
docker build -t smtp-oauth-relay:go .
```

The final image is `FROM scratch` and contains only the static binary and CA certificates (~15–25 MB).

---

## Running

### Binary

```bash
export TLS_SOURCE=file
export TLS_CERT_FILEPATH=certs/cert.pem
export TLS_KEY_FILEPATH=certs/key.pem
export AZURE_TABLES_URL=https://<account>.table.core.windows.net/<table>
./smtp-relay
```

### Docker

```bash
docker run -d \
  -p 8025:8025 \
  -e TLS_SOURCE=file \
  -e TLS_CERT_FILEPATH=/certs/cert.pem \
  -e TLS_KEY_FILEPATH=/certs/key.pem \
  -v $(pwd)/certs:/certs:ro \
  smtp-oauth-relay:go
```

### Docker Compose

Use the existing [`docker-compose.yml`](docker-compose.yml) in the repo root.

---

## Configuration

All configuration is via environment variables. The Go version uses **exactly the same variables** as the Python version, plus several Go-specific additions.

| Variable | Default | Description |
|---|---|---|
| `LOG_LEVEL` | `WARNING` | `DEBUG` / `INFO` / `WARNING` / `ERROR` / `CRITICAL` |
| `TLS_SOURCE` | `file` | `off` / `auto` / `file` / `keyvault` — `auto` generates a self-signed ECDSA certificate at startup so STARTTLS works without provisioning certs |
| `REQUIRE_TLS` | `true` | Require STARTTLS before AUTH |
| `SERVER_GREETING` | `Microsoft Graph SMTP OAuth Relay` | EHLO banner string |
| `TLS_CERT_FILEPATH` | `certs/cert.pem` | PEM certificate path (TLS_SOURCE=file) |
| `TLS_KEY_FILEPATH` | `certs/key.pem` | PEM key path (TLS_SOURCE=file) |
| `TLS_CIPHER_SUITE` | _(system default)_ | Colon-separated OpenSSL cipher names |
| `USERNAME_DELIMITER` | `@` | Delimiter between tenant ID and client ID (`@`, `:`, or `\|`) |
| `ALLOWED_FROM_DOMAINS` | _(optional)_ | Comma-separated list of allowed sender domains; unlisted domains are rejected with `553 5.7.1` |
| `AZURE_KEY_VAULT_URL` | _(optional)_ | Key Vault URL (TLS_SOURCE=keyvault) |
| `AZURE_KEY_VAULT_CERT_NAME` | _(optional)_ | Secret name in Key Vault |
| `AZURE_TABLES_URL` | _(optional)_ | Full table URL for user lookup |
| `AZURE_TABLES_PARTITION_KEY` | `user` | Partition key for table lookups |
| `WHITELIST_IPS` | _(optional)_ | Comma-separated IPs/CIDRs that skip AUTH |
| `WHITELIST_TENANT_ID` | _(optional)_ | Tenant ID for whitelisted auto-auth |
| `WHITELIST_CLIENT_ID` | _(optional)_ | Client ID for whitelisted auto-auth |
| `WHITELIST_CLIENT_SECRET` | _(optional)_ | Client secret for whitelisted auto-auth |
| `WHITELIST_FROM_EMAIL` | _(optional)_ | Override From address for whitelisted sessions |
| `SMTP_PORT` | `8025` | TCP port the relay listens on |
| `MAX_MESSAGE_SIZE` | `36700160` | Maximum accepted message size in bytes (default 35 MB) |
| `MAX_RECIPIENTS` | `0` | Maximum number of `RCPT TO` addresses per message (0 = unlimited) |
| `HTTP_TIMEOUT` | `30` | HTTP request timeout in seconds for Graph API / OAuth calls |
| `RETRY_ATTEMPTS` | `3` | Total Graph API send attempts (1 = no retry) |
| `RETRY_BASE_DELAY` | `1` | Base delay in seconds for exponential retry back-off |
| `SHUTDOWN_TIMEOUT` | `30` | Seconds to wait for in-flight sessions to finish on `SIGTERM` |
| `TLS_RELOAD_INTERVAL` | `0` | Seconds between automatic TLS certificate reloads (0 = disabled) |
| `TOKEN_CACHE_MARGIN` | `300` | Seconds before token expiry at which the cache is considered stale and a fresh token is fetched |
| `SANITIZE_HEADERS` | `false` | Strip privacy-sensitive headers (`Received`, `X-Originating-IP`, `X-Mailer`, `User-Agent`, etc.) before relaying |
| `FAILURE_WEBHOOK_URL` | _(optional)_ | HTTP(S) URL to `POST` a JSON payload to on permanent delivery failure |
| `SMTP_READ_TIMEOUT` | `60` | Seconds before an idle SMTP read is timed out (per connection) |
| `SMTP_WRITE_TIMEOUT` | `60` | Seconds before an idle SMTP write is timed out (per connection) |
| `HEALTH_PORT` | `9090` | TCP port for the health/readiness/metrics HTTP server |
| `AZURE_AUTHORITY_HOST` | `https://login.microsoftonline.com` | OAuth authority URL (for sovereign clouds: Azure Government, Azure China, etc.) |
| `GRAPH_ENDPOINT` | `https://graph.microsoft.com` | Microsoft Graph base URL (for sovereign clouds) |

---

## Health & metrics endpoints

The relay exposes a secondary HTTP server (default `:9090`) alongside the SMTP listener:

| Route | Method | Description |
|---|---|---|
| `/` | `GET` | Interactive status dashboard (HTML) — live liveness/readiness/version/metrics cards, auto-refreshes every 5 s |
| `/healthz` | `GET` | Liveness probe — always `200 OK` while the process is alive |
| `/readyz` | `GET` | Readiness probe — `200 OK` / `503` (TCP-dials the SMTP port) |
| `/version` | `GET` | JSON build metadata: `{"version", "commit", "buildDate"}` |
| `/metrics` | `GET` | Interactive Prometheus metrics dashboard (HTML) — grouped metric families, search, auto-refreshes every 15 s |
| `/metrics?$output=text` | `GET` | Raw Prometheus text exposition format for scraper ingestion |

Set `HEALTH_PORT` to change the port. All endpoints are unauthenticated — bind the health server to a private interface or apply network-level access controls as appropriate.

---

## Smoke testing

A basic connectivity test script is provided:

```bash
chmod +x scripts/test-smtp.sh
./scripts/test-smtp.sh localhost 8025
```

This sends `EHLO test` and `QUIT` and prints the server response. You should see the `220` greeting and `250` EHLO capabilities listing.

---

## Differences from the Python version

| Aspect | Python | Go |
|---|---|---|
| Runtime | CPython interpreter | Static binary, no runtime |
| Base image | `python:3.12-slim` | `scratch` |
| Image size | ~200 MB | ~15–25 MB |
| Memory usage | ~50 MB idle | ~10 MB idle |
| Startup time | ~2 s | <100 ms |
| Dependencies | pip packages, venv | Compiled in, zero runtime deps |
| Concurrency | asyncio (single thread) | Go goroutines (multi-core) |

All **environment variables**, the **SMTP port (8025)**, the **username format**, and the **Azure integration** are identical. The Go version is a drop-in replacement.

---

## Migration from Python version

No changes are required on the client or Azure side:

1. Stop the Python container / process
2. Start the Go binary or container with the same environment variables
3. Clients connect to the same port with the same credentials

The only user-visible difference is a slightly different SMTP banner format in the `220` greeting.

---

## Project structure

```
go-smtp/
├── cmd/smtp-relay/main.go       # Entrypoint: wires all components, starts server
├── internal/
│   ├── config/config.go         # Environment variable loading and validation
│   ├── auth/
│   │   ├── oauth.go             # OAuth 2.0 client credentials token acquisition (with retry)
│   │   ├── tokencache.go        # In-memory token cache (SHA-256 keyed, TTL-based, periodic GC)
│   │   ├── username.go          # Username parsing (UUID/base64url, Azure Table lookup)
│   │   └── authenticator.go     # SMTP AUTH → OAuth flow
│   ├── graph/graph.go           # Microsoft Graph sendMail (raw MIME, retry + back-off + jitter)
│   ├── health/health.go         # Liveness, readiness, Prometheus /metrics, and /version HTTP handlers
│   ├── httpclient/client.go     # Shared singleton HTTP client with configurable timeout
│   ├── metrics/metrics.go       # Prometheus metric declarations (default registry)
│   ├── retry/retry.go           # Shared retry helpers (IsRetryable, Backoff with jitter)
│   ├── tls/tls.go               # TLS from PEM files, Azure Key Vault PKCS#12, or auto-generated self-signed; auto-reload
│   ├── version/version.go       # Build-time metadata (version, commit, build date) via ldflags
│   ├── webhook/webhook.go       # Best-effort HTTP notification on permanent delivery failure
│   ├── whitelist/whitelist.go   # IP/CIDR whitelist with auto-auth
│   └── server/server.go         # go-smtp Backend + Session implementation
├── Dockerfile                   # Multi-stage build: golang:alpine → scratch
└── scripts/test-smtp.sh         # Basic smoke test
```
