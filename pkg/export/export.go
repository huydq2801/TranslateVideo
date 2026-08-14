package export

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/fogleman/gg"
	"translate-video/pkg/config"
)

func GetVideoDuration(videoPath string) float64 {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", videoPath)
	SetHideWindow(cmd)
	out, err := cmd.Output()
	if err == nil {
		dur, errD := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		if errD == nil && dur > 0 {
			return dur
		}
	}
	return 0.0
}

func GetVideoDimensions(videoPath string) (int, int) {
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "csv=p=0", videoPath)
	SetHideWindow(cmd)
	out, err := cmd.Output()
	if err == nil {
		parts := strings.Split(strings.TrimSpace(string(out)), ",")
		if len(parts) >= 2 {
			w, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
			h, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
			if errW == nil && errH == nil && w > 0 && h > 0 {
				return w, h
			}
		}
	}
	return 1920, 1080
}

func RenderSpeechBubblePNG(d config.DialogueItem, index int, outDir string, canvasWidth, canvasHeight int) (string, error) {
	if canvasWidth <= 0 || canvasHeight <= 0 {
		canvasWidth, canvasHeight = 1920, 1080
	}

	dc := gg.NewContext(canvasWidth, canvasHeight)
	text := strings.TrimSpace(d.Translated)
	if text == "" {
		text = strings.TrimSpace(d.Original)
	}

	bbox := d.BBox
	bubbleX := bbox.X
	bubbleY := bbox.Y
	bubbleW := bbox.Width
	bubbleH := bbox.Height

	// Shadow
	dc.SetHexColor("#000000")
	dc.DrawRoundedRectangle(bubbleX+3, bubbleY+4, bubbleW, bubbleH, 16)
	dc.Fill()

	// Bubble body
	dc.SetHexColor("#fffdf0") // Màu kem nhẹ, dễ đọc hơn trắng tinh
	dc.DrawRoundedRectangle(bubbleX, bubbleY, bubbleW, bubbleH, 16)
	dc.Fill()

	// Border
	dc.SetHexColor("#2c3e50")
	dc.SetLineWidth(4)
	dc.DrawRoundedRectangle(bubbleX, bubbleY, bubbleW, bubbleH, 16)
	dc.Stroke()

	// === Text Rendering ===
	fontSize := bubbleH * 0.52
	minFontSize := 20.0
	var bestLines []string
	var bestFontSize float64
	var totalTextHeight float64

	for fontSize >= minFontSize {
		if err := loadBestFont(dc, fontSize); err != nil {
			fontSize -= 2
			continue
		}

		lines, ok := wrapText(dc, text, bubbleW*0.88)
		if !ok {
			fontSize -= 2
			continue
		}

		totalTextHeight = float64(len(lines)) * (fontSize * 1.32)

		if totalTextHeight <= bubbleH*0.78 {
			bestLines = lines
			bestFontSize = fontSize
			break
		}
		fontSize -= 2
	}

	// Vẽ chữ
	if bestFontSize > 0 && len(bestLines) > 0 {
		loadBestFont(dc, bestFontSize)
		dc.SetHexColor("#1f2a44")

		lineHeight := bestFontSize * 1.32
		startY := bubbleY + (bubbleH-totalTextHeight)/2 + bestFontSize*0.85

		for i, line := range bestLines {
			tw, _ := dc.MeasureString(line)
			textX := bubbleX + (bubbleW-tw)/2
			textY := startY + float64(i)*lineHeight
			dc.DrawString(line, textX, textY)
		}
	}

	// Đuôi bubble (đã tắt theo yêu cầu)
	// drawBubbleTail(dc, bubbleX+bubbleW*0.35, bubbleY+bubbleH-6, 32, 26)

	outputPath := filepath.Join(outDir, fmt.Sprintf("bubble_%03d.png", index))
	if err := dc.SavePNG(outputPath); err != nil {
		return "", err
	}
	return outputPath, nil
}

func RenderSimsSubtitlePNG(d config.DialogueItem, index int, outDir string, canvasWidth, canvasHeight int) (string, error) {
	if canvasWidth <= 0 || canvasHeight <= 0 {
		canvasWidth, canvasHeight = 1920, 1080
	}

	dc := gg.NewContext(canvasWidth, canvasHeight)
	text := strings.TrimSpace(d.Translated)
	if text == "" {
		text = strings.TrimSpace(d.Original)
	}

	bbox := d.BBox
	subX := bbox.X
	subY := bbox.Y
	subW := bbox.Width
	subH := bbox.Height

	// Ensure reasonable minimum padding for bottom subtitle banner
	padX := math.Max(8.0, subW*0.05)
	padY := math.Max(6.0, subH*0.08)

	bgX := math.Max(10, subX-padX)
	bgY := math.Max(10, subY-padY)
	bgW := math.Min(float64(canvasWidth)-bgX-10, subW+2*padX)
	bgH := math.Min(float64(canvasHeight)-bgY-10, subH+2*padY)

	// Semi-transparent dark background bar matching Sims 4 narrative style
	dc.SetRGBA(0.08, 0.08, 0.12, 0.88)
	dc.DrawRoundedRectangle(bgX, bgY, bgW, bgH, 10)
	dc.Fill()

	// Subtle border
	dc.SetRGBA(0.4, 0.4, 0.5, 0.6)
	dc.SetLineWidth(1.5)
	dc.DrawRoundedRectangle(bgX, bgY, bgW, bgH, 10)
	dc.Stroke()

	// === Text Rendering ===
	fontSize := bgH * 0.48
	minFontSize := 18.0
	var bestLines []string
	var bestFontSize float64
	var totalTextHeight float64

	for fontSize >= minFontSize {
		if err := loadBestFont(dc, fontSize); err != nil {
			fontSize -= 2
			continue
		}

		lines, ok := wrapText(dc, text, bgW*0.92)
		if !ok {
			fontSize -= 2
			continue
		}

		totalTextHeight = float64(len(lines)) * (fontSize * 1.30)
		if totalTextHeight <= bgH*0.82 {
			bestLines = lines
			bestFontSize = fontSize
			break
		}
		fontSize -= 2
	}

	// Draw text with shadow
	if bestFontSize > 0 && len(bestLines) > 0 {
		loadBestFont(dc, bestFontSize)
		lineHeight := bestFontSize * 1.30
		startY := bgY + (bgH-totalTextHeight)/2 + bestFontSize*0.85

		for i, line := range bestLines {
			tw, _ := dc.MeasureString(line)
			textX := bgX + (bgW-tw)/2
			textY := startY + float64(i)*lineHeight

			// Shadow
			dc.SetRGBA(0, 0, 0, 0.95)
			dc.DrawString(line, textX+1.5, textY+1.5)

			// Text - warm golden white for story subtitle readability
			dc.SetHexColor("#fffdf2")
			dc.DrawString(line, textX, textY)
		}
	}

	outputPath := filepath.Join(outDir, fmt.Sprintf("bubble_%03d.png", index))
	if err := dc.SavePNG(outputPath); err != nil {
		return "", err
	}
	return outputPath, nil
}

func OverlayEndTime(dialogues []config.DialogueItem, index int) float64 {
	dialogue := dialogues[index]

	if dialogue.EndTimestamp > dialogue.Timestamp {
		end := dialogue.EndTimestamp
		if index < len(dialogues)-1 {
			nextStart := dialogues[index+1].Timestamp
			if end >= nextStart {
				return math.Max(dialogue.Timestamp+0.1, nextStart-0.05)
			}
		}
		return end
	}

	if dialogue.Duration > 0 {
		end := dialogue.Timestamp + dialogue.Duration
		if index < len(dialogues)-1 {
			nextStart := dialogues[index+1].Timestamp
			if end >= nextStart {
				return math.Max(dialogue.Timestamp+0.1, nextStart-0.05)
			}
		}
		return end
	}

	if index < len(dialogues)-1 {
		nextStart := dialogues[index+1].Timestamp
		return math.Max(dialogue.Timestamp+0.1, nextStart-0.05)
	}

	return dialogue.Timestamp + 2.0
}

func FormatSRTTimestamp(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	h := int(seconds) / 3600
	m := (int(seconds) % 3600) / 60
	s := int(seconds) % 60
	ms := int((seconds - float64(int(seconds))) * 1000)
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

func ExportSRT(dialoguesJSON string, targetOutputPath string) (string, error) {
	var dialogues []config.DialogueItem
	err := json.Unmarshal([]byte(dialoguesJSON), &dialogues)
	if err != nil {
		return "", fmt.Errorf("lỗi đọc danh sách thoại: %v", err)
	}

	var validDialogues []config.DialogueItem
	for _, d := range dialogues {
		if !d.IsTrash {
			validDialogues = append(validDialogues, d)
		}
	}
	if len(validDialogues) == 0 {
		return "", fmt.Errorf("không có câu thoại hợp lệ nào để xuất SRT")
	}

	sort.Slice(validDialogues, func(i, j int) bool {
		return validDialogues[i].Timestamp < validDialogues[j].Timestamp
	})

	var sb strings.Builder
	for i, d := range validDialogues {
		text := strings.TrimSpace(d.Translated)
		if text == "" {
			text = strings.TrimSpace(d.Original)
		}
		if text == "" {
			continue
		}

		endSec := OverlayEndTime(validDialogues, i)
		if endSec <= d.Timestamp {
			endSec = d.Timestamp + 1.5
		}

		sb.WriteString(fmt.Sprintf("%d\n", i+1))
		sb.WriteString(fmt.Sprintf("%s --> %s\n", FormatSRTTimestamp(d.Timestamp), FormatSRTTimestamp(endSec)))
		sb.WriteString(fmt.Sprintf("%s\n\n", text))
	}

	outputPath := strings.TrimSpace(targetOutputPath)
	if outputPath == "" {
		outputPath = "subtitles_vietsub.srt"
	}

	if err := os.WriteFile(outputPath, []byte(sb.String()), 0644); err != nil {
		return "", fmt.Errorf("lỗi lưu file SRT: %v", err)
	}
	return outputPath, nil
}

func ExportVideo(videoPath string, dialoguesJSON string, targetOutputPath string) (string, error) {
	return ExportVideoWithOptions(videoPath, dialoguesJSON, targetOutputPath, "bubble")
}

func ExportVideoWithOptions(videoPath string, dialoguesJSON string, targetOutputPath string, subtitleStyle string) (string, error) {
	var dialogues []config.DialogueItem
	err := json.Unmarshal([]byte(dialoguesJSON), &dialogues)
	if err != nil {
		return "", fmt.Errorf("lỗi đọc danh sách thoại: %v", err)
	}

	if len(dialogues) == 0 {
		return "", fmt.Errorf("chưa có khung thoại nào để xuất")
	}

	if videoPath == "" {
		return "", fmt.Errorf("chưa chọn file video nguồn")
	}

	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		return "", fmt.Errorf("không tìm thấy video: %s", videoPath)
	}

	// Lọc bỏ rác và chỉ lấy thoại hợp lệ
	var validDialogues []config.DialogueItem
	for _, d := range dialogues {
		if !d.IsTrash {
			validDialogues = append(validDialogues, d)
		}
	}
	if len(validDialogues) == 0 {
		return "", fmt.Errorf("không có khung thoại hợp lệ để xuất")
	}

	// Sắp xếp thoại theo mốc thời gian
	sort.Slice(validDialogues, func(i, j int) bool {
		return validDialogues[i].Timestamp < validDialogues[j].Timestamp
	})

	tempDir, err := os.MkdirTemp("", "comicify_export_*")
	if err != nil {
		return "", fmt.Errorf("lỗi tạo thư mục tạm: %v", err)
	}
	defer os.RemoveAll(tempDir)

	vW, vH := GetVideoDimensions(videoPath)

	// Tạo 1 ảnh rỗng trong suốt có kích thước bằng canvas video
	blankPath := filepath.Join(tempDir, "blank.png")
	dcBlank := gg.NewContext(vW, vH)
	if err := dcBlank.SavePNG(blankPath); err != nil {
		return "", fmt.Errorf("lỗi tạo frame rỗng: %v", err)
	}

	for i, d := range validDialogues {
		var renderErr error
		if subtitleStyle == "sims_bar" {
			_, renderErr = RenderSimsSubtitlePNG(d, i, tempDir, vW, vH)
		} else {
			_, renderErr = RenderSpeechBubblePNG(d, i, tempDir, vW, vH)
		}
		if renderErr != nil {
			return "", fmt.Errorf("lỗi render khung thoại %d: %v", i, renderErr)
		}
	}

	outputPath := strings.TrimSpace(targetOutputPath)
	if outputPath == "" {
		ext := filepath.Ext(videoPath)
		baseName := strings.TrimSuffix(filepath.Base(videoPath), ext)
		if baseName == "" {
			baseName = "video"
		}
		outputFilename := fmt.Sprintf("%s_vietsub.mp4", baseName)
		outputPath = filepath.Join(filepath.Dir(videoPath), outputFilename)
	}

	// Xây dựng concat script file để FFmpeg ghép luồng ảnh trong suốt mà không làm tràn Stack/Heap FilterGraph
	concatScriptPath := filepath.Join(tempDir, "concat_script.txt")
	var concatStr strings.Builder
	concatStr.WriteString("ffconcat version 1.0\n")

	blankSlash := filepath.ToSlash(blankPath)
	currentTime := 0.0

	for i, d := range validDialogues {
		startSec := d.Timestamp
		endSec := OverlayEndTime(validDialogues, i)

		if endSec <= startSec {
			endSec = startSec + 0.1
		}

		if startSec > currentTime {
			gapDuration := startSec - currentTime
			if gapDuration > 0.001 {
				concatStr.WriteString(fmt.Sprintf("file '%s'\n", blankSlash))
				concatStr.WriteString(fmt.Sprintf("duration %.3f\n", gapDuration))
			}
		} else if startSec < currentTime {
			startSec = currentTime
		}

		diagDuration := endSec - startSec
		if diagDuration > 0.001 {
			pngPath := filepath.Join(tempDir, fmt.Sprintf("bubble_%03d.png", i))
			pngSlash := filepath.ToSlash(pngPath)
			concatStr.WriteString(fmt.Sprintf("file '%s'\n", pngSlash))
			concatStr.WriteString(fmt.Sprintf("duration %.3f\n", diagDuration))
		}

		currentTime = endSec
	}

	totalDuration := GetVideoDuration(videoPath)
	if totalDuration > currentTime {
		finalGap := totalDuration - currentTime
		if finalGap > 0.001 {
			concatStr.WriteString(fmt.Sprintf("file '%s'\n", blankSlash))
			concatStr.WriteString(fmt.Sprintf("duration %.3f\n", finalGap))
		}
	}

	concatStr.WriteString(fmt.Sprintf("file '%s'\n", blankSlash))

	err = os.WriteFile(concatScriptPath, []byte(concatStr.String()), 0644)
	if err != nil {
		return "", fmt.Errorf("lỗi tạo kịch bản export: %v", err)
	}

	baseArgs := []string{
		"-y",
		"-i", videoPath,
		"-safe", "0",
		"-f", "concat",
		"-i", concatScriptPath,
		"-filter_complex", "[0:v][1:v]overlay=0:0:eof_action=pass[v]",
		"-map", "[v]",
		"-map", "0:a?",
		"-c:a", "copy",
	}

	gpuEncoders := []string{"h264_nvenc", "h264_qsv", "libx264"}
	var lastErr error

	for _, encoder := range gpuEncoders {
		cmdArgs := append(baseArgs, "-c:v", encoder, "-preset", "fast", outputPath)
		cmd := exec.Command("ffmpeg", cmdArgs...)
		SetHideWindow(cmd)
		outBytes, err := cmd.CombinedOutput()
		if err == nil {
			return outputPath, nil
		}
		lastErr = fmt.Errorf("lỗi encoder (%s): %v\nOutput: %s", encoder, err, string(outBytes))
	}

	return "", fmt.Errorf("xuất video thất bại qua tất cả encoder: %v", lastErr)
}

func loadBestFont(dc *gg.Context, size float64) error {
	fontPaths := []string{
		"C:\\Windows\\Fonts\\arial.ttf",
		"C:\\Windows\\Fonts\\segoeui.ttf",
		"/System/Library/Fonts/Helvetica.ttc",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}
	for _, p := range fontPaths {
		if _, err := os.Stat(p); err == nil {
			return dc.LoadFontFace(p, size)
		}
	}
	return dc.LoadFontFace("", size) // fallback
}

func wrapText(dc *gg.Context, text string, maxWidth float64) ([]string, bool) {
	words := strings.Fields(text)
	var lines []string
	var current string

	for _, word := range words {
		test := current
		if test != "" {
			test += " "
		}
		test += word
		w, _ := dc.MeasureString(test)
		if w > maxWidth && current != "" {
			lines = append(lines, current)
			current = word
		} else {
			current = test
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines, len(lines) > 0
}

func drawBubbleTail(dc *gg.Context, x, y float64, w, h float64) {
	dc.SetHexColor("#ffffff")
	dc.MoveTo(x, y)
	dc.LineTo(x+w*0.4, y+h)
	dc.LineTo(x+w*0.7, y)
	dc.ClosePath()
	dc.Fill()

	dc.SetHexColor("#2c3e50")
	dc.SetLineWidth(3)
	dc.MoveTo(x, y)
	dc.LineTo(x+w*0.4, y+h)
	dc.LineTo(x+w*0.7, y)
	dc.Stroke()
}
