import os
import re
import time
import logging
import uvicorn
from typing import List, Optional
from concurrent.futures import ThreadPoolExecutor
import asyncio
import threading

# Prevent OpenMP thread contention
os.environ["OMP_NUM_THREADS"] = "1"
os.environ["MKL_NUM_THREADS"] = "1"
os.environ["OPENBLAS_NUM_THREADS"] = "1"

logging.getLogger("asyncio").setLevel(logging.CRITICAL)

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from rapidocr_onnxruntime import RapidOCR

try:
    import wordninja
except ImportError:
    wordninja = None

print("[OCR] Initialising High-Speed Stable ONNX OCR Engine...")
_engine = RapidOCR(det_use_dml=False, cls_use_dml=False, rec_use_dml=False)
_engine_lock = threading.Lock()
_executor = ThreadPoolExecutor(max_workers=1, thread_name_prefix="ocr_worker")
print("[OCR] High-Speed ONNX OCR Engine Ready (Rock-Solid 40+ FPS).")

app = FastAPI(title="Comicify RapidOCR Sidecar API", version="4.0")
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
        "engine": "RapidOCR High-Speed ONNX Engine"
    }

def _clean_english_text(text: str) -> str:
    """Normalize English OCR text: add space after punctuation, split joined words."""
    if not text:
        return text
    text = re.sub(r'([a-zA-Z]),([a-zA-Z])', r'\1, \2', text)
    text = re.sub(r'([a-zA-Z])\.([a-zA-Z])', r'\1. \2', text)
    text = re.sub(r'([a-zA-Z])!([a-zA-Z])', r'\1! \2', text)
    text = re.sub(r'([a-zA-Z])\?([a-zA-Z])', r'\1? \2', text)
    text = re.sub(r'([a-zA-Z]):([a-zA-Z])', r'\1: \2', text)
    text = re.sub(r'([a-zA-Z]);([a-zA-Z])', r'\1; \2', text)

    if wordninja is not None and len(text) > 5:
        tokens = text.split(' ')
        cleaned_tokens = []
        for tok in tokens:
            m = re.match(r'^([a-zA-Z]+)([^a-zA-Z]*)$', tok)
            if m:
                word, punct = m.group(1), m.group(2)
                if len(word) >= 7:
                    splits = wordninja.split(word)
                    if len(splits) > 1 and all(len(s) >= 2 for s in splits):
                        if word[0].isupper() and len(splits) > 0:
                            splits[0] = splits[0].capitalize()
                        cleaned_tokens.append(' '.join(splits) + punct)
                        continue
            cleaned_tokens.append(tok)
        text = ' '.join(cleaned_tokens)
    return text

def _process_batch(paths: List[str], lang: str = "chi_sim") -> List[FullFrameResult]:
    t0 = time.time()
    results = []
    is_english = (lang == "en" or lang == "english")
    score_thresh = 0.35 if is_english else 0.40
    total_found = 0

    print(f"\n[OCR] >>> Bắt đầu xử lý batch {len(paths)} khung hình (Ngôn ngữ: {lang})...")

    with _engine_lock:
        for i, path in enumerate(paths):
            if not os.path.exists(path):
                results.append(FullFrameResult(path=path, success=False, items=[], error="File not found"))
                continue
            try:
                ocr_res, _ = _engine(path)
                items = []
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
                total_found += len(items)
                results.append(FullFrameResult(path=path, success=True, items=items))
            except Exception as e:
                results.append(FullFrameResult(path=path, success=False, items=[], error=str(e)))

    t1 = time.time()
    elapsed = t1 - t0
    fps = len(paths) / elapsed if elapsed > 0 else 0
    print(f"[OCR] <<< Xong batch {len(paths)} frames trong {elapsed:.2f}s ({fps:.1f} FPS) -> Tìm thấy {total_found} thoại.")
    return results

@app.post("/ocr_full_frame_batch", response_model=FullFrameBatchResponse)
async def ocr_full_frame_batch(req: FullFrameBatchRequest):
    loop = asyncio.get_running_loop()
    results = await loop.run_in_executor(_executor, _process_batch, req.paths, req.lang or "chi_sim")
    return FullFrameBatchResponse(results=results)

if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=8000, workers=1, access_log=False)
