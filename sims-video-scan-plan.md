# Kế Hoạch Nâng Cấp Pipeline Quét & Dịch Cho Video Dài / Sims Story (`Addict.mp4`)

> **Mục tiêu**: Tối ưu hóa toàn diện tốc độ quét OCR, độ chính xác và chất lượng dịch thuật cho các video thời lượng dài (2 tiếng+) dạng Sims Story / Hardsub đáy màn hình.

---

## 🎯 1. Bối Cảnh & Vấn Đề Cần Giải Quyết

Từ phân tích thực tế video `Addict.mp4` (2 giờ 00 phút, 720p @ 60 FPS):
1. **Thời lượng lớn (7.200s)**: Quét Full-frame 720p thông thường tốn nhiều thời gian và GPU/RAM.
2. **Nhiễu đồng hồ đếm ngược**: Góc trên màn hình (`[1123, 15]`) có số đếm từng tập dễ bị quét nhầm nếu không giới hạn vùng.
3. **Phụ đề cố định ở đáy**: Thoại tiếng Anh luôn nằm ở 15% dải đáy màn hình (`y: 85% -> 100%`).
4. **Chất lượng dịch**: Cần tận dụng **Gemini 3.5 Flash Lite** dịch theo cụm ngữ cảnh để giữ mạch câu và xưng hô tự nhiên cho phim tình cảm / đời thường.

---

## 📋 2. Phân Rã Công Việc & Nhiệm Vụ (Task Breakdown)

### Giai đoạn 1: Tối ưu hóa Bộ Trích Xuất & Quét Vùng Đáy (Bottom ROI OCR)
- **Nhiệm vụ 1.1**: Cập nhật `pkg/pipeline/pipeline.go` & `pkg/config/config.go`
  - Thêm tham số cấu hình ROI (`ScanRegion`: `full`, `bottom_bar`, `custom`).
  - Khi chọn chế độ `sims_en` hoặc chọn quét vùng đáy: FFmpeg tự động áp dụng crop filter hoặc trích xuất crop dải dưới màn hình trước khi gửi sang OCR server.
- **Nhiệm vụ 1.2**: Tối ưu hóa Python OCR Sidecar (`python_ocr/server.py`)
  - Hỗ trợ xử lý ảnh crop kích thước nhỏ, tăng tốc độ inference GPU DirectML lên gấp 4 - 6 lần.
  - Tự động lọc sạch các chuỗi số thời gian đếm ngược.

### Giai đoạn 2: Thuật Toán Gom Cụm Thoại & Khử Trùng (Timeline Clustering)
- **Nhiệm vụ 2.1**: Nâng cấp module gộp câu trong `pkg/pipeline/pipeline.go`
  - So khớp chuỗi Levenshtein Similarity (ngưỡng 85%) kết hợp delta thời gian để gộp các khung hình liên tiếp thành 1 câu thoại hoàn chỉnh với `timestamp` và `endTimestamp` chuẩn xác.
  - Tự động làm sạch dấu câu và chuẩn hóa chính tả tiếng Anh qua `_clean_english_text`.

### Giai đoạn 3: Dịch Thuật Ngữ Cảnh Qua Gemini 3.5 Flash Lite
- **Nhiệm vụ 3.1**: Cập nhật Prompt dịch thuật trong `pkg/translation/gemini.go`
  - Bổ sung chuyên sâu cho thể loại `sims_story` / `movie_dialogue`.
  - Hỗ trợ phân lô dịch (batching 30 - 50 câu) kèm ngữ cảnh của tập phim trước đó để giữ đại từ xưng hô (Anh - Em / Tôi - Cô) nhất quán suốt 2 tiếng.

### Giai đoạn 4: Hiển Thị UI & Xuất Video Chuẩn Điện Ảnh
- **Nhiệm vụ 4.1**: Cập nhật giao diện `frontend/`
  - Tự động gợi ý kiểu phụ đề `sims_bar` và tần suất quét `0.50s` khi chọn `sims_en`.
  - Hiển thị thông báo thời gian quét và số câu tìm được trực quan.
- **Nhiệm vụ 4.2**: Kiểm thử xuất video MP4 với thanh che đáy `sims_bar` đè kín hoàn toàn sub tiếng Anh gốc.

---

## 🏗️ 3. Phân Công Chuyên Gia (Agent Assignments)

| Thành phần | Chuyên gia phụ trách | Trách nhiệm |
|------------|---------------------|-------------|
| Backend Pipeline & Go Engine | `backend-specialist` | Xử lý ROI FFmpeg, cấu hình `ScanRegion`, Levenshtein clustering |
| Python OCR Sidecar | `backend-specialist` | Tối ưu hóa DirectML inference cho batch ảnh crop |
| AI Prompt & Gemini 3.5 Flash Lite | `backend-specialist` | Tối ưu hóa batch translation context engine |
| UI/UX & Controls | `frontend-specialist` | Cập nhật cấu hình form, dropdown, live timer, video overlay |
| Build & Git Automation | `devops-engineer` | `wails build`, test, auto commit & push |

---

## 🧪 4. Kế Hoạch Kiểm Thử & Nghiệm Thu (Verification Checklist)

- [ ] **Kiểm tra tốc độ**: Quét thử 5 phút đầu của `Addict.mp4` với ROI Bottom, thời gian xử lý phải < 10 giây.
- [ ] **Kiểm tra lọc nhiễu**: Không còn bắt nhầm số đồng hồ đếm ngược ở góc trên bên phải.
- [ ] **Kiểm tra dịch thuật**: Gemini 3.5 Flash Lite dịch mượt mà danh sách thoại theo văn phong phim ngôn tình/đời thường.
- [ ] **Kiểm tra đè sub**: Xem trước trên Canvas và xuất thử 1 đoạn video MP4 kiểm tra dải sub che kín sub gốc.
- [ ] **Tự động Commit & Push**: Toàn bộ thay đổi được commit và đẩy lên GitHub `main`.
