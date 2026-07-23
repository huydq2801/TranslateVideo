import ctypes
import gc
import io
import os
import sys
import re
from fastapi import FastAPI, File, UploadFile, HTTPException
from fastapi.middleware.cors import CORSMiddleware
import uvicorn
from PIL import Image, ImageOps
import numpy as np

def release_system_ram():
    gc.collect()
    try:
        import torch
        if torch.cuda.is_available():
            torch.cuda.empty_cache()
    except Exception:
        pass

    if sys.platform == 'win32':
        try:
            ctypes.windll.psapi.EmptyWorkingSet(-1)
        except Exception:
            try:
                handle = ctypes.windll.kernel32.GetCurrentProcess()
                ctypes.windll.kernel32.SetProcessWorkingSetSize(handle, -1, -1)
            except Exception:
                pass

# Configure NVIDIA CUDA DLL paths on Windows startup if installed via pip
if sys.platform == 'win32':
    try:
        import site
        added_paths = []
        for site_dir in site.getsitepackages():
            nvidia_dir = os.path.join(site_dir, 'nvidia')
            if os.path.isdir(nvidia_dir):
                for root, dirs, files in os.walk(nvidia_dir):
                    if 'bin' in dirs:
                        bin_path = os.path.abspath(os.path.join(root, 'bin'))
                        added_paths.append(bin_path)
                        try:
                            os.add_dll_directory(bin_path)
                        except Exception:
                            pass
        if added_paths:
            os.environ['PATH'] = ';'.join(added_paths) + ';' + os.environ['PATH']
            print(f"[OCR Sidecar] Configured {len(added_paths)} pip CUDA DLL directories on Windows.")
    except Exception as e:
        print(f"[OCR Sidecar] Warning: failed to configure pip CUDA DLL paths: {e}")

app = FastAPI(title="Comicify OCR Sidecar")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Engine initialization (EasyOCR / RapidOCR / PaddleOCR / pytesseract / manga-ocr)
ocr_engine = None
engine_name = "None"

def init_ocr():
    global ocr_engine, engine_name

    # Check if GPU is actually available for PyTorch
    has_cuda = False
    try:
        import torch
        has_cuda = torch.cuda.is_available()
    except:
        pass

    # 1. Try EasyOCR with GPU if CUDA is active
    if has_cuda:
        try:
            import easyocr
            print("[OCR Sidecar] Initializing EasyOCR for Chinese (ch_sim) with GPU...")
            ocr_engine = easyocr.Reader(['ch_sim', 'en'], gpu=True)
            engine_name = "EasyOCR (GPU)"
            print("[OCR Sidecar] EasyOCR GPU initialized successfully.")
            return
        except Exception as e:
            print(f"[OCR Sidecar] EasyOCR GPU initialization failed: {e}")

    # 2. Try RapidOCR (PP-OCRv4 via ONNX Runtime) - Extremely fast (~50-100ms) even on CPU, and blazingly fast on GPU!
    try:
        import onnxruntime
        from rapidocr_onnxruntime import RapidOCR, utils as rapidocr_utils

        # Patch rapidocr_onnxruntime OrtInferSession to enable DmlExecutionProvider
        orig_ort_init = rapidocr_utils.OrtInferSession.__init__
        def patched_ort_init(self, config):
            sess_opt = onnxruntime.SessionOptions()
            sess_opt.log_severity_level = 4
            sess_opt.enable_cpu_mem_arena = False
            sess_opt.graph_optimization_level = onnxruntime.GraphOptimizationLevel.ORT_ENABLE_ALL

            providers = onnxruntime.get_available_providers()
            ep_list = []
            if 'DmlExecutionProvider' in providers:
                ep_list.append('DmlExecutionProvider')
            elif config.get('use_cuda') and onnxruntime.get_device() == 'GPU' and 'CUDAExecutionProvider' in providers:
                ep_list.append('CUDAExecutionProvider')
            ep_list.append('CPUExecutionProvider')

            self._verify_model(config['model_path'])
            self.session = onnxruntime.InferenceSession(
                config['model_path'],
                sess_options=sess_opt,
                providers=ep_list
            )
        rapidocr_utils.OrtInferSession.__init__ = patched_ort_init

        providers = onnxruntime.get_available_providers()
        has_dml = "DmlExecutionProvider" in providers
        has_ort_cuda = "CUDAExecutionProvider" in providers

        if has_dml:
            print("[OCR Sidecar] Initializing RapidOCR (PP-OCRv4) with DirectML GPU Acceleration (use_angle_cls=False)...")
            ocr_engine = RapidOCR(use_angle_cls=False)
            engine_name = "RapidOCR (DirectML GPU)"
        elif has_ort_cuda:
            print("[OCR Sidecar] Initializing RapidOCR (PP-OCRv4) with GPU (CUDA)...")
            ocr_engine = RapidOCR(
                use_angle_cls=False,
                det_use_cuda=True, det_model_path=None,
                rec_use_cuda=True, rec_model_path=None,
                cls_use_cuda=False, cls_model_path=None
            )
            engine_name = "RapidOCR (GPU)"
        else:
            print("[OCR Sidecar] Initializing RapidOCR (PP-OCRv4) on CPU...")
            ocr_engine = RapidOCR(use_angle_cls=False)
            engine_name = "RapidOCR (PP-OCRv4)"

        print(f"[OCR Sidecar] {engine_name} initialized successfully.")
        return
    except Exception as e:
        print(f"[OCR Sidecar] RapidOCR unavailable ({e}). Trying EasyOCR (CPU)...")

    # 3. Fallback to EasyOCR (CPU) - slow but functional
    try:
        import easyocr
        print("[OCR Sidecar] Initializing EasyOCR for Chinese (ch_sim) in CPU mode...")
        ocr_engine = easyocr.Reader(['ch_sim', 'en'], gpu=False)
        engine_name = "EasyOCR (CPU)"
        print("[OCR Sidecar] EasyOCR CPU initialized successfully.")
        return
    except Exception as e:
        print(f"[OCR Sidecar] EasyOCR CPU failed ({e}). Trying PaddleOCR...")

    # 3. Try PaddleOCR
    try:
        from paddleocr import PaddleOCR
        print("[OCR Sidecar] Initializing PaddleOCR...")
        ocr_engine = PaddleOCR(use_angle_cls=True, lang='ch', show_log=False)
        engine_name = "PaddleOCR"
        print("[OCR Sidecar] PaddleOCR initialized successfully.")
        return
    except Exception as e:
        print(f"[OCR Sidecar] PaddleOCR unavailable ({e}). Trying PyTesseract...")

    # 4. Fallback: PyTesseract
    try:
        import pytesseract
        engine_name = "PyTesseract"
        ocr_engine = pytesseract
        print("[OCR Sidecar] PyTesseract initialized as fallback.")
        return
    except Exception as e:
        print(f"[OCR Sidecar] PyTesseract unavailable ({e}). Trying manga-ocr...")

    # 5. Fallback: MangaOCR
    try:
        from manga_ocr import MangaOcr
        print("[OCR Sidecar] Initializing MangaOcr...")
        ocr_engine = MangaOcr()
        engine_name = "MangaOCR"
        print("[OCR Sidecar] MangaOCR initialized successfully.")
        return
    except Exception as e:
        print(f"[OCR Sidecar] MangaOCR unavailable ({e}). No OCR engine loaded.")
        engine_name = "Offline"

def preprocess_image(image: Image.Image) -> Image.Image:
    """Add border padding matching corner color and scale up small crops for optimal CJK OCR recognition."""
    img = image.convert("RGB")
    w, h = img.size

    # Upscale small cropped text regions
    if w < 160 or h < 100:
        scale = max(2, int(320 / max(w, h)))
        img = img.resize((w * scale, h * scale), Image.Resampling.BICUBIC)

    # Sample top-left corner pixel to match background color instead of hardcoded white
    bg_color = img.getpixel((0, 0))

    # Add 20px border padding so text is never cut off at edge boundaries
    pad = 20
    nw, nh = img.size[0] + pad * 2, img.size[1] + pad * 2
    padded = Image.new("RGB", (nw, nh), bg_color)
    padded.paste(img, (pad, pad))
    return padded

def contains_cjk(text: str) -> bool:
    return bool(re.search(r'[\u4e00-\u9fff\u3400-\u4dbf\u3040-\u30ff]', text))

def run_ocr_on_image(image: Image.Image, lang: str = "chi_sim") -> str:
    img_np = np.array(image)
    lines = []

    if engine_name.startswith("RapidOCR"):
        result, _ = ocr_engine(img_np)
        if result:
            lines = [line[1] for line in result if line[1] and float(line[2]) > 0.35]

    elif engine_name.startswith("EasyOCR"):
        res_detail = ocr_engine.readtext(img_np, detail=1)
        if res_detail:
            lines = [item[1] for item in res_detail if item[1] and float(item[2]) > 0.35]

    elif engine_name.startswith("PaddleOCR"):
        res = ocr_engine.ocr(img_np, cls=True)
        if res and res[0]:
            lines = [line[1][0] for line in res[0] if line[1] and float(line[1][1]) > 0.35]

    elif engine_name == "PyTesseract":
        import pytesseract
        tess_lang = "chi_sim" if lang == "chi_sim" else "eng"
        return pytesseract.image_to_string(image, lang=tess_lang, config="--psm 6").strip()

    elif engine_name == "MangaOCR":
        return ocr_engine(image).strip()

    if lines:
        return "".join(lines) if any(contains_cjk(l) for l in lines) else " ".join(lines)
    return ""

@app.get("/health")
def health():
    if ocr_engine is None:
        init_ocr()
    return {
        "status": "online" if engine_name != "Offline" else "degraded",
        "engine": engine_name
    }

@app.post("/ocr")
def process_ocr(file: UploadFile = File(...), lang: str = "chi_sim"):
    if ocr_engine is None:
        init_ocr()

    if ocr_engine is None and engine_name == "Offline":
        raise HTTPException(status_code=503, detail="No OCR engine available on sidecar")

    raw_image = None
    image = None
    try:
        contents = file.file.read()
        raw_image = Image.open(io.BytesIO(contents))
        image = preprocess_image(raw_image)

        text_result = run_ocr_on_image(image, lang=lang)

        return {
            "success": True,
            "text": text_result.strip(),
            "engine": engine_name
        }

    except Exception as err:
        return {"success": False, "error": str(err), "text": ""}
    finally:
        if raw_image:
            raw_image.close()
        if image and image is not raw_image:
            image.close()
        try:
            file.file.close()
        except:
            pass

batch_request_count = 0

@app.post("/ocr_batch")
def process_ocr_batch(files: list[UploadFile] = File(...), lang: str = "chi_sim"):
    global batch_request_count
    batch_request_count += 1
    if ocr_engine is None:
        init_ocr()

    if ocr_engine is None and engine_name == "Offline":
        raise HTTPException(status_code=503, detail="No OCR engine available on sidecar")

    results = []
    for file in files:
        raw_image = None
        image = None
        try:
            contents = file.file.read()
            raw_image = Image.open(io.BytesIO(contents))
            image = preprocess_image(raw_image)

            text_result = run_ocr_on_image(image, lang=lang)

            results.append({
                "success": True,
                "text": text_result.strip(),
                "engine": engine_name
            })

        except Exception as err:
            results.append({
                "success": False,
                "error": str(err),
                "text": ""
            })
        finally:
            if raw_image:
                try: raw_image.close()
                except: pass
            if image and image is not raw_image:
                try: image.close()
                except: pass
            try:
                file.file.close()
            except:
                pass
            del raw_image, image

    release_system_ram()
    return {"results": results}


from pydantic import BaseModel
from typing import List

class FrameBatchRequest(BaseModel):
    paths: List[str]
    lang: str = "chi_sim"

from concurrent.futures import ThreadPoolExecutor

def load_and_preprocess_full_frame(path: str):
    if not os.path.exists(path):
        return path, None, "file not found"
    try:
        with Image.open(path) as raw_img:
            image = raw_img.convert("RGB")
            w, h = image.size
            max_dim = max(w, h)
            scale = 1.0
            if max_dim > 720:
                scale = 720.0 / float(max_dim)
                target_w = max(1, int(w * scale))
                target_h = max(1, int(h * scale))
                detect_img = image.resize((target_w, target_h), Image.Resampling.BILINEAR)
            else:
                detect_img = image
            img_np = np.array(detect_img)
            return path, (img_np, scale), None
    except Exception as e:
        return path, None, str(e)

@app.post("/ocr_full_frame_batch")
def ocr_full_frame_batch(req: FrameBatchRequest):
    with ThreadPoolExecutor(max_workers=min(16, (os.cpu_count() or 4) * 2)) as executor:
        loaded_frames = list(executor.map(load_and_preprocess_full_frame, req.paths))

    batch_results = []
    for path, data, err in loaded_frames:
        if err or data is None:
            batch_results.append({"path": path, "success": False, "items": [], "error": err or "load failed"})
            continue
        try:
            img_np, scale = data
            inv_scale = 1.0 / scale
            items = []
            if engine_name.startswith("RapidOCR"):
                result, _ = ocr_engine(img_np)
                if result:
                    for line in result:
                        if line[1] and float(line[2]) > 0.35:
                            pts = line[0]
                            xs = [p[0] * inv_scale for p in pts]
                            ys = [p[1] * inv_scale for p in pts]
                            min_x, max_x = min(xs), max(xs)
                            min_y, max_y = min(ys), max(ys)
                            items.append({
                                "text": line[1].strip(),
                                "score": float(line[2]),
                                "bbox": [int(min_x), int(min_y), int(max_x - min_x), int(max_y - min_y)]
                            })
            elif engine_name.startswith("EasyOCR"):
                res = ocr_engine.readtext(img_np, detail=1)
                if res:
                    for item in res:
                        if item[1] and float(item[2]) > 0.35:
                            pts = item[0]
                            xs = [p[0] * inv_scale for p in pts]
                            ys = [p[1] * inv_scale for p in pts]
                            min_x, max_x = min(xs), max(xs)
                            min_y, max_y = min(ys), max(ys)
                            items.append({
                                "text": item[1].strip(),
                                "score": float(item[2]),
                                "bbox": [int(min_x), int(min_y), int(max_x - min_x), int(max_y - min_y)]
                            })
            batch_results.append({"path": path, "success": True, "items": items, "error": ""})
        except Exception as e:
            batch_results.append({"path": path, "success": False, "items": [], "error": str(e)})

    return {"results": batch_results}

@app.api_route("/clear_memory", methods=["GET", "POST"])
def clear_memory():
    release_system_ram()
    return {"status": "cleared", "message": "Physical RAM and VRAM pages released back to Windows OS"}


if __name__ == "__main__":
    port = int(os.environ.get("PORT", 8000))
    print(f"[OCR Sidecar] Starting HTTP server on http://127.0.0.1:{port}...")
    uvicorn.run(app, host="127.0.0.1", port=port)

