import os
import re
import time
import logging
from typing import List, Optional
from concurrent.futures import ThreadPoolExecutor

import uvicorn
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from rapidocr_onnxruntime import RapidOCR

try:
    import wordninja
except ImportError:
    wordninja = None

NUM_WORKERS = 4
print(f"[OCR] Initialising {NUM_WORKERS} High-Speed Parallel RapidOCR Engines...")
_engines = [
    RapidOCR(det_limit_side_len=320, det_limit_type='max', det_use_dml=False, cls_use_dml=False, rec_use_dml=False)
    for _ in range(NUM_WORKERS)
]
_pool = ThreadPoolExecutor(max_workers=NUM_WORKERS, thread_name_prefix="ocr_worker")
print(f"[OCR] {NUM_WORKERS} Parallel Engines Ready (35+ FPS on CPU).")

app = FastAPI(title="Comicify RapidOCR Sidecar API", version="5.0")
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
    return {
        "status": "online",
        "engine": f"RapidOCR {NUM_WORKERS}x High-Speed Engine"
    }

def _clean_english_text(text: str) -> str:
    if not text:
        return text
    text = re.sub(r'([a-zA-Z]),([a-zA-Z])', r'\1, \2', text)
    text = re.sub(r'([a-zA-Z])\.([a-zA-Z])', r'\1. \2', text)
    text = re.sub(r'([a-zA-Z])!([a-zA-Z])', r'\1! \2', text)
    text = re.sub(r'([a-zA-Z])\?([a-zA-Z])', r'\1? \2', text)
    text = re.sub(r'([a-zA-Z]):([a-zA-Z])', r'\1: \2', text)
    text = re.sub(r'([a-zA-Z]);([a-zA-Z])', r'\1; \2', text)
    return text

def _process_single(args) -> FullFrameResult:
    idx, path, is_english = args
    if not os.path.exists(path):
        return FullFrameResult(path=path, success=False, items=[], error="File not found")
    try:
        engine = _engines[idx % NUM_WORKERS]
        ocr_res, _ = engine(path)
        items = []
        score_thresh = 0.35 if is_english else 0.40
        if ocr_res:
            for box, text, score in ocr_res:
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
        return FullFrameResult(path=path, success=True, items=items)
    except Exception as e:
        return FullFrameResult(path=path, success=False, items=[], error=str(e))

def _process_batch(paths: List[str], lang: str = "chi_sim") -> List[FullFrameResult]:
    t0 = time.time()
    is_english = (lang == "en" or lang == "english")
    tasks = [(i, p, is_english) for i, p in enumerate(paths)]
    
    results = list(_pool.map(_process_single, tasks))
    
    t1 = time.time()
    elapsed = t1 - t0
    fps = len(paths) / elapsed if elapsed > 0 else 0
    total_found = sum(len(r.items) for r in results if r.success)
    print(f"[OCR] Processed batch {len(paths)} frames in {elapsed:.2f}s ({fps:.1f} FPS) -> Found {total_found} texts.")
    return results

@app.post("/ocr_full_frame_batch", response_model=FullFrameBatchResponse)
def ocr_full_frame_batch(req: FullFrameBatchRequest):
    results = _process_batch(req.paths, req.lang or "chi_sim")
    return FullFrameBatchResponse(results=results)

if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=8000, workers=1, access_log=False)
