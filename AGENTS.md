# AGENTS Guide for FIELDWORK

## Repository State
Milestone 0 scaffold is in place.

Observed top-level files/directories:
- `cmd/fieldwork/` Cobra CLI (`dig`, `query`, `status`)
- `config/config.go` env loading + validation
- `python/marker_service/` FastAPI stub service
- `.env.example`, `Makefile`, `docker-compose.yml`, `README.md`
- `ROADMAP.md` remains the planning source for M1+

There is no ingestion/query implementation yet (`internal/` packages do not exist).

## Verified Commands
- Build: `make build`
- Run status: `make status`
- Run dig stub: `make dig`
- Run query stub: `make query QUERY="text"`
- Start local stack: `make service`
- Clean binary: `make clean`
- Go tests: `go test ./...`
- Python syntax check: `python3 -m compileall python/marker_service`

## CLI Behavior (Current)
- `fieldwork dig` is a stub that prints selected flags.
- `fieldwork query <text>` is a stub that prints selected flags.
- `fieldwork status` prints environment values and service checks.

Status checks implemented:
- Marker: `GET {MARKER_SERVICE_URL}/health`
- Qdrant: `GET {QDRANT_URL}/collections`
- Redis: `PING` using `redis://` URL

Gotcha: `fieldwork status` exits non-zero if any service is unhealthy.

## Config and Environment
`config.Load()` uses `.env` via `godotenv` if present, then defaults.

Configured keys are:
- `ARXIV_CATEGORIES`, `ARXIV_PAPER_LIMIT`, `ARXIV_RATE_LIMIT_SECONDS`
- `GEMINI_API_KEY`, `GEMINI_EMBEDDING_MODEL`, `GEMINI_EMBEDDING_DIMENSIONS`
- `MARKER_SERVICE_URL`, `MARKER_SERVICE_TIMEOUT_SECONDS`
- `QDRANT_URL`, `QDRANT_COLLECTION`
- `REDIS_URL`, `REDIS_CACHE_TTL_HOURS`, `REDIS_CACHE_SIMILARITY_THRESHOLD`
- `PIPELINE_WORKERS`, `PDF_CACHE_DIR`, `LOG_LEVEL`

Validation enforces non-empty service URLs/collection/log level and numeric bounds.

## Marker Sidecar (Current)
`python/marker_service/main.py` exposes:
- `GET /health` -> `{ "status": "ok", "marker_version": "stub" }`
- `POST /parse` with `{ "pdf_path": string }` -> stubbed parse payload

Data models are in `python/marker_service/models.py`.

## Dependency Baseline
Go module includes roadmap stack dependencies (not all are used yet), including:
- `github.com/spf13/cobra`
- `github.com/joho/godotenv`
- `github.com/redis/go-redis/v9`
- `github.com/qdrant/go-client v1.17.1`
- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/lipgloss`
- `google.golang.org/genai`

Python requirements are pinned in `python/marker_service/requirements.txt`.

## Implementation Constraints to Preserve (from ROADMAP)
When implementing M1+ keep these exact invariants:
- arXiv categories default: `quant-ph,gr-qc,hep-th,hep-ph,math-ph,cond-mat`
- arXiv rate limit: 3 seconds between PDF requests
- default paper limit: 50
- Qdrant collection: `fieldwork`
- named vectors: `text`, `abstract`, `chart`
- embedding dimension: 3072
- Redis status key schema:
  - `fieldwork:paper:{arxiv_id}:status`
  - `fieldwork:paper:{arxiv_id}:meta`
  - `fieldwork:dig:current`
  - `fieldwork:dig:stats`

## Near-Term Build Order
Follow roadmap sequence strictly:
1. M1 arXiv harvester + Redis status/meta store
2. M2 Marker parse client integration
3. M3 Gemini embedding layer + chunking
4. M4 Qdrant upsert + pipeline orchestration

Treat each roadmap "Done when" section as the acceptance gate before moving forward.
