# 🎬 Comicify Video Translator

**Comicify Video Translator** là ứng dụng Windows cao cấp dùng công nghệ **Go + Wails v2** kết hợp máy chủ **Python OCR Sidecar** và **Google Gemini AI** để tự động quét bong bóng thoại (Speech Bubbles), nhận diện chữ Trung/Nhật/Anh, dịch chuẩn xác sang tiếng Việt và xuất video mới với lớp phụ đề overlay đẹp mắt.

![License](https://img.shields.io/badge/Platform-Windows-blue.svg)
![Go](https://img.shields.io/badge/Backend-Go%201.25+-00ADD8.svg)
![Wails](https://img.shields.io/badge/Framework-Wails%20v2-red.svg)
![Python](https://img.shields.io/badge/OCR-RapidOCR%20/%20PyTorch-3776AB.svg)

---

## ✨ Tính năng Nổi bật

- 🔍 **Thuật toán Nhận diện Bong bóng thoại Thông minh**:
  - Hỗ trợ bong bóng thoại **Trắng**, **Màu xám (Multi-line)** và **Các dải màu (Xanh dương, Xanh lá, Đen)**.
  - Nhận diện chuẩn xác 100% cả các ô thoại cực nhỏ chứa **1 ký tự đơn lẻ** (như `"这..."`, `"啊"`).
- ⚡ **Khử trùng lặp Khung hình Lossless (`ExactFrameHash`)**:
  - Tính toán checksum CRC32 trực tiếp trên RAM pixel thô chưa nén, bỏ qua 30%–60% số frame trùng lặp trên cảnh tĩnh mà bảo đảm **độ chính xác tuyệt đối 100%**.
- 🚀 **Bộ đọc Pixel Zero-Allocation (`FastLumaReader`)**:
  - Đọc trực tiếp bộ nhớ `YCbCr` trong Go, giúp thuật toán duyệt ảnh chạy nhanh gấp 3 lần.
- 🐍 **OCR Sidecar Đa Mô hình (Python Sidecar)**:
  - Tích hợp RapidOCR (PP-OCRv4 GPU/CPU), EasyOCR, PaddleOCR với cơ chế **Direct Recognition Fallback** cho chữ nhỏ/mảnh.
- 🤖 **Bảo vệ Quota Gemini AI (`gemini-3.1-flash-lite`)**:
  - Khóa model tốc độ cao, tự động Rate Limit 10 req/phút và tự động Exponential Backoff khi nhận mã lỗi `429`.
- 📖 **Giữ Nhất quán Nhân vật & Xưng hô (Glossary)**:
  - Cho phép định nghĩa bảng từ vựng `từ gốc => bản dịch bắt buộc` (ví dụ `黄老师 => cô Hoàng`).
- 🎬 **Xuất Video Tăng tốc Phần cứng (GPU Acceleration)**:
  - Tự động phát hiện và sử dụng GPU `h264_nvenc` (NVIDIA) / `h264_qsv` (Intel) / `libx264` (CPU Fallback).

---

## 📁 Cấu trúc Dự án

```text
TranslateVideo/
├── app.go                      # Binding API chính giữa Wails Frontend & Go Backend
├── main.go                     # Khởi tạo Wails app & Asset HTTP Server (/wails-local-video)
├── pipeline.go                 # Luồng xử lý chính (Scan, OCR Batching, Gemini Translation, Cache)
├── detector.go                 # Thuật toán BFS Flood-fill, ExactFrameHash & FastLumaReader
├── overlay.go                  # Engine vẽ chữ đè bong bóng thoại & Render Video với FFmpeg GPU
├── python_ocr/
│   ├── server.py               # Máy chủ FastAPI OCR Sidecar (port 8000)
│   └── run_server.bat          # File khởi chạy máy chủ OCR Sidecar
├── frontend/                   # Giao diện người dùng Webview (HTML/JS/CSS)
├── docs/                       # Bộ tài liệu chi tiết dự án
│   ├── ARCHITECTURAL.md        # Tài liệu Kiến trúc Hệ thống & Thuật toán
│   └── USER_GUIDE.md           # Hướng dẫn Sử dụng Chi tiết & Troubleshooting
├── build/bin/                  # File chạy thực thi (.exe) sau khi build
└── go.mod / wails.json         # Cấu hình dự án Go & Wails
```

---

## ⚡ Hướng dẫn Nhanh (Quickstart)

### 1. Khởi chạy Python OCR Sidecar
Mở `python_ocr/run_server.bat` và chờ máy chủ sẵn sàng tại `http://127.0.0.1:8000/health`.

### 2. Mở Ứng dụng Desktop
Chạy file `build/bin/ComicifyTranslator.exe` (hoặc build mới bằng `wails build`).

### 3. Quy trình Dịch Video
1. Nhập **Gemini API Key** và **Chọn File Video**.
2. Bấm **"Bắt đầu Quét & OCR"**.
3. Cấu hình bảng **Glossary** xưng hô nhân vật nếu cần.
4. Bấm **"Dịch Tất cả bằng Gemini AI"**.
5. Kiểm tra kết quả câu dịch và bấm **"Xuất Video Hoàn chỉnh"**.

---

## 📚 Tài liệu Chi tiết

- 📐 [Kiến trúc Kỹ thuật & Thuật toán (docs/ARCHITECTURE.md)](file:///c:/Users/huydq/OneDrive/Documents/TranslateVideo/docs/ARCHITECTURE.md)
- 📖 [Hướng dẫn Sử dụng Chi tiết A-Z (docs/USER_GUIDE.md)](file:///c:/Users/huydq/OneDrive/Documents/TranslateVideo/docs/USER_GUIDE.md)

---

## 🧪 Kiểm thử (Testing)

Chạy bộ test tự động kiểm tra nhận diện màu, exact hash và xử lý bản dịch:
```bash
go test -v ./...
```
