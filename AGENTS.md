# AGENTS Guide for FIELDWORK

## Repository State
Milestone 0 and Milestone 1 are complete. Milestone 2 parsing foundation is implemented.

Observed top-level files/directories:
- `cmd/fieldwork/` Cobra CLI (`dig`, `query`, `status`)
- `config/config.go` env loading + validation
- `internal/arxiv/` Atom API client + parser
- `internal/harvester/` PDF harvest workflow
- `internal/marker/` Go client for Marker service parse endpoint
- `internal/store/` Redis store helpers (status/meta/dig keys)
- `python/marker_service/` FastAPI parse service
- `.env.example`, `Makefile`, `docker-compose.yml`, `README.md`, `ROADMAP.md`

## Verified Commands
- Build: `make build`
- Start services: `make service`
- Status check: `make status`
- Harvest run: `make dig ARGS="--phase harvest --limit 5"`
- Full run (harvest + parse): `make dig ARGS="--phase full --limit 5"`
- Query stub: `make query QUERY="text"`
- Clean binary: `make clean`
- Go tests: `go test ./...`
- Python syntax check: `python3 -m compileall python/marker_service`

## CLI Behavior (Current)
- `fieldwork dig`
  - `--phase harvest` executes Milestone 1 pipeline.
  - `--phase full` executes harvest, then parses all papers currently in Redis status `downloaded` using marker service.
  - Parsed papers are moved to `parsed`; parse failures are moved to `failed`.
  - Supports `--category`, `--limit`, `--force` flags (`force` still parsed but not behaviorally implemented).
- `fieldwork query <text>` is still a stub.
- `fieldwork status` prints env values and service checks.

Status checks implemented:
- Marker: `GET {MARKER_SERVICE_URL}/health`
- Qdrant: `GET {QDRANT_URL}/collections`
- Redis: `PING` using `redis://` URL

Gotcha: `fieldwork status` exits non-zero if any service is unhealthy.

## Milestone 1 Implementation Details
### arXiv client (`internal/arxiv/client.go`)
- Uses `http://export.arxiv.org/api/query`.
- Builds `search_query` as OR over categories (`cat:x+OR+cat:y`).
- Requests are sorted by `submittedDate` descending.
- Maps Atom entries to:
  - `Paper{ID, Title, Authors, Abstract, Categories, SubmittedAt, PDFURL}`
- Extracts PDF link from Atom links (`title=pdf` / `type=application/pdf`), with fallback URL construction.

### Harvester (`internal/harvester/harvester.go`)
- Fetches latest papers from arXiv.
- Ensures cache dir exists (`PDF_CACHE_DIR`).
- Downloads PDFs with configured rate limit (`ARXIV_RATE_LIMIT_SECONDS`).
- Skips already-existing PDFs.
- Writes metadata and status to Redis per paper.
- Writes dig run keys:
  - `fieldwork:dig:current`
  - `fieldwork:dig:stats`

### Redis store (`internal/store/redis.go`)
- Connection via `redis.ParseURL` + startup ping health check.
- Helpers implemented:
  - `SetStatus`, `GetStatus`, `ListByStatus`
  - `SetMeta`, `GetMeta`
  - `SetDigCurrent`, `SetDigStats`

## Milestone 2 Foundation Details
### Marker service (`python/marker_service/main.py`)
- `GET /health` returns `{status, marker_version}`.
- `POST /parse` validates `pdf_path`, attempts Marker `convert_single_pdf`, and normalizes output into:
  - `markdown`, `title`, `sections`, `charts`, `latex_blocks`
- If Marker module import fails, returns a fallback empty markdown payload with derived title.
- Chart extraction accepts several metadata key variants (`charts|figures|images`, image base64 key variants), validates base64, and derives ±200-char context from markdown.

### Go marker client (`internal/marker/client.go`)
- `Parse(pdfPath string) (*ParseResult, error)`
- Uses `MARKER_SERVICE_URL`
- Timeout configurable from `MARKER_SERVICE_TIMEOUT_SECONDS` (default 120)
- Retries: 2 attempts with 5s backoff (configurable fields on client)

### Dig full-phase behavior (`cmd/fieldwork/dig.go`)
- Runs harvest first.
- Lists papers in Redis with status `downloaded`.
- Calls marker parse endpoint for each cached PDF path.
- Updates status `parsed` or `failed` and writes updated dig stats.

## Redis Key Contract (In Use)
- `fieldwork:paper:{arxiv_id}:status` -> status string (`downloaded|parsed|failed` currently exercised)
- `fieldwork:paper:{arxiv_id}:meta` -> hash (`title`, `authors`, `categories`, `submitted_at`, `pdf_url`)
- `fieldwork:dig:current` -> current dig timestamp
- `fieldwork:dig:stats` -> hash (`total`, `downloaded`, `parsed`, `embedded`, `done`, `failed`)

## Testing Coverage (Current)
- `internal/arxiv/client_test.go`
  - Query construction and Atom parsing
  - non-2xx handling
- `internal/store/redis_test.go`
  - status/meta/dig key helpers using miniredis
- `internal/harvester/harvester_test.go`
  - end-to-end harvest of one paper via httptest + miniredis
- `internal/marker/client_test.go`
  - parse success
  - retry-once behavior
  - non-2xx error propagation

## Non-Obvious Gotchas
- Harvest requires live network access to arXiv unless tests/mocks are used.
- arXiv IDs include version suffixes (example: `2401.12345v1`), and that exact ID is used for PDF filename and Redis key segment.
- `force` flag is parsed by CLI but not yet enforced in harvester/full-pipeline logic.
- `dig --phase full` currently depends on marker service availability; no local fallback in Go client path.
- Marker Python dependency may not be present in host env unless installed from `python/marker_service/requirements.txt`; service still returns fallback parse payload when Marker import fails.

## Next Build Order
Follow roadmap sequence:
1. Complete M2 internals (real Marker conversion output schema persistence)
2. M3 Gemini embedding layer + chunking
3. M4 Qdrant ingestion + pipeline orchestration
4. M5 full resumable dig behavior (`force`, queue workers, progress)
