import base64
import re
from importlib import metadata as importlib_metadata
from pathlib import Path
from typing import Any

from fastapi import FastAPI, HTTPException

from models import ChartResult, HealthResponse, ParseRequest, ParseResponse

app = FastAPI(title="FIELDWORK Marker Service", version="0.2.0")


@app.get("/health", response_model=HealthResponse)
def health() -> HealthResponse:
    return HealthResponse(status="ok", marker_version=get_marker_version())


@app.post("/parse", response_model=ParseResponse)
def parse_pdf(req: ParseRequest) -> ParseResponse:
    pdf_path = Path(req.pdf_path).expanduser()
    if not pdf_path.exists():
        raise HTTPException(status_code=404, detail="pdf_path not found")
    if not pdf_path.is_file():
        raise HTTPException(status_code=400, detail="pdf_path must be a file")

    markdown, metadata = convert_with_marker_or_fallback(str(pdf_path))
    title = extract_title(metadata, markdown, pdf_path.stem)
    sections = extract_sections(markdown)
    latex_blocks = extract_latex_blocks(markdown)
    charts = extract_charts(metadata, markdown)

    return ParseResponse(
        markdown=markdown,
        title=title,
        sections=sections,
        charts=charts,
        latex_blocks=latex_blocks,
    )


def convert_with_marker_or_fallback(pdf_path: str) -> tuple[str, dict[str, Any]]:
    converter = load_convert_single_pdf()
    if converter is None:
        return "", {}

    converted = converter(pdf_path)
    if isinstance(converted, tuple):
        if len(converted) >= 2:
            markdown = to_markdown(converted[0])
            metadata = to_metadata(converted[1])
            return markdown, metadata
        if len(converted) == 1:
            return to_markdown(converted[0]), {}

    if isinstance(converted, dict):
        markdown = to_markdown(
            converted.get("markdown")
            or converted.get("md")
            or converted.get("text")
            or ""
        )
        return markdown, to_metadata(converted)

    return to_markdown(converted), {}


def load_convert_single_pdf():
    candidates = [
        ("marker.convert", "convert_single_pdf"),
        ("marker.pdf", "convert_single_pdf"),
        ("marker.converters.pdf", "convert_single_pdf"),
    ]

    for module_name, symbol in candidates:
        try:
            module = __import__(module_name, fromlist=[symbol])
            converter = getattr(module, symbol)
            return converter
        except Exception:
            continue

    return None


def to_markdown(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    return str(value)


def to_metadata(value: Any) -> dict[str, Any]:
    if isinstance(value, dict):
        return value
    return {}


def extract_title(metadata: dict[str, Any], markdown: str, fallback: str) -> str:
    title = metadata.get("title")
    if isinstance(title, str) and title.strip():
        return title.strip()

    for line in markdown.splitlines():
        candidate = line.strip()
        if candidate.startswith("#"):
            return candidate.lstrip("#").strip() or fallback
        if candidate:
            return candidate[:200]

    return fallback


def extract_sections(markdown: str) -> list[str]:
    sections: list[str] = []
    seen: set[str] = set()

    for line in markdown.splitlines():
        candidate = line.strip()
        if not candidate.startswith("#"):
            continue
        normalized = candidate.lstrip("#").strip()
        if not normalized:
            continue
        if normalized in seen:
            continue
        seen.add(normalized)
        sections.append(normalized)

    return sections


def extract_latex_blocks(markdown: str) -> list[str]:
    blocks: list[str] = []

    for match in re.finditer(r"\$\$(.*?)\$\$", markdown, flags=re.DOTALL):
        snippet = match.group(1).strip()
        if snippet:
            blocks.append(snippet)

    for match in re.finditer(r"\\begin\{[^}]+\}.*?\\end\{[^}]+\}", markdown, flags=re.DOTALL):
        snippet = match.group(0).strip()
        if snippet:
            blocks.append(snippet)

    return blocks


def extract_charts(metadata: dict[str, Any], markdown: str) -> list[ChartResult]:
    source = metadata.get("charts")
    if not isinstance(source, list):
        source = metadata.get("figures")
    if not isinstance(source, list):
        source = metadata.get("images")
    if not isinstance(source, list):
        return []

    output: list[ChartResult] = []
    for index, item in enumerate(source):
        if not isinstance(item, dict):
            continue

        caption = pick_string(item, ["caption", "title", "description"]) or ""
        image_b64 = pick_string(item, ["image_b64", "image", "base64", "png_b64"]) or ""
        image_b64 = sanitize_base64(image_b64)
        if not image_b64:
            continue

        context_before, context_after = extract_context(markdown, caption)
        output.append(
            ChartResult(
                index=index,
                image_b64=image_b64,
                context_before=context_before,
                context_after=context_after,
                caption=caption,
            )
        )

    return output


def pick_string(data: dict[str, Any], keys: list[str]) -> str | None:
    for key in keys:
        value = data.get(key)
        if isinstance(value, str) and value.strip():
            return value.strip()
    return None


def sanitize_base64(raw: str) -> str:
    if not raw:
        return ""

    cleaned = raw.strip()
    if "," in cleaned and cleaned.startswith("data:"):
        cleaned = cleaned.split(",", 1)[1].strip()

    try:
        base64.b64decode(cleaned, validate=True)
    except Exception:
        return ""

    return cleaned


def extract_context(markdown: str, caption: str, window: int = 200) -> tuple[str, str]:
    if not markdown:
        return "", ""

    if caption:
        index = markdown.find(caption)
        if index >= 0:
            before_start = max(0, index - window)
            after_start = index + len(caption)
            after_end = min(len(markdown), after_start + window)
            return markdown[before_start:index], markdown[after_start:after_end]

    snippet = markdown[: window * 2]
    return snippet[:window], snippet[window:]


def get_marker_version() -> str:
    try:
        return importlib_metadata.version("marker-pdf")
    except Exception:
        return "unknown"
