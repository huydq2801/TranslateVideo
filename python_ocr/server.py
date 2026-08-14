import os

os.environ["OMP_NUM_THREADS"] = "4"
os.environ["MKL_NUM_THREADS"] = "4"
os.environ["OPENBLAS_NUM_THREADS"] = "4"
os.environ["VECLIB_MAXIMUM_THREADS"] = "4"
os.environ["NUMEXPR_NUM_THREADS"] = "4"

import logging
import uvicorn

logging.getLogger("asyncio").setLevel(logging.CRITICAL)

class WinErrorFilter(logging.Filter):
    def filter(self, record):
        msg = record.getMessage()
        return "10054" not in msg and "WinError 64" not in msg

logging.getLogger("uvicorn.error").addFilter(WinErrorFilter())
logging.getLogger("uvicorn.access").addFilter(WinErrorFilter())

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from typing import List, Optional
from concurrent.futures import ThreadPoolExecutor
import asyncio
import threading
import gc

from rapidocr_onnxruntime import RapidOCR

# Single engine with a dedicated thread — avoids any multi-instance DirectML issues.
# A dedicated thread means uvicorn's event loop stays free to accept new connections
# while OCR runs, so health-checks and UI remain responsive during scanning.
print("[OCR] Initialising DirectML engine...")
try:
    _engine = RapidOCR(det_use_dml=True, cls_use_dml=True, rec_use_dml=True)
except Exception:
    _engine = RapidOCR()

_engine_lock = threading.Lock()
# One dedicated background thread so OCR never blocks the asyncio event loop
_executor = ThreadPoolExecutor(max_workers=1, thread_name_prefix="ocr_worker")
print("[OCR] Engine ready.")

app = FastAPI(title="Comicify RapidOCR Sidecar API", version="3.2")
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

class FullFrameBatchRequest(BaseModel):
    paths: List[str]
    lang: Optional[str] = "chi_sim"

class OCRItem(BaseModel):
    text: str
    score: float
    bbox: List[int]

class FullFrameResult(BaseModel):
    path: str
    success: bool
    items: List[OCRItem]
    error: Optional[str] = None

class FullFrameBatchResponse(BaseModel):
    results: List[FullFrameResult]

@app.get("/health")
def health():
    return {"status": "online", "engine": "RapidOCR DirectML/ONNX"}

import re
try:
    import wordninja
except ImportError:
    wordninja = None

def _clean_english_text(text: str) -> str:
    """Normalize English OCR text: add space after punctuation, split joined words."""
    if not text:
        return text
    # 1. Punctuation spacing
    text = re.sub(r'([a-zA-Z]),([a-zA-Z])', r'\1, \2', text)
    text = re.sub(r'([a-zA-Z])\.([a-zA-Z])', r'\1. \2', text)
    text = re.sub(r'([a-zA-Z])!([a-zA-Z])', r'\1! \2', text)
    text = re.sub(r'([a-zA-Z])\?([a-zA-Z])', r'\1? \2', text)
    text = re.sub(r'([a-zA-Z]):([a-zA-Z])', r'\1: \2', text)
    text = re.sub(r'([a-zA-Z]);([a-zA-Z])', r'\1; \2', text)

    # 2. Word splitting for joined words if wordninja is available
    if wordninja is not None:
        tokens = text.split(' ')
        cleaned_tokens = []
        for tok in tokens:
            m = re.match(r'^([a-zA-Z]+)([^a-zA-Z]*)$', tok)
            if m:
                word, punct = m.group(1), m.group(2)
                if len(word) >= 6:
                    splits = wordninja.split(word)
                    if len(splits) > 1 and all(len(s) >= 2 for s in splits):
                        # Preserve uppercase if first char was uppercase
                        if word[0].isupper() and len(splits) > 0:
                            splits[0] = splits[0].capitalize()
                        cleaned_tokens.append(' '.join(splits) + punct)
                        continue
            cleaned_tokens.append(tok)
        text = ' '.join(cleaned_tokens)
    return text

def _process_batch(paths: List[str], lang: str = "chi_sim") -> List[FullFrameResult]:
    """Run OCR on a list of image paths sequentially in the worker thread."""
    results = []
    is_english = (lang == "en" or lang == "english")

    for path in paths:
        if not os.path.exists(path):
            results.append(FullFrameResult(path=path, success=False, items=[], error="File not found"))
            continue
        try:
            with _engine_lock:
                ocr_res, _ = _engine(path)
            items = []
            if ocr_res:
                for box, text, score in ocr_res:
                    score_thresh = 0.35 if is_english else 0.40
                    if score >= score_thresh and text.strip():
                        cleaned_text = text.strip()
                        if is_english:
                            cleaned_text = _clean_english_text(cleaned_text)

                        xs = [p[0] for p in box]
                        ys = [p[1] for p in box]
                        min_x, max_x = int(min(xs)), int(max(xs))
                        min_y, max_y = int(min(ys)), int(max(ys))
                        items.append(OCRItem(
                            text=cleaned_text,
                            score=float(score),
                            bbox=[min_x, min_y, max_x - min_x, max_y - min_y]
                        ))
            results.append(FullFrameResult(path=path, success=True, items=items))
        except Exception as e:
            results.append(FullFrameResult(path=path, success=False, items=[], error=str(e)))
    gc.collect()
    return results

@app.post("/ocr_full_frame_batch", response_model=FullFrameBatchResponse)
async def ocr_full_frame_batch(req: FullFrameBatchRequest):
    # Run the blocking OCR work in the dedicated thread, freeing the event loop
    loop = asyncio.get_running_loop()
    results = await loop.run_in_executor(_executor, _process_batch, req.paths, req.lang or "chi_sim")
    return FullFrameBatchResponse(results=results)

if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=8000, workers=1)
