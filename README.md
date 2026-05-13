# FIELDWORK

<img width="751" height="751" alt="image" src="https://github.com/user-attachments/assets/0d15acdb-e542-4470-b842-afb4e79f9255" />

FIELDWORK is a Go CLI and Python Marker sidecar for harvesting and parsing arXiv physics papers.

## Current Milestone Status
- Milestone 0 scaffold: complete
- Milestone 1 harvest phase: implemented (`RSS -> PDF cache -> Redis status/meta`)
- Milestone 2 parsing foundation: in progress (`marker_service /parse` + Go marker client + `dig --phase full` parse step)
- Milestones 3+: not implemented yet

See `ROADMAP.md` for milestone tracker and acceptance criteria.

## Prerequisites
- Go toolchain version compatible with `go.mod`
- Python 3.11+
- Docker + Docker Compose
- Network access to `rss.arxiv.org`

## Quickstart
```bash
cp .env.example .env
make build
make service
make status
```

## Main Commands
```bash
./bin/fieldwork dig --phase harvest --limit 5
./bin/fieldwork dig --phase full --limit 5
./bin/fieldwork status
./bin/fieldwork query "non-adiabatic transitions"
```

## Make Targets
- `make build`
- `make service`
- `make dig ARGS="--phase harvest --limit 5"`
- `make dig ARGS="--phase full --limit 5"`
- `make query QUERY="text"`
- `make status`
- `make clean`

## Current CLI Behavior
- `fieldwork dig`
  - `--phase harvest`: runs RSS harvest only.
  - `--phase full`: runs harvest, then parses all papers in Redis status `downloaded`.
  - `--category`: overrides configured category list for a run.
  - `--limit`: overrides configured paper limit.
  - `--force`: accepted by CLI, not yet behaviorally implemented.
- `fieldwork query <text>`: milestone 6 stub; prints selected flags.
- `fieldwork status`: prints env summary + service checks for Marker, Qdrant, Redis.
  - exits non-zero if any dependency is unhealthy.

## Paper Sources
- Default pipeline source: arXiv RSS feeds via `internal/rss`.
- `internal/semanticscholar` client exists for Graph API ingestion experiments but is not the default `dig` source.
- `internal/arxiv` Atom client remains for compatibility/tests.

## Marker Service
The FastAPI sidecar exposes:
- `GET /health`
- `POST /parse` with body: `{ "pdf_path": "/data/pdfs/file.pdf" }`

`/parse` behavior currently:
- validates that `pdf_path` exists
- attempts `convert_single_pdf` from Marker when available
- falls back to empty markdown payload when Marker module import fails
- returns normalized fields: `markdown`, `title`, `sections`, `charts`, `latex_blocks`

Python dependency note:
- `marker-pdf==1.10.0` requires `Pillow>=10.1.0,<11.0.0`; repo pins `Pillow==10.4.0`.

## Harvest + Parse Notes
- RSS fetch uses `https://rss.arxiv.org/rss/{category}` per category.
- Items are merged across categories, deduplicated by arXiv ID, and trimmed to `--limit`.
- PDFs are cached in `PDF_CACHE_DIR` (default `./data/pdfs`).
- Existing PDFs are skipped; metadata and status are still synced to Redis.
- Full phase parse sends paths using `MARKER_PDF_ROOT` when set; otherwise `PDF_CACHE_DIR`.
- Redis keys currently used:
  - `fieldwork:paper:{arxiv_id}:status`
  - `fieldwork:paper:{arxiv_id}:meta`
  - `fieldwork:dig:current`
  - `fieldwork:dig:stats`

## Testing
```bash
go test ./...
python3 -m compileall python/marker_service
```

## CI
GitHub Actions workflow: `.github/workflows/ci.yml`
- Runs `go test ./...` on push to `main` and pull requests.
- Runs `python -m compileall python/marker_service` for marker service syntax validation.

## Environment Variables
Use `.env.example` as the base runtime config.