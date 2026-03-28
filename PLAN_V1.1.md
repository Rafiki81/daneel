# Daneel v1.1 — Cerrar Gaps + Mejoras Nuevas

> Plan de mejoras post-v1.0. Cubre funcionalidades declaradas pero no implementadas, y oportunidades nuevas detectadas en auditoría del codebase.

---

## Estado Global

| Fase | Estado | Resumen |
|---|---|---|
| **Fase 1** — Streaming End-to-End | ✅ Completada | `ChatStream()` en 4 providers, runner `callProvider`/`consumeStream`, mock `RespondStream`, 5 tests |
| **Fase 2** — Context Window Management | ✅ Completada | `context_mgmt.go`, `WithMaxContextTokens`, sliding + summarize + error strategies, 8 tests |
| **Fase 3** — Config / Session / Validation | ✅ Completada | `config.go` (`LoadConfig`/`BuildAgents`), session cleanup vía `StartCleanup`, `RunStructured` validation + retry, 13 tests |
| **Fase 4** — Tenant Wiring + Resiliencia | ✅ Completada | `WithTenant` scoped sessions, circuit breaker, `WithMaxHandoffDepth`, `WithFailFast`, composable `WithRunHook`, 10 tests |
| **Fase 5** — Ejemplos + CI/CD | ⬜ Pendiente | |
| **Fase 6** — Observabilidad Mejorada | ⬜ Pendiente | |

---

## Estado Actual (v1.0 → v1.1)

La implementación core está sólida: agent loop, tools con genéricos, permisos, workflows (chain/parallel/router/orchestrator/FSM), memoria (sliding/summary/vector/composite), 4 providers (OpenAI/Anthropic/Google/Ollama), 6 plataformas, bridge, MCP, approval, billing, tenant, pub/sub, cron, knowledge base, fine-tuning, WebSocket server.

### Gaps Detectados y Resueltos

| Gap | Severidad | Estado | Resolución |
|---|---|---|---|
| **Streaming** | 🔴 Alta | ✅ | `ChatStream()` implementado en OpenAI, Anthropic, Google, Ollama. Runner detecta `StreamProvider` automáticamente. |
| **Context Window** | 🟡 Media | ✅ | `context_mgmt.go` con `manageContext()`, `slidingWindowTrim`, `summarizeOldMessages`. `ModelInfoProvider` + `TokenCounter` integrados. `WithMaxContextTokens(n)`. |
| **Config Loading** | 🟡 Media | ✅ | `config.go` con `LoadConfig(path)`, `BuildAgents(tools, memFactory)`, `BuildPlatforms()`. Expansión `${ENV_VAR}`. |
| **Session Cleanup** | 🟡 Media | ✅ | `FileStore.StartCleanup(ctx, interval)` con `sync.Once`. Runner auto-inicia vía type assertion. |
| **Tenant Wiring** | 🟡 Media | ✅ | `WithTenant(mgr, id)` scoped sessions (`"tenantID:uuid"`), quota pre-check, usage post-record. |
| **Structured Validation** | 🟢 Baja | ✅ | `validateSchemaConstraints()` chequea `required` + `enum`. Auto-retry con hint al LLM. |

### Mejoras Nuevas Implementadas

| Mejora | Categoría | Estado | Resolución |
|---|---|---|---|
| **Circuit Breaker** | Resiliencia | ✅ | `provider.CircuitBreaker(p, MaxFailures(5), OpenTimeout(30s))`. 3 estados: closed → open → half-open. |
| **Max Handoff Depth** | Seguridad | ✅ | `WithMaxHandoffDepth(n)`. Propagado por context. `ErrMaxHandoffDepth` + `HandoffDepthError`. |
| **FailFast Tool Strategy** | Resiliencia | ✅ | `WithFailFast()`. Context cancelable en ejecución paralela de tools. |
| **Composable RunHooks** | Extensibilidad | ✅ | `WithRunHook` ahora compone (no sobreescribe). `CombineRunOptions(opts...)`. |
| **Métricas en Bridge** | Observabilidad | ⬜ | Pendiente (Fase 6) |
| **Diagnóstico JSON** | Debugging | ⬜ | Pendiente (Fase 6) |
| **CI/CD Pipeline** | Infraestructura | ⬜ | Pendiente (Fase 5) |
| **Provider Unit Tests** | Calidad | ⬜ | Pendiente (Fase 5) |
| **Ejemplos** | Documentación | ⬜ | Pendiente (Fase 5) |

---

## Fases de Implementación

### Fase 1: Streaming End-to-End ✅

> Bloqueante para `ws/` (WebSocket server) y cualquier UI en tiempo real.

| # | Tarea | Archivo(s) | Descripción |
|---|---|---|---|
| 1.1 | `ChatStream()` OpenAI | `provider/openai/openai.go` | SSE con `stream: true`. Parsear `data: {...}` línea a línea. Emitir `StreamText`, `StreamToolCallStart`, `StreamToolCallDone`, `StreamDone`. |
| 1.2 | `ChatStream()` Anthropic | `provider/anthropic/anthropic.go` | SSE con eventos `message_start`, `content_block_delta`, `message_stop`. Mapear a `StreamChunk`. |
| 1.3 | `ChatStream()` Google | `provider/google/google.go` | SSE con `streamGenerateContent`. Mapear `candidates[].content.parts[]` parciales. |
| 1.4 | `ChatStream()` Ollama | `provider/ollama/ollama.go` | NDJSON streaming con `"stream": true` en `/api/chat`. Cada línea JSON es un chunk. |
| 1.5 | Conectar en Runner | `runner.go` | Si `cfg.streamFn != nil` y provider es `StreamProvider`, usar `ChatStream()` en vez de `Chat()`. Acumular chunks para `Response` final. **Depende de 1.1–1.4.** |
| 1.6 | Tests streaming | `provider/openai/openai_test.go`, `daneel_test.go` | Mock HTTP server que emite SSE chunks. Verificar orden de callbacks. Test de cancelación mid-stream. |

**Referencia**: `StreamProvider` interface en `options.go:25`, `StreamChunk` types en `options.go:193`.

**Verificación**: Test que crea agent con mock streaming provider → ejecuta `Run()` con `WithStreaming()` → verifica callbacks reciben tokens incrementales.

---

### Fase 2: Context Window Management Robusto ✅

> Paralela con Fase 1 — sin dependencias.

| # | Tarea | Archivo(s) | Descripción |
|---|---|---|---|
| 2.1 | Crear `context_mgmt.go` | `context_mgmt.go` (nuevo) | Extraer `truncateMessages()` de `runner.go`. Implementar `ContextSummarize` (LLM resume mensajes antiguos con prompt dedicado). Hacer `maxTokens` configurable en vez de hardcoded 100K. |
| 2.2 | Integrar `TokenCounter` | `context_mgmt.go`, `runner.go` | Si provider implementa `TokenCounter` → conteo real. Si implementa `ModelInfoProvider` → usar `ContextWindow`. Fallback a heurística `len/4`. |
| 2.3 | `WithMaxContextTokens(n)` | `options.go` | Nueva `AgentOption` para override del context window del modelo. |
| 2.4 | Tests | `context_mgmt_test.go` | Test cada strategy (sliding, summarize, error). Test fallback heurística vs conteo. Modelo con 4K tokens que se trunca correctamente. |

**Referencia**: `ContextStrategy` enum en `options.go:237`, `TokenCounter` interface en `options.go:38`.

**Verificación**: Test con historial que excede contexto → `ContextSummarize` genera resumen → mantiene últimos N mensajes dentro del límite.

---

### Fase 3: Config Loading + Session Cleanup + Structured Validation ✅

> Paralela con Fases 1 y 2.

| # | Tarea | Archivo(s) | Descripción |
|---|---|---|---|
| 3.1 | Config loading | `config.go` (nuevo) | `Config` struct + `LoadConfig(path string) (*Config, error)`. JSON con `encoding/json` (stdlib). Expansión de `${ENV_VAR}` en strings. Métodos `BuildPlatforms()` y `BuildAgents(platforms)`. |
| 3.2 | Session cleanup | `session/file.go` | `FileStore.StartCleanup(ctx, interval)` que lanza goroutine background. Limpia sesiones donde `UpdatedAt + TTL < now`. Runner llama si sessionStore soporta cleanup (interface assertion). |
| 3.3 | Structured validation | `daneel.go` | En `RunStructured[T]()`, post-unmarshal: validar `required` fields presentes, `enum` values válidos, tipos correctos. Si viola schema → reintentar 1 vez con hint al LLM. |
| 3.4 | Tests | Varios `_test.go` | `LoadConfig("testdata/config.json")` produce agentes. Cleanup elimina expiradas. `RunStructured` con schema inválido reintenta. |

**Esquema JSON para config** (ya documentado en PLAN.md):

```json
{
  "provider": {
    "type": "openai",
    "api_key": "${OPENAI_API_KEY}",
    "model": "gpt-4o"
  },
  "platforms": { "twitter": {...}, "github": {...} },
  "agents": [
    {
      "name": "support",
      "instructions": "Handle customer support",
      "model": "gpt-4o",
      "tools": ["twitter.reply", "github.comment"],
      "deny_tools": ["github.merge_pr"],
      "max_turns": 15,
      "memory": { "type": "sliding", "size": 20 }
    }
  ]
}
```

**Verificación**: `LoadConfig("testdata/config.json")` → agentes funcionales. Session cleanup elimina sesiones expiradas. `RunStructured` con campo faltante → reintenta → error claro.

---

### Fase 4: Tenant Wiring + Resiliencia ✅

> Depende parcialmente de Fase 3 (config para tenant config).

| # | Tarea | Archivo(s) | Descripción |
|---|---|---|---|
| 4.1 | Tenant → Runner | `runner.go`, `tenant/middleware.go` | `WithTenant(manager, tenantID)` como `RunOption`. Pre-run: `checkQuota()` → error si excede. Post-run: `recordUsage()`. Scoping: prefix session IDs con `tenantID:`. |
| 4.2 | Circuit breaker | `provider/circuitbreaker.go` (nuevo) | Wrapper `CircuitBreaker(p Provider, opts...)`. 3 estados: closed → open → half-open. Config: `MaxFailures(5)`, `OpenTimeout(30s)`, `HalfOpenRequests(1)`. ~80 LOC, zero deps. |
| 4.3 | Max handoff depth | `runner.go`, `options.go` | `WithMaxHandoffDepth(n)` (default 5). Runner lleva contador de profundidad. Si excede → `ErrMaxHandoffDepth`. |
| 4.4 | FailFast tool strategy | `runner.go` | Nueva strategy para ejecución paralela: `FailFast` cancela ctx del resto si una tool falla. Junto al actual `ContinueOnError`. |
| 4.5 | Tests | Varios `_test.go` | Quota enforcement. Circuit breaker (3 estados + transiciones). Handoff depth. FailFast cancela correctamente. |

**Circuit Breaker — Diseño**:

```go
// Closed (normal): requests pasan. Cuenta fallos.
// Open (tras MaxFailures consecutivos): falla inmediatamente sin llamar al provider. Dura OpenTimeout.
// Half-Open (tras OpenTimeout): deja pasar HalfOpenRequests. Si succeeden → Closed. Si fallan → Open.

p := provider.CircuitBreaker(openai.New(...),
    provider.MaxFailures(5),
    provider.OpenTimeout(30 * time.Second),
)
```

**Verificación**: Agent con tenant "acme" que excede quota → error. Provider con circuit breaker tras 5 fallos → `ErrCircuitOpen`. Handoff depth 3 con cadena de 4 → `ErrMaxHandoffDepth`.

---

### Fase 5: Ejemplos + CI/CD + Provider Tests

> Depende de Fases 1–4 para poder demostrar features completas.

| # | Tarea | Archivo(s) | Descripción |
|---|---|---|---|
| 5.1 | Ejemplos completos | `examples/*/main.go` + `README.md` | Implementar los 5 directorios vacíos (ver tabla abajo). Cada uno con mock provider para que compilen sin API keys. |
| 5.2 | CI/CD pipeline | `.github/workflows/ci.yml` (nuevo) | `go test ./...`, `go vet ./...`, `golangci-lint run`, coverage badge. Matrix: Go 1.24+. Solo unit tests. |
| 5.3 | Makefile | `Makefile` (nuevo) | Targets: `test`, `lint`, `coverage`, `build`, `examples`. |
| 5.4 | Provider tests | `provider/*/openai_test.go`, etc. (nuevos) | `httptest.Server` mocks. Verificar: request format, response parsing, retry en 429, timeout, JSON roto, error responses. |
| 5.5 | Godoc examples | `doc.go` | Agregar `Example_*` functions para quickstart, memoria, workflows. |

**Ejemplos a implementar**:

| Directorio | Qué Demuestra |
|---|---|
| `examples/slack-assistant/` | Bot Slack con memoria sliding + approval workflow |
| `examples/github-reviewer/` | PR review agent con GitHub tools |
| `examples/twitter-bot/` | Bot que responde a menciones automáticamente |
| `examples/multi-platform/` | Un agente en Slack + Telegram simultáneo via Bridge |
| `examples/permissions/` | Demo de allow/deny lists + guardrails + strict mode |

**CI/CD pipeline**:

```yaml
name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go-version: ['1.24']
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: ${{ matrix.go-version }} }
      - run: go test -race -coverprofile=coverage.out ./...
      - run: go vet ./...
```

**Verificación**: `make test` pasa. `make lint` pasa. Los 6 ejemplos compilan. CI green en push.

---

### Fase 6: Observabilidad Mejorada

> Paralela con cualquier otra fase.

| # | Tarea | Archivo(s) | Descripción |
|---|---|---|---|
| 6.1 | Métricas en Bridge | `bridge/bridge.go` | `WithMetrics(MetricsCollector)` option. Reportar: mensajes procesados, errores, latencia, sesiones activas, cleanups. Compatible con OTEL si existe, noop si no. |
| 6.2 | Diagnóstico JSON | Providers + `tool.go` | Cuando JSON parsing falla: loguear vía `slog` primeros 500 chars de respuesta raw. Incluir provider, modelo y status code en el error. |
| 6.3 | Tests | `bridge/bridge_test.go`, provider tests | Mock metrics → verificar contadores. JSON malformed → verificar log. |

**Interface de métricas para Bridge**:

```go
type BridgeMetrics interface {
    RecordMessageProcessed(platform string, duration time.Duration, err error)
    RecordActiveConversations(count int)
    RecordCleanup(deletedSessions int)
}
```

**Verificación**: Bridge con mock metrics → contadores incrementan correctamente.

---

## Grafo de Dependencias

```
Fase 1 (Streaming) ─────────────┐
Fase 2 (Context) ───────────────┤
Fase 3 (Config/Session/Schema) ─┼──→ Fase 5 (Ejemplos/CI)
Fase 4 (Tenant/Resiliencia) ────┘
Fase 6 (Observabilidad) ─────── (paralela con todo)
```

- **Fases 1, 2, 3, 6** son independientes entre sí → pueden ejecutarse en paralelo.
- **Fase 4** depende parcialmente de Fase 3 (config para tenant).
- **Fase 5** depende de 1–4 (necesita features completas para los ejemplos).

---

## Inventario de Archivos

### Archivos a Modificar

| Archivo | Fases | Cambios |
|---|---|---|
| `runner.go` | 1, 2, 4 | Streaming integration, extraer truncateMessages, handoff depth, FailFast, tenant hooks |
| `options.go` | 2, 4 | `WithMaxContextTokens`, `WithMaxHandoffDepth`, `ToolExecutionStrategy` |
| `daneel.go` | 3 | Structured validation en `RunStructured[T]` |
| `session/file.go` | 3 | Background cleanup goroutine |
| `tenant/middleware.go` | 4 | Wire `WithTenant` como `RunOption` |
| `bridge/bridge.go` | 6 | Metrics hooks |
| `provider/openai/openai.go` | 1 | `ChatStream()` implementation |
| `provider/anthropic/anthropic.go` | 1 | `ChatStream()` implementation |
| `provider/google/google.go` | 1 | `ChatStream()` implementation |
| `provider/ollama/ollama.go` | 1 | `ChatStream()` implementation |
| `doc.go` | 5 | Godoc examples |

### Archivos a Crear

| Archivo | Fase | ~LOC |
|---|---|---|
| `context_mgmt.go` | 2 | ~150 |
| `config.go` | 3 | ~200 |
| `provider/circuitbreaker.go` | 4 | ~80 |
| `provider/openai/openai_test.go` | 5 | ~200 |
| `provider/anthropic/anthropic_test.go` | 5 | ~150 |
| `provider/google/google_test.go` | 5 | ~150 |
| `provider/ollama/ollama_test.go` | 5 | ~150 |
| `.github/workflows/ci.yml` | 5 | ~40 |
| `Makefile` | 5 | ~30 |
| `examples/slack-assistant/main.go` | 5 | ~60 |
| `examples/slack-assistant/README.md` | 5 | ~30 |
| `examples/github-reviewer/main.go` | 5 | ~60 |
| `examples/github-reviewer/README.md` | 5 | ~30 |
| `examples/twitter-bot/main.go` | 5 | ~60 |
| `examples/twitter-bot/README.md` | 5 | ~30 |
| `examples/multi-platform/main.go` | 5 | ~60 |
| `examples/multi-platform/README.md` | 5 | ~30 |
| `examples/permissions/main.go` | 5 | ~60 |
| `examples/permissions/README.md` | 5 | ~30 |

---

## Decisiones Técnicas

| # | Decisión | Rationale |
|---|---|---|
| 1 | JSON para config (no YAML) | Consistente con zero deps. `encoding/json` es stdlib. |
| 2 | Circuit breaker sin dep externa | ~80 LOC puro Go. No justifica importar `gobreaker` o similar. |
| 3 | No token counting exacto (tiktoken) | Heurística mejorada + `ModelInfo` es suficiente. tiktoken en Go puro requiere dep pesada. |
| 4 | Ejemplos usan mock provider | Para que compilen y se testeen sin API keys reales. |
| 5 | CI solo unit tests | Integration tests requieren API keys. Se corren manualmente con `go test -tags=integration`. |
| 6 | FailFast usa context cancellation | Un `ctx` compartido entre tools paralelas. Si una falla → `cancel()` → resto aborta via `ctx.Done()`. |
| 7 | Streaming acumula en buffer | `ChatStream()` devuelve `<-chan StreamChunk`. Runner consume, emite callbacks, y construye `Response` final acumulando chunks. |
| 8 | Session cleanup opt-in | `StartCleanup()` debe llamarse explícitamente. No arranca goroutines automáticamente (filosofía: no magic). |

---

## Verificación Global (Post todas las fases)

```bash
# 1. Compila limpio
go build ./...
go build ./cmd/daneel/

# 2. Tests pasan
go test -race ./...

# 3. Sin warnings
go vet ./...

# 4. Coverage objetivo
go test -coverprofile=cov.out ./...
go tool cover -func=cov.out | tail -1  # > 80%

# 5. Ejemplos compilan
for d in examples/*/; do (cd "$d" && go build .); done

# 6. Config funcional
# go test -run TestLoadConfig ./...

# 7. Streaming end-to-end (manual con API key real)
# OPENAI_API_KEY=... go test -run TestStreamingLive -tags=integration ./provider/openai/
```

---

## Excluido de este Plan

Lo siguiente queda para post-v1.1 (roadmap futuro):

- Dashboard web UI
- Wasm plugins (wazero)
- go-plugin support (HashiCorp)
- Guardrails DSL
- Agent marketplace
- Distributed agents
- Conversation branching
- PDF/DOCX parsing nativo en knowledge base

---

*"The Laws of Robotics are not suggestions." — R. Daneel Olivaw*
