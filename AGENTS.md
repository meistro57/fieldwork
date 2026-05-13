# AGENTS Guide for FIELDWORK

## Repository State
Milestone 0 and Milestone 1 are complete. Milestone 2 parsing foundation is implemented.

Observed top-level files/directories:
- `cmd/fieldwork/` Cobra CLI (`dig`, `query`, `status`)
- `config/config.go` env loading + validation (includes optional `MARKER_PDF_ROOT`)
- `internal/arxiv/` Legacy Atom API client + parser (kept, not wired by default)
- `internal/semanticscholar/` Semantic Scholar Graph API JSON client
- `internal/rss/` arXiv RSS feed XML client
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
### Semantic Scholar client (`internal/semanticscholar/client.go`)
- Uses `https://api.semanticscholar.org/graph/v1/paper/search`.
- Queries physics-focused terms (`quantum physics OR quantum mechanics OR condensed matter OR high energy physics`) and requests fields `title,abstract,authors,year,externalIds,openAccessPdf,publicationDate,fieldsOfStudy`.
- Requests are sorted by `publicationDate` descending.
- Maps JSON results to `arxiv.Paper{ID, Title, Authors, Abstract, Categories, SubmittedAt, PDFURL}`.
- Uses `externalIds.ArXiv` as paper ID and falls back to `https://arxiv.org/pdf/{id}` when `openAccessPdf` is missing.
- Enforces 1 second request spacing and retries on `429` with exponential backoff + `Retry-After` support.

### arXiv client (`internal/arxiv/client.go`)
- Still available for compatibility and tests, but not wired into `dig` by default.

### RSS client (`internal/rss/client.go`)
- Uses `https://rss.arxiv.org/rss/{category}` for each configured category.
- Parses RSS item fields (`title`, `link`, `description`, `dc:creator`, `pubDate`, `category`, `guid`).
- Extracts IDs from GUID values formatted as `oai:arXiv.org:{id}` and constructs PDF URL as `https://arxiv.org/pdf/{id}`.
- Merges category feeds, deduplicates by arXiv ID, trims to `limit`, and enforces 1-second spacing between category fetches.
- Sets `User-Agent: fieldwork/1.0 (https://github.com/meistro57/fieldwork)`.

### Harvester (`internal/harvester/harvester.go`)
- Fetches latest papers via a `PaperSource` interface (`FetchLatest`) so different providers can be injected.
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
- Uses arXiv RSS feeds as the default paper source.
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
- Harvest requires live network access to arXiv RSS feeds (default) unless tests/mocks are used.
- arXiv IDs include version suffixes (example: `2401.12345v1`), and that exact ID is used for PDF filename and Redis key segment.
- `force` flag is parsed by CLI but not yet enforced in harvester/full-pipeline logic.
- `dig --phase full` currently depends on marker service availability; no local fallback in Go client path.
- If Marker runs in a container with a different mount path, set `MARKER_PDF_ROOT` so parse requests use container-visible PDF paths.
- Marker Python dependency may not be present in host env unless installed from `python/marker_service/requirements.txt`; service still returns fallback parse payload when Marker import fails.

## Next Build Order
Follow roadmap sequence:
1. Complete M2 internals (real Marker conversion output schema persistence)
2. M3 Gemini embedding layer + chunking
3. M4 Qdrant ingestion + pipeline orchestration
4. M5 full resumable dig behavior (`force`, queue workers, progress)
