import io
import os
import uvicorn
from fastapi import FastAPI, UploadFile, File, Form, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from typing import List, Optional
from PIL import Image
from rapidocr_onnxruntime import RapidOCR

app = FastAPI(title="Comicify RapidOCR Sidecar API", version="2.0")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

engine = RapidOCR()

@app.get("/health")
def health():
    return {"status": "online", "engine": "RapidOCR DirectML/ONNX"}

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

@app.post("/ocr_full_frame_batch", response_model=FullFrameBatchResponse)
def ocr_full_frame_batch(req: FullFrameBatchRequest):
    results = []
    for path in req.paths:
        if not os.path.exists(path):
            results.append(FullFrameResult(path=path, success=False, items=[], error="File not found"))
            continue
        try:
            ocr_res, _ = engine(path)
            items = []
            if ocr_res:
                for box, text, score in ocr_res:
                    if score >= 0.40 and text.strip():
                        xs = [p[0] for p in box]
                        ys = [p[1] for p in box]
                        min_x, max_x = int(min(xs)), int(max(xs))
                        min_y, max_y = int(min(ys)), int(max(ys))
                        items.append(OCRItem(
                            text=text.strip(),
                            score=float(score),
                            bbox=[min_x, min_y, max_x - min_x, max_y - min_y]
                        ))
            results.append(FullFrameResult(path=path, success=True, items=items))
        except Exception as e:
            results.append(FullFrameResult(path=path, success=False, items=[], error=str(e)))
    return FullFrameBatchResponse(results=results)

if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=8000)
