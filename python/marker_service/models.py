from pydantic import BaseModel, Field


class ParseRequest(BaseModel):
    pdf_path: str = Field(..., min_length=1)


class ChartResult(BaseModel):
    index: int
    image_b64: str
    context_before: str
    context_after: str
    caption: str


class ParseResponse(BaseModel):
    markdown: str
    title: str
    sections: list[str]
    charts: list[ChartResult]
    latex_blocks: list[str]


class HealthResponse(BaseModel):
    status: str
    marker_version: str
