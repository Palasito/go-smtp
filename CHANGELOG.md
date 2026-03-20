# Changelog

All notable changes to this project are documented in this file.

---

## v1.5.1

### Logging Overhaul
- **Persistent file logging** — new `LOG_FILE` env var writes logs to both stdout and a file via `io.MultiWriter`. Parent directories are created automatically.
- **Hourly log rotation** — new `LOG_ROTATE_HOURS` env var (default `1`). Timestamped filenames (e.g. `relay-2026-03-20T14.log`) are created at each rotation boundary.
- **Automatic log retention** — new `LOG_RETENTION_DAYS` env var (default `0` = keep forever). A background goroutine periodically deletes log files older than the configured retention.
- **JSON log format** — new `LOG_FORMAT` env var (`text` / `json`). `json` outputs structured JSON lines for log aggregation pipelines (ELK, Grafana Loki, Splunk, Azure Monitor).
- **Startup banner in each log file** — every rotated file begins with a header line containing version, commit, build date, PID, and Go runtime version.
- **SIGHUP hot-reload** — all five logging env vars (`LOG_LEVEL`, `LOG_FILE`, `LOG_FORMAT`, `LOG_ROTATE_HOURS`, `LOG_RETENTION_DAYS`) can be changed at runtime.
- **New `internal/logfile` package** — zero-dependency rotating writer with `io.Writer` + `io.Closer` interface.

---

## v1.5

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

---

## v1.4

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

---

## v1.3

- **SMTP session timeouts** — `SMTP_READ_TIMEOUT` and `SMTP_WRITE_TIMEOUT` cap idle reads and writes at the TCP level, protecting against slow or stalled clients that could otherwise hold connections open indefinitely.
- **Health and readiness probes** — a lightweight HTTP server (default port `9090`) exposes `GET /healthz` (liveness) and `GET /readyz` (readiness) for use with Kubernetes, Docker, and load balancers. `GET /` serves an interactive HTML status dashboard with live auto-refresh.
- **Prometheus metrics** — `GET /metrics` on the same health port serves an interactive HTML dashboard with grouped metric families, search, human-readable value formatting (bytes → KB/MB/GB, seconds → ms/s, ratios → %), and 15 s auto-refresh. `GET /metrics?$output=text` returns the raw Prometheus text format for scraper ingestion. Metrics covered: SMTP active connections, auth totals, message delivery counters and size histogram, Graph API latency and attempt histograms, OAuth token cache hit/miss counters, and webhook notification counters.
- **SIGHUP config reload** — send `SIGHUP` to the process to hot-reload all reloadable fields (log level, timeouts, retry settings, webhook URL, whitelist, etc.) without restarting. Non-reloadable fields (`SMTP_PORT`, `HEALTH_PORT`, `TLS_SOURCE`) are detected and a warning is logged.

---

## v1.2

- **OAuth token caching** — access tokens are cached in memory (keyed by `SHA-256(tenantID+clientID+clientSecret)`) and reused until near-expiry, eliminating redundant Azure AD round-trips. Wrong credentials always miss the cache. Configurable safety margin via `TOKEN_CACHE_MARGIN`.
- **Header sanitization** — when `SANITIZE_HEADERS=true`, privacy-sensitive headers (`Received`, `X-Originating-IP`, `X-Mailer`, `User-Agent`, etc.) are stripped from the message before it is forwarded to Graph API.
- **Failure webhook** — set `FAILURE_WEBHOOK_URL` to receive a JSON `POST` whenever a message permanently fails delivery (after all retry attempts). Best-effort, fire-and-forget — never blocks or affects the SMTP response to the client.

---

## v1.1

- Initial Go port with feature parity to the Python SMTP-to-Graph relay.
