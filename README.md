# FIELDWORK

FIELDWORK is a Go CLI and Python Marker sidecar for ingesting and querying arXiv physics papers.

## Prerequisites
- Go 1.22+
- Python 3.11+
- Docker + Docker Compose

## Quickstart
1. Copy environment configuration.
2. Build the CLI.
3. Start local services.
4. Check service health.

```bash
cp .env.example .env
make build
make service
make status
```

## Environment Variables
Use `.env.example` as the source of required runtime settings.

## Make Targets
- `make build`
- `make service`
- `make dig`
- `make query QUERY="text"`
- `make status`
- `make clean`

## CLI
```bash
./bin/fieldwork dig --phase harvest
./bin/fieldwork query "non-adiabatic transitions"
./bin/fieldwork status
```

## Marker Service
The FastAPI sidecar exposes:
- `GET /health`
- `POST /parse` with body: `{ "pdf_path": "/data/pdfs/file.pdf" }`
