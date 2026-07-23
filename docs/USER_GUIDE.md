# 📖 Hướng dẫn Sử dụng (User Guide) - Comicify Video Translator

Tài liệu hướng dẫn chi tiết từ A-Z cách vận hành ứng dụng **Comicify Video Translator** để quét bong bóng thoại, nhận diện OCR chữ Trung/Nhật/Anh, dịch sang tiếng Việt bằng Gemini AI và xuất video hoàn chỉnh.

---

## 🛠️ 1. Yêu cầu Hệ thống & Môi trường

Trước khi bắt đầu, đảm bảo máy tính Windows của bạn đáp ứng các yêu cầu sau:

- **Hệ điều hành**: Windows 10/11 (64-bit).
- **FFmpeg & FFprobe**: Đã cài đặt và có trong biến môi trường `PATH`. (Có thể kiểm tra bằng lệnh `ffmpeg -version` trong CMD).
- **Python**: Phiên bản 3.10 trở lên để chạy máy chủ OCR Sidecar.
- **Gemini API Key**: Đăng ký miễn phí tại [Google AI Studio](https://aistudio.google.com/) để lấy API Key.

---

## 🚀 2. Hướng dẫn Khởi chạy Ứng dụng

### Bước 1: Mở Máy chủ OCR Sidecar (Python)
1. Truy cập thư mục `python_ocr/`.
2. Chạy file `run_server.bat` (hoặc mở Terminal chạy `python server.py`).
3. Chờ cửa sổ Console hiển thị thông báo:
   ```text
   [OCR Sidecar] Starting HTTP server on http://127.0.0.1:8000...
   [OCR Sidecar] RapidOCR (GPU) initialized successfully.
   ```
4. Giữ cửa sổ Console này chạy ẩn trong suốt quá trình dịch video.

### Bước 2: Mở Ứng dụng Desktop
Mở file `build/bin/ComicifyTranslator.exe` (hoặc chạy lệnh `wails dev` khi lập trình).

---

## 📝 3. Hướng dẫn Dịch Video Theo Các Bước

### Bước 1: Cấu hình API Key & Chọn Video
1. Dán **Gemini API Key** của bạn vào ô nhập API Key ở góc trên giao diện.
2. Bấm nút **"Chọn File Video"** và chọn video game/anime cần dịch (định dạng `.mp4`, `.mkv`, `.mov`, `.webm`).

### Bước 2: Quét & Nhận diện OCR Bong bóng thoại
1. Điều chỉnh **Khoảng cách quét (giây)**: Mặc định là `1.0s` (hoặc `0.5s` nếu video thoại nhanh).
2. Điều chỉnh **Ngưỡng nhạy sáng (Threshold)**: Mặc định `240` (cho phép quét bong bóng thoại trắng, xám và màu).
3. Bấm **"Bắt đầu Quét & OCR"**.
4. Ứng dụng sẽ tự động trích xuất frame, chạy lọc trùng lặp `ExactFrameHash` và trích xuất danh sách câu thoại gốc.

### Bước 3: Giữ Nhất quán Nhân vật (Glossary & Cách xưng hô)
1. Mở mục **Glossary nhân vật & cách xưng hô**.
2. Thêm quy tắc dịch theo cú pháp `từ gốc => bản dịch bắt buộc` (mỗi quy tắc 1 dòng):
   ```text
   黄老师 => cô Hoàng
   李总 => Giám đốc Lý
   你 => cậu
   我 => tôi
   ```
3. Bộ Glossary này sẽ lưu tự động trên máy và được áp dụng cho mọi video.

### Bước 4: Dịch Batch Văn bản bằng Gemini AI
1. Bấm nút **"Dịch Tất cả bằng Gemini AI"**.
2. Ứng dụng tự động điều phối tần suất gọi API (tối đa 10 req/phút) để tránh vi phạm Quota 429 của Google.
3. Bản dịch tiếng Việt sẽ tự động xuất hiện trên bảng danh sách câu thoại. Bạn có thể sửa trực tiếp câu dịch nếu muốn điều chỉnh văn phong.

### Bước 5: Xuất Video Thành Phẩm (Render Overlay)
1. Xem trước hình ảnh vị trí bong bóng thoại và câu dịch tương ứng.
2. Bấm **"Xuất Video Hoàn chỉnh"**.
3. Hệ thống sẽ gọi FFmpeg kết hợp GPU (`NVENC`/`QSV`) để ghi đè bong bóng thoại mới vào video.
4. Sau khi hoàn tất, ứng dụng sẽ tự động mở thư mục chứa video thành phẩm (`video_translated_*.mp4`).

---

## ❓ 4. Xử lý Lỗi Thường Gặp (Troubleshooting)

| Sự cố / Lỗi | Nguyên nhân | Cách khắc phục |
| :--- | :--- | :--- |
| **"Lỗi OCR HTTP: Connection Refused"** | Chưa bật máy chủ Python OCR Sidecar | Mở lại `python_ocr/run_server.bat` và kiểm tra port 8000 đã sẵn sàng. |
| **"Lỗi API Gemini 429 Too Many Requests"** | Vượt quá Quota API của Google | Ứng dụng tự động chờ 15s-60s và thử lại. Bạn không cần bấm gì thêm. |
| **"Không tìm thấy FFmpeg"** | FFmpeg chưa được thêm vào `PATH` | Tải FFmpeg bản static build và thêm thư mục `bin` vào biến môi trường `PATH` của Windows. |
| **Quét bị bỏ sót thoại màu xám hoặc thoại 1 chữ** | Ngưỡng nhạy sáng đặt quá cao | Đảm bảo bạn đang sử dụng phiên bản ứng dụng mới nhất (đã hỗ trợ thoại xám/màu và 1 chữ). |
