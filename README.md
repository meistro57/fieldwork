# FIELDWORK

<img width="751" height="751" alt="image" src="https://github.com/user-attachments/assets/0d15acdb-e542-4470-b842-afb4e79f9255" />


FIELDWORK is a Go CLI and Python Marker sidecar for ingesting and querying arXiv physics papers.

## Current Milestone Status
- Milestone 0 scaffold: complete
- Milestone 1 harvest phase: implemented (`arXiv -> PDF cache -> Redis status/meta`)
- Milestone 2 parsing foundation: in progress (`marker_service /parse` + Go marker client + `dig --phase full` parse step)
- Milestones 3+: not implemented yet

See `ROADMAP.md` for the live milestone tracker and acceptance criteria.

## Prerequisites
- Go 1.22+
- Python 3.11+
- Docker + Docker Compose
- Network access to `export.arxiv.org`

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

`dig --phase full` currently performs harvest plus marker parse status transitions (`downloaded -> parsed/failed`). Embedding/upsert stages are pending.

## Make Targets
- `make build`
- `make service`
- `make dig ARGS="--phase harvest --limit 5"`
- `make query QUERY="text"`
- `make status`
- `make clean`

## Testing
```bash
go test ./...
python3 -m compileall python/marker_service
```

## Marker Service
The FastAPI sidecar exposes:
- `GET /health`
- `POST /parse` with body: `{ "pdf_path": "/data/pdfs/file.pdf" }`

`/parse` behavior currently:
- validates file path existence
- attempts `convert_single_pdf` from Marker if installed
- falls back to empty markdown payload if Marker module is unavailable
- returns normalized fields: `markdown`, `title`, `sections`, `charts`, `latex_blocks`

## Harvest + Parse Implementation Notes
- arXiv feed query sorts by `submittedDate` descending and supports category OR queries.
- PDFs are stored in `PDF_CACHE_DIR` (default `./data/pdfs`).
- Existing PDFs are skipped; paper metadata and statuses are still synced to Redis.
- During full phase, papers in `downloaded` status are parsed via marker service client.
- Redis keys currently used:
  - `fieldwork:paper:{arxiv_id}:status`
  - `fieldwork:paper:{arxiv_id}:meta`
  - `fieldwork:dig:current`
  - `fieldwork:dig:stats`

## Environment Variables
Use `.env.example` as the source of required runtime settings.
