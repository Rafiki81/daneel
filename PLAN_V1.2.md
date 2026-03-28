# Daneel v1.2 — Local-First AI + Contenedores + CI/CD

> Plan de mejoras post-v1.1. Cierra Fases 5 y 6 pendientes, añade soporte completo para IA 100% local (embeddings, convenience API, doctor CLI), infraestructura de contenedores, y actualización del README.

---

## Estado Global

| Fase | Estado | Resumen |
|---|---|---|
| **Fase 5** — Ejemplos + CI/CD + Docker | ⬜ Pendiente | Hereda de v1.1 + Dockerfile, docker-compose, .dockerignore |
| **Fase 6** — Observabilidad Mejorada | ⬜ Pendiente | Hereda de v1.1 sin cambios |
| **Fase 7** — Local-First AI Stack | ⬜ Pendiente | OllamaEmbedder, WithLocalStack, auto-detect, `daneel doctor` |
| **Fase 8** — README + Documentación | ⬜ Pendiente | Secciones Local AI, Docker, actualización de features |

---

## Motivación

Daneel tiene zero external dependencies y soporte para Ollama como provider local. Sin embargo, el ciclo completo de IA local está incompleto:

1. **Embeddings** — La interfaz `Embedder` existe, `Vector` memory y `KnowledgeBase` la aceptan, pero **no hay implementación local incluida**. Sin ella, RAG no funciona offline.
2. **Ergonomía** — Configurar un stack local requiere wiring manual (Ollama provider + file store + memory). Falta un `WithLocalStack()` que lo haga en una línea.
3. **Diagnóstico** — No hay forma de verificar que Ollama está corriendo, el modelo descargado, y el embedder disponible. Falta un `daneel doctor`.
4. **Despliegue** — No hay Dockerfile ni docker-compose. El CLI `daneel listen` es un proceso long-running candidato a contenedor.

---

## Grafo de Dependencias

```
Fase 7 (Local-First) ──────────┐
Fase 6 (Observabilidad) ───────┼──→ Fase 5 (Ejemplos/CI/Docker)
                                │         │
                                └─────────┼──→ Fase 8 (README)
```

- **Fase 7** es independiente — puede empezar inmediatamente.
- **Fase 6** es independiente — paralela con todo.
- **Fase 5** depende de 7 (para el ejemplo `local-rag/`) y de 6 (para tests de métricas).
- **Fase 8** va al final — documenta todo lo nuevo.

---

## Fases de Implementación

### Fase 5: Ejemplos + CI/CD + Docker + Provider Tests

> Hereda tareas 5.1–5.5 del PLAN_V1.1 + nuevas tareas de contenedores.

| # | Tarea | Archivo(s) | Descripción |
|---|---|---|---|
| 5.1 | Ejemplos completos | `examples/*/main.go` + `README.md` | Implementar los 6 directorios (5 originales + `local-rag`). Cada uno con mock provider para compilar sin API keys. |
| 5.2 | CI/CD pipeline | `.github/workflows/ci.yml` | `go test ./...`, `go vet ./...`, `gofmt -s -l`, coverage. Matrix: Go 1.24+. |
| 5.3 | Makefile | `Makefile` | Targets: `test`, `lint`, `coverage`, `build`, `examples`, `docker-build`, `docker-run`. |
| 5.4 | Provider tests | `provider/*/..._test.go` | `httptest.Server` mocks para OpenAI, Anthropic, Google, Ollama. |
| 5.5 | Godoc examples | `doc.go` | `Example_*` functions para quickstart, memoria, workflows, local stack. |
| 5.6 | Dockerfile | `Dockerfile` (nuevo) | Multi-stage build para `cmd/daneel`. Imagen final `scratch` o `distroless`. |
| 5.7 | Docker Compose | `docker-compose.yml` (nuevo) | Ejemplo: Daneel + Ollama en red local. Configuración por env vars. |
| 5.8 | .dockerignore | `.dockerignore` (nuevo) | Excluir testdata, docs, .git, etc. |
| 5.9 | Ejemplo local-rag | `examples/local-rag/main.go` + `README.md` | RAG completo sin cloud: Ollama + OllamaEmbedder + KnowledgeBase + Vector memory. |

**Ejemplos a implementar**:

| Directorio | Qué Demuestra |
|---|---|
| `examples/quickstart/` | Ya existe — verificar que compila limpio |
| `examples/slack-assistant/` | Bot Slack con memoria sliding + approval workflow |
| `examples/github-reviewer/` | PR review agent con GitHub tools |
| `examples/twitter-bot/` | Bot que responde a menciones automáticamente |
| `examples/multi-platform/` | Un agente en Slack + Telegram simultáneo via Bridge |
| `examples/permissions/` | Demo de allow/deny lists + guardrails + strict mode |
| `examples/local-rag/` | **NUEVO** — RAG completo 100% local con Ollama + embeddings |

**Dockerfile — Diseño**:

```dockerfile
# Build stage
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /daneel ./cmd/daneel/

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /daneel /daneel
ENTRYPOINT ["/daneel"]
```

**Docker Compose — Diseño**:

```yaml
services:
  ollama:
    image: ollama/ollama:latest
    ports: ["11434:11434"]
    volumes: ["ollama_data:/root/.ollama"]

  daneel:
    build: .
    command: ["listen", "--config", "/etc/daneel/config.json"]
    environment:
      - OLLAMA_BASE_URL=http://ollama:11434
    volumes:
      - ./config.json:/etc/daneel/config.json:ro
      - daneel_sessions:/data/sessions
    depends_on: [ollama]

volumes:
  ollama_data:
  daneel_sessions:
```

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
      - run: test -z "$(gofmt -s -l .)"
  docker:
    runs-on: ubuntu-latest
    needs: test
    steps:
      - uses: actions/checkout@v4
      - run: docker build -t daneel:ci .
```

**Verificación**: `make test` pasa. `make lint` pasa. `docker build .` funciona. Los 7 ejemplos compilan. CI green.

---

### Fase 6: Observabilidad Mejorada

> Sin cambios respecto a PLAN_V1.1. Se incluye aquí por completitud.

| # | Tarea | Archivo(s) | Descripción |
|---|---|---|---|
| 6.1 | Métricas en Bridge | `bridge/bridge.go` | `WithMetrics(BridgeMetrics)` option. Reportar: mensajes procesados, errores, latencia, sesiones activas. |
| 6.2 | Diagnóstico JSON | Providers + `tool.go` | Cuando JSON parsing falla: loguear vía `slog` primeros 500 chars. Incluir provider, modelo, status code. |
| 6.3 | Tests | `bridge/bridge_test.go` | Mock metrics → verificar contadores. JSON malformed → verificar log. |

**Interface**:

```go
type BridgeMetrics interface {
    RecordMessageProcessed(platform string, duration time.Duration, err error)
    RecordActiveConversations(count int)
    RecordCleanup(deletedSessions int)
}
```

**Verificación**: Bridge con mock metrics → contadores incrementan correctamente.

---

### Fase 7: Local-First AI Stack

> El cambio más significativo de v1.2. Cierra el gap de embeddings y hace que Daneel sea 100% usable sin cloud.

| # | Tarea | Archivo(s) | Descripción |
|---|---|---|---|
| 7.1 | `OllamaEmbedder` | `provider/ollama/embedder.go` (nuevo) | Implementar `daneel.Embedder` vía Ollama `/api/embed`. ~40 LOC. |
| 7.2 | `WithLocalStack()` | `daneel.go` | Convenience que configura Ollama provider + OllamaEmbedder + file session store en una línea. |
| 7.3 | Auto-detect Ollama | `daneel.go` | Si no hay provider explícito ni `OPENAI_API_KEY`, probar `http://localhost:11434` antes de fallar. |
| 7.4 | `daneel doctor` | `cmd/daneel/doctor.go` (nuevo) | Health check: Ollama up?, modelo descargado?, embedder disponible?, Python venv para finetune? |
| 7.5 | Tests embedder | `provider/ollama/embedder_test.go` (nuevo) | `httptest.Server` mock de `/api/embed`. Verificar dimensiones, errores, timeouts. |
| 7.6 | Tests local stack | `daneel_test.go` | Test `WithLocalStack` configura provider + embedder + session correctamente. |

#### 7.1 — OllamaEmbedder

El gap **más crítico**. Ollama expone `/api/embed` (antes `/api/embeddings`). Daneel tiene la interfaz `Embedder` y tanto `memory.Vector()` como `knowledge.New()` la aceptan, pero no hay implementación local.

**Diseño**:

```go
// provider/ollama/embedder.go
package ollama

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "bytes"
)

// Embedder implements daneel.Embedder using Ollama's /api/embed endpoint.
type Embedder struct {
    baseURL string
    model   string
    client  *http.Client
}

// EmbedderOption configures the Ollama embedder.
type EmbedderOption func(*Embedder)

func EmbedModel(m string) EmbedderOption {
    return func(e *Embedder) { e.model = m }
}

func EmbedBaseURL(u string) EmbedderOption {
    return func(e *Embedder) { e.baseURL = u }
}

func EmbedHTTPClient(c *http.Client) EmbedderOption {
    return func(e *Embedder) { e.client = c }
}

// NewEmbedder creates an Ollama-backed embedder.
func NewEmbedder(opts ...EmbedderOption) *Embedder {
    e := &Embedder{
        baseURL: "http://localhost:11434",
        model:   "nomic-embed-text",
        client:  &http.Client{},
    }
    for _, o := range opts {
        o(e)
    }
    return e
}

// Embed returns the embedding vector for the given text.
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
    body, _ := json.Marshal(map[string]string{
        "model": e.model,
        "input": text,
    })
    req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/api/embed", bytes.NewReader(body))
    if err != nil {
        return nil, err
    }
    req.Header.Set("Content-Type", "application/json")
    resp, err := e.client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("ollama embed: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("ollama embed: status %d", resp.StatusCode)
    }
    var result struct {
        Embeddings [][]float32 `json:"embeddings"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("ollama embed: decode: %w", err)
    }
    if len(result.Embeddings) == 0 {
        return nil, fmt.Errorf("ollama embed: empty response")
    }
    return result.Embeddings[0], nil
}
```

**Modelos de embedding recomendados para Ollama**:
- `nomic-embed-text` (768 dims, buena calidad, ~270MB)
- `mxbai-embed-large` (1024 dims, mejor calidad, ~670MB)
- `all-minilm` (384 dims, más rápido, ~45MB)

#### 7.2 — WithLocalStack

**Diseño**:

```go
// daneel.go

// WithLocalStack configures an agent for fully local operation:
// Ollama as LLM provider, Ollama embedder, and file-backed sessions.
// model is the LLM model (e.g. "llama3.3:70b").
// embedModel is the embedding model (e.g. "nomic-embed-text").
func WithLocalStack(model, embedModel string) AgentOption {
    return func(cfg *agentConfig) {
        p := ollama.New(ollama.WithModel(model))
        cfg.provider = p
        cfg.embedder = ollama.NewEmbedder(ollama.EmbedModel(embedModel))
    }
}
```

**Uso**:

```go
agent := daneel.New("assistant",
    daneel.WithLocalStack("llama3.3:70b", "nomic-embed-text"),
    daneel.WithInstructions("You are a helpful assistant."),
    daneel.WithMemory(memory.Vector(store, nil)), // embedder from config
)
```

#### 7.3 — Auto-detect Ollama

Modificar `resolveProvider()` en `daneel.go`:

```go
func resolveProvider(agent *Agent) *Agent {
    if agent.config.provider != nil {
        return agent
    }
    if agent.config.model == "" {
        return agent
    }
    // Try OPENAI_API_KEY first (existing behavior)
    if key := os.Getenv("OPENAI_API_KEY"); key != "" {
        cp := agent.clone()
        cp.config.provider = &miniClient{
            baseURL: "https://api.openai.com/v1",
            apiKey:  key,
            model:   agent.config.model,
        }
        return cp
    }
    // Auto-detect Ollama on localhost
    if ollamaAvailable() {
        cp := agent.clone()
        cp.config.provider = ollama.New(ollama.WithModel(agent.config.model))
        return cp
    }
    return agent
}

func ollamaAvailable() bool {
    ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
    defer cancel()
    req, _ := http.NewRequestWithContext(ctx, "GET", "http://localhost:11434/api/version", nil)
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return false
    }
    resp.Body.Close()
    return resp.StatusCode == http.StatusOK
}
```

#### 7.4 — `daneel doctor`

**Diseño**:

```go
// cmd/daneel/doctor.go

// $ daneel doctor
// ✅ Go 1.24.0
// ✅ Ollama running (http://localhost:11434) — v0.5.x
// ✅ Model "llama3.3:70b" available
// ✅ Embedding model "nomic-embed-text" available
// ⚠️  Python venv not found (.daneel-venv) — finetune unavailable
// ✅ Config valid (daneel.json)
//
// Summary: 5/6 checks passed

func doctorCmd() {
    checks := []struct {
        name string
        fn   func() (string, error)
    }{
        {"Go version", checkGo},
        {"Ollama server", checkOllama},
        {"LLM model", checkModel},
        {"Embedding model", checkEmbedModel},
        {"Python venv (finetune)", checkPythonVenv},
        {"Config file", checkConfig},
    }
    // ... run all, print results
}
```

**Checks a implementar**:

| Check | Qué verifica | Cómo |
|---|---|---|
| Go version | Go 1.24+ instalado | `runtime.Version()` |
| Ollama server | Ollama corriendo en localhost | `GET /api/version` |
| LLM model | Modelo descargado en Ollama | `GET /api/tags` → buscar en lista |
| Embedding model | Modelo de embeddings disponible | `GET /api/tags` → buscar en lista |
| Python venv | Venv de finetune configurado | `finetune.Check()` |
| Config file | `daneel.json` válido | `config.LoadConfig()` |

**Verificación Fase 7**: `OllamaEmbedder` funciona con mock httptest. `WithLocalStack` configura provider + embedder. Auto-detect encuentra Ollama local. `daneel doctor` reporta estado.

---

### Fase 8: README + Documentación

> Va al final — documenta todas las mejoras de v1.2.

| # | Tarea | Archivo(s) | Descripción |
|---|---|---|---|
| 8.1 | Sección "Local AI" en README | `README.md` | Nuevo bloque después de Quick Start: setup 100% local con Ollama. |
| 8.2 | Sección "Docker" en README | `README.md` | Cómo correr con Docker y docker-compose. |
| 8.3 | Actualizar tabla Features | `README.md` | Añadir Local AI, Docker, doctor CLI. |
| 8.4 | Actualizar tabla Examples | `README.md` | Añadir `local-rag` example. |
| 8.5 | Actualizar Package Index | `README.md` | Añadir `ollama.Embedder`, sección Docker. |
| 8.6 | Actualizar CLI section | `README.md` | Añadir `daneel doctor`. |
| 8.7 | Actualizar PLAN_V1.1.md | `PLAN_V1.1.md` | Marcar Fases 5 y 6 como completadas. |

#### 8.1 — Sección "Local AI" (a añadir después de Quick Start)

```markdown
## Local AI (No Cloud Required)

Daneel runs fully offline with [Ollama](https://ollama.com):

### Setup

​```sh
# 1. Install Ollama
curl -fsSL https://ollama.com/install.sh | sh

# 2. Pull models
ollama pull llama3.3:70b          # LLM
ollama pull nomic-embed-text       # Embeddings

# 3. Verify
daneel doctor
​```

### One-Line Local Stack

​```go
agent := daneel.New("assistant",
    daneel.WithLocalStack("llama3.3:70b", "nomic-embed-text"),
    daneel.WithInstructions("You are a helpful assistant."),
)
result, _ := daneel.Run(ctx, agent, "Hello!")
​```

### Local RAG (Knowledge Base)

​```go
import (
    "github.com/Rafiki81/daneel/knowledge"
    "github.com/Rafiki81/daneel/memory/store"
    "github.com/Rafiki81/daneel/provider/ollama"
)

embedder := ollama.NewEmbedder(ollama.EmbedModel("nomic-embed-text"))
vs := store.NewLocal("./vectors.db")
kb := knowledge.New(knowledge.WithEmbedder(embedder), knowledge.WithStore(vs))
kb.Ingest(ctx, "docs/")

agent := daneel.New("rag-assistant",
    daneel.WithLocalStack("llama3.3:70b", "nomic-embed-text"),
    daneel.WithKnowledge(kb),
)
​```
```

#### 8.2 — Sección "Docker" (a añadir después de CLI)

```markdown
## Docker

### Build

​```sh
docker build -t daneel .
​```

### Run with Ollama

​```sh
docker compose up
​```

This starts Daneel + Ollama. Configure via environment variables and `config.json`:

​```yaml
# docker-compose.yml
services:
  ollama:
    image: ollama/ollama:latest
    ports: ["11434:11434"]
  daneel:
    build: .
    command: ["listen", "--config", "/etc/daneel/config.json"]
    environment:
      - OLLAMA_BASE_URL=http://ollama:11434
    depends_on: [ollama]
​```
```

#### 8.3 — Actualización tabla Features

Añadir filas:

```markdown
| **Local AI** | `WithLocalStack`, `OllamaEmbedder`, auto-detect Ollama, `daneel doctor` |
| **Docker** | Multi-stage Dockerfile, docker-compose with Ollama |
```

---

## Inventario de Archivos

### Archivos a Crear

| Archivo | Fase | ~LOC |
|---|---|---|
| `provider/ollama/embedder.go` | 7 | ~60 |
| `provider/ollama/embedder_test.go` | 7 | ~80 |
| `cmd/daneel/doctor.go` | 7 | ~120 |
| `Dockerfile` | 5 | ~15 |
| `docker-compose.yml` | 5 | ~25 |
| `.dockerignore` | 5 | ~10 |
| `.github/workflows/ci.yml` | 5 | ~35 |
| `Makefile` | 5 | ~40 |
| `examples/local-rag/main.go` | 5 | ~70 |
| `examples/local-rag/README.md` | 5 | ~30 |
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
| `provider/openai/openai_test.go` | 5 | ~200 |
| `provider/anthropic/anthropic_test.go` | 5 | ~150 |
| `provider/google/google_test.go` | 5 | ~150 |
| `provider/ollama/ollama_test.go` | 5 | ~150 |

### Archivos a Modificar

| Archivo | Fase | Cambios |
|---|---|---|
| `daneel.go` | 7 | `WithLocalStack()`, `ollamaAvailable()`, modificar `resolveProvider()` |
| `bridge/bridge.go` | 6 | `WithMetrics(BridgeMetrics)` option |
| `tool.go` / providers | 6 | Diagnóstico JSON mejorado |
| `doc.go` | 5 | `Example_*` functions |
| `README.md` | 8 | Secciones Local AI, Docker, actualizar Features/Examples/CLI |
| `PLAN_V1.1.md` | 8 | Marcar Fases 5 y 6 como completadas |
| `cmd/daneel/main.go` | 7 | Registrar subcomando `doctor` |

---

## Decisiones Técnicas

| # | Decisión | Rationale |
|---|---|---|
| 1 | Ollama `/api/embed` (no `/api/embeddings`) | `/api/embed` es el endpoint actual de Ollama (v0.4+). Soporta batch nativo. |
| 2 | Default embed model: `nomic-embed-text` | Buen equilibrio calidad/tamaño (768 dims, ~270MB). Licencia Apache 2.0. |
| 3 | Auto-detect con timeout 500ms | Si Ollama no está corriendo, no añade lag perceptible. Solo se ejecuta si no hay provider explícito ni `OPENAI_API_KEY`. |
| 4 | `distroless` como base Docker | Imagen final ~5MB. Sin shell ni herramientas — máxima seguridad. |
| 5 | `daneel doctor` sin deps | Usa HTTP calls directos a Ollama API + `runtime.Version()`. Zero imports nuevos. |
| 6 | Embedder como paquete separado | `provider/ollama/embedder.go` — no mezclar con el `Provider` de chat. Pueden usarse independientemente. |
| 7 | `WithLocalStack` no auto-descarga modelos | Principio de menor sorpresa. El usuario descarga con `ollama pull` explícitamente. `doctor` le dice qué falta. |

---

## Orden de Ejecución Recomendado

```
1. Fase 7.1  → OllamaEmbedder (desbloquea RAG local)
2. Fase 7.2  → WithLocalStack (ergonomía)
3. Fase 7.3  → Auto-detect Ollama
4. Fase 7.5  → Tests embedder
5. Fase 6    → Observabilidad (paralela)
6. Fase 5.6  → Dockerfile + compose
7. Fase 5.1  → Ejemplos (incluye local-rag)
8. Fase 5.2  → CI/CD
9. Fase 5.3  → Makefile
10. Fase 5.4 → Provider tests
11. Fase 7.4 → daneel doctor
12. Fase 8   → README + docs (al final, documenta todo)
```

---

## Verificación Global

```bash
# 1. Compila limpio
go build ./...
go build ./cmd/daneel/

# 2. Tests pasan
go test -race ./...

# 3. Sin warnings
go vet ./...
test -z "$(gofmt -s -l .)"

# 4. Docker
docker build -t daneel .
docker compose config  # valida compose

# 5. Ejemplos compilan
for d in examples/*/; do (cd "$d" && go build .); done

# 6. Doctor (con Ollama corriendo)
# daneel doctor

# 7. Local RAG (manual)
# ollama pull nomic-embed-text
# go run examples/local-rag/main.go

# 8. Coverage
go test -coverprofile=cov.out ./...
go tool cover -func=cov.out | tail -1  # > 80%
```

---

## Matriz de Capacidades Post-v1.2

| Capacidad | Cloud | Local (Ollama) |
|---|---|---|
| LLM Chat | ✅ OpenAI/Anthropic/Google | ✅ Ollama |
| Streaming | ✅ Todos | ✅ Ollama |
| Tool Calling | ✅ Todos | ✅ Ollama |
| **Embeddings** | ✅ (externo) | ✅ **OllamaEmbedder** |
| **RAG / Knowledge** | ✅ | ✅ **Cerrado** |
| **Vector Memory** | ✅ | ✅ **Cerrado** |
| Fine-tuning | N/A | ✅ Python subprocess |
| Deploy modelo | N/A | ✅ `ImportToOllama` |
| Sessions | ✅ | ✅ File-backed |
| **Stack 1-liner** | `WithOpenAI()` | ✅ **`WithLocalStack()`** |
| **Health Check** | N/A | ✅ **`daneel doctor`** |
| **Containerized** | N/A | ✅ **Dockerfile + Compose** |

---

*"A robot shall protect its own existence — and its user's data sovereignty." — R. Daneel Olivaw, updated*
