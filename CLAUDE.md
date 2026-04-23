# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run (development — uses .env.dev, port 8090, debug logging)
go run ./cmd/api

# Run (production — uses .env.prod, port 8080, JSON logging)
APP_ENV=prod go run ./cmd/api

# Build
go build -o bin/chat-it-api ./cmd/api

# Add a dependency
go get <module>

# Tidy dependencies
go mod tidy
```

No tests exist yet. When added, run them with `go test ./...`.

## Architecture

**Entry point:** `cmd/api/main.go` — loads config, initializes logger, starts the Gin HTTP server with graceful shutdown (SIGINT/SIGTERM, 30s timeout).

**Boot order:** `config.Load()` → `logger.Init()` → `apphttp.NewRouter()` → listen

### Package layout (`internal/`)

| Package | Role |
|---------|------|
| `config` | Reads `APP_ENV`, loads `.env.{APP_ENV}`, validates required fields (`PORT`) |
| `logger` | Singleton `slog`-based structured logger; must call `Init()` before `L()` |
| `errors` | `AppError` with HTTP status + machine-readable code; constructors: `BadRequest`, `NotFound`, `Internal` |
| `http` | Gin engine, route registration, middleware |
| `http/handlers/v1` | One file per handler group; each exports `RegisterXRoutes(rg)` |
| `response` | `OK(c, data)` and `Created(c, data)` — wrap payload with `request_id` |

### Middleware stack (applied in order)

1. `Recovery()` — catches panics → 500
2. `RequestLogger()` — generates `req_<uuid>` request ID, attaches logger to context, logs method/path/status/duration
3. `ErrorHandler()` — reads errors set on the context, formats the error JSON response; 4xx logged as warnings, 5xx logged with full context

### Request/response shapes

**Success:**
```json
{ "request_id": "req_...", "data": { ... } }
```

**Error:**
```json
{ "request_id": "req_...", "error": { "code": "MACHINE_CODE", "message": "human message" } }
```

### Adding a new handler group

1. Create `internal/http/handlers/v1/<name>.go` with a `RegisterXRoutes(rg gin.RouterGroup)` function.
2. Call `RegisterXRoutes` inside `NewRouter()` in `internal/http/router.go`.
3. Use `response.OK` / `response.Created` for success and `errors.BadRequest` / `errors.NotFound` / `errors.Internal` for failures — the `ErrorHandler` middleware handles the formatting.

### Environment variables

| Variable | Default | Notes |
|----------|---------|-------|
| `APP_ENV` | `dev` | Selects `.env.{APP_ENV}` to load; `prod` enables Gin release mode |
| `PORT` | — | Required; fatal if missing |
| `LOG_LEVEL` | `info` | `debug` or `info` |
| `LOG_FORMAT` | `text` | `text` or `json` |

Copy `.env.example` to create new environment files. The `.gitignore` excludes all `.env.*` except `.env.example`.
