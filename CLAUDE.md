# Agent Development Guide

RelayAI: Wails v3 desktop app that proxies multi-vendor AI APIs into Claude/Codex-compatible endpoints.

## After Every Code Change

```bash
make fmt && make test
```

Do NOT consider a task done until both pass.

## Commands

```bash
make dev              # Dev mode with hot reload
make build            # Build for current platform (macOS: .app, others: exe)
make run              # Build and run
make test             # go test ./... -v
make test-short       # go test ./... -short
make fmt              # go fix + gofmt + frontend lint/format + go vet
make install          # Install Go + frontend deps
make clean            # Clean build artifacts
make info             # Show versions (Go, Node, Wails3)
make generate-bindings # Regenerate Go→TS bindings (wails3 generate)
```

Single test: `go test ./internal/proxy -run TestName -v`
Coverage: `go test ./... -coverprofile=c.out && go tool cover -html=c.out`

## Architecture

```
main.go                    # Entry point: window, tray, app lifecycle
app.go                     # Wails service: all exported methods → frontend bindings
internal/proxy/
  slog.go                  # Global slog setup (LevelVar, default=Info)
  server.go                # HTTP server wrapper, SetDebug toggles slog level
  router.go                # ~2100 lines: routing, SSE translation, retries
  logger.go                # Request logging + provider usage tracking (SQLite)
  session.go               # In-memory session store for multi-turn context
internal/config/
  model.go                 # Provider, AppSettings, ModelMapping structs
  store.go                 # SQLite-backed config CRUD
internal/database/
  db.go                    # SQLite init, migrations, table creation
internal/cli/
  claude.go / codex.go     # Write CLI config files (~/.claude/settings.json etc.)
internal/native/
  appearance_*.go          # OS-specific dark/light mode (CGo on macOS)
internal/singleinstance/
  singleinstance.go        # PID lock file for single-instance enforcement
frontend/src/
  App.vue                  # Root: tabs (提供商/日志/调试/关于), theme switcher
  views/
    ProvidersView.vue      # Provider list + CRUD
    AboutView.vue          # App info (click icon 3× to unlock debug tab)
    DebugView.vue          # Debug toggle (slog level control)
  components/
    ProviderForm.vue       # Add/edit provider modal
    ProviderCard.vue       # Provider card in list
    ProviderDetailsModal.vue # Provider detail + usage stats
    ProxyStatusBar.vue     # Proxy status + start/stop
    LogViewer.vue          # Request log viewer
    CLIIcon.vue            # Claude/Codex icons
  stores/app.ts            # Pinia store (providers, logs, proxy status)
  composables/             # useTheme, useMessage
  utils/index.ts           # maskKey, formatDuration, copyToClipboard, etc.
  bindings/relay-ai/app.ts # Auto-generated Wails bindings (DO NOT EDIT)
frontend/dist/             # Built output (go:embed, must exist for Go build)
```

## Routing Model

| Path | CLI | Conversion |
|---|---|---|
| `/anthropic/*` | Claude | Passthrough (Anthropic native format) |
| `/openai/*` | Codex | Responses API ↔ Chat Completions if `chat_compat_mode` |
| `/v1/responses` | Codex | Same as `/openai/*` |
| `/health` | — | Returns `ok` |

Auth tokens:
- **Proxy-level** (`sk-local-*`): routes to all enabled providers for that CLI type
- **Provider-level**: routes to a specific provider only

Provider fallback: tries each matching provider in order; 5xx triggers fallback.

## Key Technical Notes

- **Logging**: Uses `log/slog` throughout. `slog.go` sets up a `LevelVar` (default=Info). `Server.SetDebug(true)` switches to Debug level. All errors use `slog.Error`, warnings use `slog.Warn`, startup info uses `slog.Info`. No `log.Printf` or `debugLog` calls remain.
- **SSE streaming** (`translateStream`): Converts Chat Completions SSE → Responses API SSE. `headersSent` initializes to `preResponseID != ""` — caller may have already sent headers. Keep-alive every 15s. Retry: 3 attempts with exponential backoff for transient network errors.
- **`synthesizeResponsesSSE`**: Accepts `preResponseID` to skip duplicate header/response.created when caller already sent them.
- **Transport**: `sharedUpstreamTransport` — shared `http.Transport` with `ResponseHeaderTimeout=60s`, `MaxIdleConns=100`. `WriteTimeout=0` on server for long SSE streams.
- **`frontend/dist` must exist** for Go compilation. `make test`/`make fmt` auto-build it if missing.
- **`wails3`** must be in `GOPATH/bin`. Verify with `make info`.

## Data Models

```go
// Provider — stored in SQLite `providers` table
type Provider struct {
    ID, Name, BaseURL, APIKey, AuthToken string
    DefaultModel    string
    ModelMappings   []ModelMapping  // {From, To}
    CLITypes        []string        // ["claude"] or ["codex"] or both
    ChatCompatMode  bool            // Codex only: Responses→Chat conversion
    Enabled         bool
    PromptTokens, CompletionTokens, TotalTokens int64
}

// RequestLog — stored in `request_logs` (max 1000 rows, FIFO)
type RequestLog struct {
    ID, Method, Path, UpstreamURL, CLIType, ProviderID, Provider, Model string
    StatusCode int; Duration int64
    PromptTokens, CompletionTokens, TotalTokens int
    Error, ResponseBody string
}
```

**Constraint**: Provider names must be unique per CLI type. Same name on different CLI types is allowed (e.g., "mimo" for Claude + "mimo" for Codex).

## Data Storage

- SQLite: `~/.relayai/relayai.db` (via `ncruces/go-sqlite3`)
- Tables: `settings`, `providers`, `request_logs`, `provider_usage_points`
- Settings key-value: `port`, `debug`, `proxy_auth_token`

## Frontend

- Vue 3 + Naive UI + Pinia + TypeScript + Vite
- Auto-imports via `unplugin-auto-import` + `unplugin-vue-components`
- Bindings: `make generate-bindings` → `frontend/bindings/relay-ai/app.ts`
- Vite chunks: `naive-ui` in separate chunk, `chunkSizeWarningLimit: 700`

## Error Handling

- **Never ignore errors.** All `w.Write`, `fmt.Fprintf`, and similar calls must check the returned error and log it via `slog.Error`.
- Use `_, _ = ...` only as a last resort; prefer `if _, err := ...; err != nil { slog.Error(...) }`.
- Frontend: all Wails binding calls must be wrapped in try/catch; display errors via Naive UI `message.error()`.

## Conventions

- UTF-8, no BOM
- Go: `gofmt` + `go fix`; frontend: ESLint + Prettier
- Comments & git commits: **English**
- UI strings: **Simplified Chinese**
- Never hardcode secrets; validate all external input
