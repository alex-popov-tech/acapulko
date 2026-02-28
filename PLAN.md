# Acapulko — Application Architecture Plan

## Overview

Power outages tracking dashboard for a specific address in Odesa. Polls Home Assistant for real-time grid power state, DTEK for emergency outage info, keeps a rolling 1-week history, and serves an SSR dashboard. Runs as a bare binary on a Raspberry Pi 5 via systemd behind Cloudflare.

## Tech Stack

- **Go 1.25** with Echo v5
- **html/template** for SSR dashboard + client-side JS polling
- **slog** (stdlib) for structured logging → stdout (journald captures it)
- **Sentry** (`getsentry/sentry-go`) for error tracking
- **godotenv** for `.env` loading
- **JSON file** for history persistence

External deps: `labstack/echo/v5`, `getsentry/sentry-go`, `joho/godotenv`

## Project Structure

```
acapulko/
  main.go                       # entry point: config, sentry, slog, services, Echo server, routes
  config.go                     # Config struct + loadConfig() from env
  grid-state-service.go         # GridStateService: HA poller + subscriber notifications
  emergency-outage-service.go   # OutageService: DTEK poller + subscriber notifications
  grid-history-service.go       # GridHistoryService: observes grid state changes, JSON file persistence
  utils.go                      # shared datetime type, Kyiv timezone helpers
  templates/
    index.html                  # SSR dashboard template (embeds initial state as JSON for JS)
  static/
    app.js                      # client-side rendering + 60s polling of /api/state
    styles.css                  # dark-themed CSS with power state animations
```

All files are `package main`. No sub-packages.

## Services

All three services use a callback-based observer pattern: callers pass `[]func(...)` subscriber slices which are notified on state changes (each in its own goroutine).

### GridStateService (`grid-state-service.go`)

Polls Home Assistant for grid power status.

```
GET {HA_BASE_URL}/api/states/{HA_ENTITY}
Authorization: Bearer {HA_TOKEN}
```

Types:
```go
type homeassistantResponse struct {
    State string `json:"state"` // "on" or "off"
}

type GridStateService struct {
    homeassistantURL string
    token            string
    pollInterval     time.Duration
    state            string                // cached current state
    subscribers      []func(state string)
    client           *http.Client
}
```

Methods:
- `NewGridStateService(homeassistantURL, token string, pollInterval time.Duration) *GridStateService`
- `Start(ctx context.Context, subs []func(state string))` — launches goroutine with ticker, polls HA, notifies subscribers on change
- `onTick(state string)` — compares with previous state, notifies if changed
- `getGridState(ctx context.Context) (string, error)` — HTTP GET with Bearer auth

On error: `slog.Error` + `sentry.CaptureException` + wait 10s and retry.

### OutageService (`emergency-outage-service.go`)

Polls DTEK for emergency outage info.

```
GET {DTEK_BASE_URL}/api/status?region={DTEK_REGION}&city={DTEK_CITY}&street={DTEK_STREET}
```

Types:
```go
type Outage struct {
    Type string    `json:"type"`
    From *datetime `json:"from"`
    To   *datetime `json:"to"`
}

type OutageService struct {
    betterDtekURL string
    region, city, street, building string
    pollInterval  time.Duration
    state         *Outage
    subscribers   []func(o *Outage)
    client        *http.Client
}
```

Methods:
- `NewOutageService(betterDtekBaseURL, region, city, street, building string, pollInterval time.Duration) *OutageService`
- `Start(ctx context.Context, subs []func(o *Outage))` — goroutine with ticker, polls DTEK
- `onTick(o *Outage)` — deep equality check via `Outage.Equal()`, notifies if changed
- `getOutage(ctx context.Context) (*Outage, error)` — HTTP GET, decode, lookup building

On error: `slog.Error` + `sentry.CaptureException` + wait 10s and retry.

### GridHistoryService (`grid-history-service.go`)

Observes grid state changes and maintains a rolling 1-week history.

Types:
```go
type HistoryItem struct {
    State string    `json:"state"`
    From  datetime  `json:"from"`
    To    *datetime `json:"to,omitempty"`
}

type GridHistoryService struct {
    mu            sync.Mutex
    jsonDbPath    string
    historyWindow time.Duration
    state         []HistoryItem
    subscribers   []func(state []HistoryItem)
}
```

Methods:
- `NewGridHistoryService(jsonDbPath string, historyWindow time.Duration) *GridHistoryService`
- `Start(subs []func(state []HistoryItem))` — loads JSON from disk, sets up subscribers
- `State() []HistoryItem` — reads from cache (mutex-protected)
- `OnHistoryUpdate(ctx context.Context) func(state string)` — returns closure used as GridStateService subscriber

On each state change:
1. Close previous item (set `To` timestamp)
2. Append new `HistoryItem{State, From: now}`
3. Prune entries older than `historyWindow`
4. Notify subscribers with a copy
5. Atomic write to disk: `history.json.tmp` → `os.Rename` to `history.json`

On startup: read `history.json` if exists; missing/corrupt file → start with empty history.

### Shared datetime type (`utils.go`)

```go
type datetime struct { time.Time }
```

Custom JSON marshal/unmarshal with Ukrainian format (`"HH:MM DD.MM.YYYY"`) in `Europe/Kyiv` timezone. Used by both `Outage` and `HistoryItem`.

Also provides `nowKyiv()` helper for consistent timestamping.

## Configuration (`config.go`)

```go
type Config struct {
    Port             string
    HABaseURL        string
    HAToken          string
    HAEntity         string
    HAPollInterval   time.Duration
    DTEKBaseURL      string
    DTEKRegion       string
    DTEKCity         string
    DTEKStreet       string
    DTEKBuilding     string
    DTEKPollInterval time.Duration
    HistoryFilePath  string
    HistoryWindow    time.Duration
    SentryDSN        string
    SentryEnv        string
}
```

`loadConfig() (*Config, error)`:
- `godotenv.Load()` — best-effort (no error if `.env` missing, allows pure env vars via systemd)
- All environment variables are required; fails fast listing any missing vars
- Duration fields (`HA_POLL_INTERVAL`, `DTEK_POLL_INTERVAL`, `HISTORY_WINDOW`) are parsed and validated

## HTTP Server (in `main.go`)

Echo v5 setup with middleware and routes, all defined inline in `main()`.

### Middleware Stack (in order)
1. `middleware.Recover()` — catch panics, return 500
2. `middleware.RequestLoggerWithConfig()` — log requests via slog (method, path, status, latency, IP)

### Routes

| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| GET | `/` | inline | SSR template with current `PowerState` snapshot |
| GET | `/api/state` | inline | JSON endpoint for JS polling (includes version) |
| GET | `/static/*` | `e.Static` | CSS, JS assets |

### Template Rendering

Uses `html/template` with a custom `json` template function that marshals `PowerState` into the page as `INITIAL_STATE` for the client-side JS.

### Dashboard Data

```go
type PowerState struct {
    Outage  *Outage       `json:"outage"`
    Grid    string        `json:"grid"`    // "on", "off", or "pending"
    History []HistoryItem `json:"history"`
    Address string        `json:"address"`
    Version string        `json:"version"`
}
```

Protected by a `sync.Mutex` in `main()`. Each service updates its relevant field via subscriber callbacks.

### Graceful Shutdown

Uses Echo v5's `StartConfig.Start(ctx, e)` which handles graceful shutdown when the context is cancelled. Combined with `signal.NotifyContext` for SIGINT/SIGTERM.

## Lifecycle

### Startup (`main.go`)

```
1.  godotenv.Load()
2.  loadConfig()
3.  slog.SetDefault(slog.NewTextHandler(os.Stdout, ...))
4.  sentry.Init(dsn, env, release=version)
5.  defer sentry.Flush(2 * time.Second)
6.  ctx, stop := signal.NotifyContext(SIGINT, SIGTERM)
7.  outageService := NewOutageService(...)
8.  gridStateService := NewGridStateService(...)
9.  gridHistoryService := NewGridHistoryService(...)
10. gridHistoryService.Start(subscribers)
11. outageService.Start(ctx, subscribers)
12. gridStateService.Start(ctx, subscribers)  // history update wired here
13. Echo setup: middleware + routes
14. sc.Start(ctx, e)  // blocks until signal
```

### Shutdown

```
Signal received (SIGINT/SIGTERM)
  → ctx cancelled
  → GridState/Outage services exit ticker loops
  → Echo graceful shutdown (drains in-flight requests)
  → sentry.Flush(2s)
  → main() returns
```

## Error Handling

| Domain | Strategy |
|--------|----------|
| Background poll failure | slog.Error + sentry.CaptureException + keep stale cache; retry after 10s |
| History file I/O error | slog.Error + sentry.CaptureException; in-memory data stays authoritative |
| HTTP handler panic | middleware.Recover catches it, returns 500 |
| HTTP handler error | Echo default error handler |
| Context cancellation | Clean exit, not logged as error |
| Startup config missing | Fail fast with clear error message via log.Fatal |

## Version

The `version` variable (`main.go`) defaults to `"dev"` and is set at build time via `-ldflags "-X main.version=vX.Y.Z"`.

Exposed in:
- Startup log line
- `/api/state` JSON response
- Sentry release tag

## Future (not in scope now)

- PWA support (service worker, manifest.json)
- Pre-populate history from HA historical data
- Systemd unit file as part of the repo
- Browser notifications on state change
