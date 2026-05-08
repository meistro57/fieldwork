# FIELDWORK — Project Roadmap

> *Fine-grained semantic excavation of physics pre-prints.*
> Go CLI · Gemini Embedding 2 · Marker · Qdrant · Redis

---

## Vision Statement

FIELDWORK is an automated research ingestion and retrieval system targeting the arXiv physics pre-print ecosystem. It harvests the latest fifty papers across quantum mechanics and related disciplines, decomposes their structural and visual elements with vision-guided parsing, encodes them into a natively multimodal 3072-dimension vector space, and stores the result in a local Qdrant instance — enabling sub-second semantic search across text, equations, and scientific charts simultaneously.

The project sits inside the KAE ecosystem as a domain-specific sibling: where KAE explores broad consciousness literature, FIELDWORK goes deep into formal physics, treating arXiv as a continuously refreshing dig site.

---

## Architecture Overview

```
arXiv Atom API
      │
      ▼
[ Go Harvester ]  ──── rate limiter (3s) ────▶  /data/pdfs/
      │
      ▼  Redis Streams (task queue)
      │
      ▼
[ Python Marker Service ]  (FastAPI sidecar)
      │  POST /parse
      ▼
  Markdown + Charts (base64 + context windows)
      │
      ▼
[ Go Embedder ]  ──▶  Gemini Embedding 2 (3072-dim)
      │                 text | abstract | chart+context
      ▼
[ Qdrant fieldwork collection ]
      │  named vectors: text / abstract / chart
      ▼
[ Go Query CLI ]  ◀──  Redis Semantic Cache (RedisVL)
```

---

## Milestones

---

### MILESTONE 0 — Repository Foundation
**Goal:** Runnable skeleton. All subcommands registered, config loading, services reachable.

**Deliverables:**
- `go.mod` / `go.sum` initialized with all dependencies declared
- `.env.example` with all required environment variables documented
- `config/config.go` — typed struct, `godotenv` loader, validation on startup
- `cmd/fieldwork/main.go` — Cobra CLI with three subcommands wired: `dig`, `query`, `status`
- `Makefile` — `make build`, `make service`, `make dig`, `make query`, `make status`, `make clean`
- `python/marker_service/` — empty FastAPI skeleton with `/health` and `/parse` stubs
- `python/marker_service/Dockerfile` — Python 3.11 image, Marker installed, service starts on port 8000
- `docker-compose.yml` — Marker service + existing Qdrant + Redis wired together
- `README.md` — quickstart, prereqs, env vars, make commands

**Done when:** `make build` compiles, `fieldwork status` prints env/service health, Marker `/health` returns 200.

---

### MILESTONE 1 — arXiv Harvester
**Goal:** Reliably pull fifty papers from targeted physics categories into local PDF cache.

**Deliverables:**
- `internal/arxiv/client.go`
  - Atom XML fetcher with configurable category list
  - Boolean OR query across `quant-ph`, `gr-qc`, `hep-th`, `hep-ph`, `math-ph`, `cond-mat`
  - Sort by `submittedDate` descending, fetch top 50
  - Structs: `Paper{ID, Title, Authors, Abstract, Categories, SubmittedAt, PDFUrl}`
- `internal/harvester/harvester.go`
  - PDF downloader with 3-second inter-request rate limiter (respects arXiv ToS)
  - Skips already-downloaded papers by checking `/data/pdfs/{arxiv_id}.pdf` existence
  - Writes paper metadata to Redis hash: `fieldwork:paper:{id}:meta`
  - Sets paper status to `downloaded` in Redis: `fieldwork:paper:{id}:status`
- `internal/store/redis.go`
  - Connection pool, health check on startup
  - Status helpers: `SetStatus(id, status)`, `GetStatus(id)`, `ListByStatus(status)`
  - Meta helpers: `SetMeta(id, paper)`, `GetMeta(id)`
- `cmd/fieldwork/dig.go` — Phase 1 of `dig` subcommand: harvest only

**Redis Key Schema:**
```
fieldwork:paper:{arxiv_id}:status    → queued|downloaded|parsed|embedded|done|failed
fieldwork:paper:{arxiv_id}:meta      → hash: title, authors, categories, submitted_at, pdf_url
fieldwork:dig:current                → string: timestamp of active dig run
fieldwork:dig:stats                  → hash: total, downloaded, parsed, embedded, done, failed
```

**Done when:** `fieldwork dig --phase=harvest` downloads 50 PDFs and all status keys are set in Redis.

---

### MILESTONE 2 — Marker Parsing Service
**Goal:** Python FastAPI sidecar that accepts a PDF path and returns structured Markdown + chart objects.

**Deliverables:**
- `python/marker_service/main.py`
  - `POST /parse` — accepts `{ "pdf_path": string }`, returns full parse result
  - Runs Marker's `convert_single_pdf()` internally
  - Extracts chart image regions as base64 PNG
  - For each chart: captures 200-char context window before and after in the Markdown
  - Returns:
    ```json
    {
      "markdown": "...",
      "title": "...",
      "sections": ["Introduction", "Methods", ...],
      "charts": [
        {
          "index": 0,
          "image_b64": "...",
          "context_before": "...",
          "context_after": "...",
          "caption": "..."
        }
      ],
      "latex_blocks": ["\\hat{H}\\Psi = ...", "..."]
    }
    ```
  - `GET /health` — returns `{ "status": "ok", "marker_version": "..." }`
- `python/marker_service/requirements.txt` — pinned versions
- `python/marker_service/models.py` — Pydantic models for request/response
- `internal/marker/client.go`
  - HTTP client targeting `MARKER_SERVICE_URL` env var
  - `Parse(pdfPath string) (*ParseResult, error)`
  - 120-second timeout (large PDFs can be slow)
  - Retries: 2 attempts with 5s backoff

**Done when:** `curl -X POST localhost:8000/parse -d '{"pdf_path": "..."}' ` returns valid JSON with markdown and charts.

---

### MILESTONE 3 — Gemini Embedding Layer
**Goal:** Encode text chunks and chart+context pairs into 3072-dimension vectors.

**Deliverables:**
- `internal/embedder/gemini.go`
  - Google Gemini Embedding 2 client (`text-embedding-004` or `gemini-embedding-exp-03-07`)
  - `EmbedText(text string, taskType string) ([]float32, error)` — task types: `RETRIEVAL_DOCUMENT`, `RETRIEVAL_QUERY`
  - `EmbedImage(imageB64 string, contextBefore string, contextAfter string) ([]float32, error)` — interleaved multimodal payload
  - `EmbedBatch(inputs []EmbedInput) ([][]float32, error)` — batched for efficiency
  - Respects Gemini API rate limits with exponential backoff
  - Dimension: 3072 (MRL full precision)
- Chunking strategy for `text` vectors:
  - Split Markdown by section headers first
  - Then by 512-token sliding window with 64-token overlap
  - Each chunk stores: `arxiv_id`, `section`, `chunk_index`, `latex_blocks[]`, `token_count`
- `internal/embedder/types.go` — `EmbedInput`, `EmbedResult`, `ChunkMeta` structs

**Embedding Task Type Matrix:**
| Vector Name | Content | Gemini Task Type |
|---|---|---|
| `text` | Markdown section chunks | `RETRIEVAL_DOCUMENT` |
| `abstract` | Full abstract string | `RETRIEVAL_DOCUMENT` |
| `chart` | Image B64 + before + after context | `RETRIEVAL_DOCUMENT` |
| Query time | User query string | `RETRIEVAL_QUERY` |

**Done when:** Single paper produces valid 3072-dim vectors for text chunks, abstract, and all charts.

---

### MILESTONE 4 — Qdrant Ingestion
**Goal:** Upsert all vectors and payloads into the `fieldwork` Qdrant collection with named vector support.

**Deliverables:**
- `internal/store/qdrant.go`
  - Collection bootstrap: create `fieldwork` if not exists, with named vectors configured
  - Named vector definitions:
    - `text` — size 3072, Distance: Cosine
    - `abstract` — size 3072, Distance: Cosine
    - `chart` — size 3072, Distance: Cosine
  - `UpsertTextChunk(arxivID, chunk ChunkMeta, vector []float32) error`
  - `UpsertAbstract(paper Paper, vector []float32) error`
  - `UpsertChart(arxivID string, chart ChartMeta, vector []float32) error`
  - Batch upsert: groups of 100 points per request
  - Payload schema per point type (see below)
- `internal/pipeline/pipeline.go`
  - Orchestrates full paper lifecycle: download → parse → embed → upsert → mark done
  - Concurrent workers (configurable, default 3) processing Redis queue
  - Updates Redis status at each stage transition
  - Catches and logs failures without halting the batch

**Qdrant Payload Schema:**

*Text chunk point:*
```json
{
  "arxiv_id": "2401.12345",
  "point_type": "text_chunk",
  "title": "...",
  "authors": ["..."],
  "categories": ["quant-ph"],
  "submitted_at": "2024-01-15T00:00:00Z",
  "section": "3. Results",
  "chunk_index": 4,
  "chunk_text": "...",
  "latex_blocks": ["\\hat{H}\\Psi = E\\Psi"],
  "pdf_url": "https://arxiv.org/pdf/2401.12345"
}
```

*Chart point:*
```json
{
  "arxiv_id": "2401.12345",
  "point_type": "chart",
  "title": "...",
  "chart_index": 2,
  "caption": "...",
  "context_before": "...",
  "context_after": "...",
  "categories": ["quant-ph"]
}
```

**Done when:** `fieldwork dig` on a single paper produces queryable points in Qdrant with all three named vectors populated.

---

### MILESTONE 5 — Full Dig Pipeline
**Goal:** End-to-end `fieldwork dig` command processes all 50 papers with progress reporting and resumability.

**Deliverables:**
- Full pipeline wired: harvest → parse → embed → upsert for all 50 papers
- Redis Streams as task buffer — papers enqueue as `queued`, workers drain the stream
- Resumability: re-running `fieldwork dig` skips any paper already at `done` status
- `--force` flag: re-processes all papers regardless of status
- `--category` flag: target specific arXiv category for a focused dig
- `--limit` flag: override the default 50 paper limit
- Progress output: live terminal table showing paper ID, title (truncated), status, elapsed time
- Final summary: total processed, succeeded, failed, total vectors upserted, dig duration
- Dig run metadata stored: `fieldwork:dig:{timestamp}:summary`

**Done when:** `fieldwork dig` runs unattended on 50 papers, handles Marker/Gemini failures gracefully, and is fully resumable after interruption.

---

### MILESTONE 6 — Query Interface
**Goal:** `fieldwork query` enables semantic search across text, abstracts, and charts with Redis semantic caching.

**Deliverables:**
- `internal/store/redis.go` additions:
  - Semantic cache: store query embedding + result in Redis with configurable TTL (default 24h)
  - Cache lookup: cosine similarity threshold (default 0.92) to detect cache hit
  - `fieldwork:cache:{hash}` key structure
- `cmd/fieldwork/query.go`
  - Embeds user query with `RETRIEVAL_QUERY` task type
  - Checks Redis semantic cache first — returns cached result in <10ms if hit
  - Falls back to Qdrant search across named vectors
  - `--vector` flag: target specific vector (`text`, `abstract`, `chart`, `all`)
  - `--limit` flag: number of results (default 5)
  - `--category` flag: filter by arXiv category
  - `--type` flag: filter by point type (`text_chunk`, `chart`, `abstract`)
  - Output: formatted results with arxiv ID, title, section, relevance score, snippet
  - `--json` flag: raw JSON output for piping
- `internal/query/formatter.go` — terminal output formatting (lipgloss for styled output)

**Query examples:**
```bash
fieldwork query "experimental evidence for non-adiabatic transitions"
fieldwork query "Bose-Einstein condensate vortex formation" --vector chart
fieldwork query "Hamiltonian with spin-orbit coupling" --type text_chunk --limit 10
fieldwork query "gravitational wave detection" --category gr-qc --json
```

**Done when:** All query flags work, semantic cache demonstrably returns hits on repeated similar queries.

---

### MILESTONE 7 — Status Dashboard (Bubbletea TUI)
**Goal:** `fieldwork status` renders a live terminal dashboard showing dig progress and collection health.

**Deliverables:**
- `cmd/fieldwork/status.go` — Bubbletea TUI model
- Panels:
  - **Dig Status** — current/last dig: started, elapsed, paper counts by status, worker activity
  - **Collection Stats** — Qdrant `fieldwork` collection: total points, breakdown by vector type and point_type
  - **Cache Stats** — Redis: total cache keys, hit rate (stored in Redis counter), memory usage
  - **Recent Activity** — last 10 status transitions pulled from Redis, auto-refreshing every 2s
- `--once` flag: print status snapshot and exit (no TUI, for scripting/Lewis integration)

**Done when:** `fieldwork status` renders a live dashboard during a dig run, updates in real time.

---

### MILESTONE 8 — Lewis Integration (Discord Commander)
**Goal:** Wire FIELDWORK into the Lewis Discord agent so digs can be triggered and monitored remotely.

**Deliverables:**
- `fieldwork status --once --json` — machine-readable status output for Lewis consumption
- Lewis command bindings (in OpenClaw/Lewis config):
  - `!fw dig` — trigger `fieldwork dig` on BOXX, stream status updates to Discord
  - `!fw status` — post current dig status summary to channel
  - `!fw query <text>` — run a query and post top 3 results to Discord
  - `!fw stats` — post Qdrant collection stats
- Discord status messages: paper count, vector count, cache hit rate, any failures
- Lewis polls `fieldwork:dig:stats` Redis key every 30s during active dig

**Done when:** A dig can be triggered entirely from Discord and results reported back to channel.

---

### MILESTONE 9 — Vectoreologist Lens Integration
**Goal:** Add `fieldwork` collection to the Vectoreologist TUI for topology analysis alongside KAE collections.

**Deliverables:**
- Vectoreologist collection selector includes `fieldwork` alongside existing collections
- UMAP clustering view for `fieldwork` text vectors — physics sub-discipline clusters should be visually distinct
- Cross-collection query: find semantic neighbors in `fieldwork` from a KAE node (physics ↔ consciousness literature crossover detection)
- Export: cluster report as Markdown field notes

**Done when:** Vectoreologist can load and cluster `fieldwork` vectors and cross-reference against KAE collections.

---

### MILESTONE 10 — Scheduled Refresh + Anomalyzer Hook
**Goal:** FIELDWORK auto-refreshes on a schedule and feeds novel papers to the planned Anomalyzer.

**Deliverables:**
- `fieldwork dig --schedule=daily` — cron-style scheduling (runs at 06:00 UTC by default)
- Novelty detection: compare incoming paper abstracts against existing collection — flag papers with low similarity to anything in the corpus as "high novelty"
- `fieldwork:novel:{arxiv_id}` Redis key for flagged papers
- Anomalyzer interface stub: `internal/anomalyzer/anomalyzer.go` — accepts novel points, ready for cross-domain pattern analysis against other KAE collections
- Lewis alert: notifies Discord when high-novelty papers are flagged

**Done when:** Daily dig runs automatically, novel papers are flagged, Lewis reports them.

---

## Environment Variables

```env
# arXiv
ARXIV_CATEGORIES=quant-ph,gr-qc,hep-th,hep-ph,math-ph,cond-mat
ARXIV_PAPER_LIMIT=50
ARXIV_RATE_LIMIT_SECONDS=3

# Gemini
GEMINI_API_KEY=
GEMINI_EMBEDDING_MODEL=text-embedding-004
GEMINI_EMBEDDING_DIMENSIONS=3072

# Marker Service
MARKER_SERVICE_URL=http://localhost:8000
MARKER_SERVICE_TIMEOUT_SECONDS=120

# Qdrant
QDRANT_URL=http://localhost:6333
QDRANT_COLLECTION=fieldwork

# Redis
REDIS_URL=redis://localhost:6379
REDIS_CACHE_TTL_HOURS=24
REDIS_CACHE_SIMILARITY_THRESHOLD=0.92

# Pipeline
PIPELINE_WORKERS=3
PDF_CACHE_DIR=./data/pdfs
LOG_LEVEL=info
```

---

## Dependency Manifest

**Go:**
```
github.com/qdrant/go-client              v1.17.1
github.com/redis/go-redis/v9
github.com/spf13/cobra
github.com/charmbracelet/bubbletea
github.com/charmbracelet/lipgloss
github.com/joho/godotenv
google.golang.org/genai                  (Gemini SDK)
```

**Python (Marker Service):**
```
fastapi
uvicorn
marker-pdf
pydantic
Pillow
```

---

## Project Status Tracker

| Milestone | Status | Notes |
|---|---|---|
| M0 — Repository Foundation | ⬜ Not Started | |
| M1 — arXiv Harvester | ⬜ Not Started | |
| M2 — Marker Parsing Service | ⬜ Not Started | |
| M3 — Gemini Embedding Layer | ⬜ Not Started | |
| M4 — Qdrant Ingestion | ⬜ Not Started | |
| M5 — Full Dig Pipeline | ⬜ Not Started | |
| M6 — Query Interface | ⬜ Not Started | |
| M7 — Status Dashboard (TUI) | ⬜ Not Started | |
| M8 — Lewis Integration | ⬜ Not Started | |
| M9 — Vectoreologist Lens | ⬜ Not Started | |
| M10 — Scheduled Refresh + Anomalyzer Hook | ⬜ Not Started | |

---

*FIELDWORK is part of the KAE ecosystem — Knowledge Archaeology Engine.*
*Where quantum literature meets the ground.*
