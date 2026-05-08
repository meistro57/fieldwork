from pathlib import Path

from fastapi import FastAPI

from models import HealthResponse, ParseRequest, ParseResponse

app = FastAPI(title="FIELDWORK Marker Service", version="0.1.0")


@app.get("/health", response_model=HealthResponse)
def health() -> HealthResponse:
    return HealthResponse(status="ok", marker_version="stub")


@app.post("/parse", response_model=ParseResponse)
def parse_pdf(req: ParseRequest) -> ParseResponse:
    pdf_name = Path(req.pdf_path).stem
    return ParseResponse(
        markdown="",
        title=pdf_name,
        sections=[],
        charts=[],
        latex_blocks=[],
    )
