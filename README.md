# (Go) SMTP OAuth Relay — v1.2

[![Docker Image](https://github.com/Palasito/go-smtp/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/Palasito/go-smtp/actions/workflows/docker-publish.yml)

A high-performance, statically-linked Go port of the [Python SMTP-to-Microsoft-Graph relay](../README.md).

It accepts SMTP connections, authenticates clients via **OAuth 2.0 client credentials**, and delivers messages through the **Microsoft Graph API** (`sendMail`). It is a **drop-in replacement** for the Python version — all environment variables, the SMTP port, and the observable behaviour are identical.

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

See [docs/authentication.md](../docs/authentication.md) for the full username format and [docs/configuration.md](../docs/configuration.md) for all environment variables.

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

Use the existing [`docker-compose.yml`](../docker-compose.yml) in the repo root. Change the `image:` or `build:` context to point at `go-smtp/`.

---

## Configuration

All configuration is via environment variables. The Go version uses **exactly the same variables** as the Python version — see [docs/configuration.md](../docs/configuration.md) for the full reference.

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
| `HTTP_TIMEOUT` | `30` | HTTP request timeout in seconds for Graph API / OAuth calls |
| `RETRY_ATTEMPTS` | `3` | Total Graph API send attempts (1 = no retry) |
| `RETRY_BASE_DELAY` | `1` | Base delay in seconds for exponential retry back-off |
| `SHUTDOWN_TIMEOUT` | `30` | Seconds to wait for in-flight sessions to finish on `SIGTERM` |
| `TLS_RELOAD_INTERVAL` | `0` | Seconds between automatic TLS certificate reloads (0 = disabled) |
| `TOKEN_CACHE_MARGIN` | `300` | Seconds before token expiry at which the cache is considered stale and a fresh token is fetched |
| `SANITIZE_HEADERS` | `false` | Strip privacy-sensitive headers (`Received`, `X-Originating-IP`, `X-Mailer`, `User-Agent`, etc.) before relaying |
| `FAILURE_WEBHOOK_URL` | _(optional)_ | HTTP(S) URL to `POST` a JSON payload to on permanent delivery failure |

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
│   │   ├── oauth.go             # OAuth 2.0 client credentials token acquisition
│   │   ├── tokencache.go        # In-memory token cache (SHA-256 keyed, TTL-based)
│   │   ├── username.go          # Username parsing (UUID/base64url, Azure Table lookup)
│   │   └── authenticator.go     # SMTP AUTH → OAuth flow
│   ├── graph/graph.go           # Microsoft Graph sendMail (raw MIME, retry + back-off)
│   ├── httpclient/client.go     # Shared singleton HTTP client with configurable timeout
│   ├── tls/tls.go               # TLS from PEM files, Azure Key Vault PKCS#12, or auto-generated self-signed; auto-reload
│   ├── webhook/webhook.go       # Best-effort HTTP notification on permanent delivery failure
│   ├── whitelist/whitelist.go   # IP/CIDR whitelist with auto-auth
│   └── server/server.go         # go-smtp Backend + Session implementation
├── Dockerfile                   # Multi-stage build: golang:alpine → scratch
└── scripts/test-smtp.sh         # Basic smoke test
```
