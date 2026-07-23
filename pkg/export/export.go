package export

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"translate-video/pkg/config"
)

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

	bbox := d.BBox
	text := d.Translated
	if text == "" {
		text = d.Original
	}

	x := bbox.X
	y := bbox.Y
	w := bbox.Width
	h := bbox.Height
	r := 15.0

	dc.SetHexColor("#ffffff")
	dc.DrawRoundedRectangle(x, y, w, h, r)
	dc.Fill()

	dc.SetHexColor("#000000")
	fontSize := 32.0 * (float64(canvasHeight) / 1080.0)
	if fontSize < 16 {
		fontSize = 16
	}

	fontCandidates := []string{
		"C:\\Windows\\Fonts\\arial.ttf",
		"C:\\Windows\\Fonts\\segoeui.ttf",
		"C:\\Windows\\Fonts\\tahoma.ttf",
		"/System/Library/Fonts/Helvetica.ttc",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}

	for _, fPath := range fontCandidates {
		if _, err := os.Stat(fPath); err == nil {
			if err := dc.LoadFontFace(fPath, fontSize); err == nil {
				break
			}
		}
	}

	words := strings.Fields(text)
	var lines []string
	var currentLine string
	maxWidth := w * 0.85

	for _, word := range words {
		testLine := word
		if currentLine != "" {
			testLine = currentLine + " " + word
		}
		tw, _ := dc.MeasureString(testLine)
		if tw > maxWidth && currentLine != "" {
			lines = append(lines, currentLine)
			currentLine = word
		} else {
			currentLine = testLine
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	lineHeight := fontSize * 1.3
	startY := y + (h / 2.0) - (float64(len(lines)-1) * lineHeight / 2.0)

	for i, line := range lines {
		dc.DrawStringAnchored(line, x+(w/2.0), startY+(float64(i)*lineHeight), 0.5, 0.5)
	}

	filename := fmt.Sprintf("bubble_%d.png", index)
	outPath := filepath.Join(outDir, filename)
	err := dc.SavePNG(outPath)
	if err != nil {
		return "", err
	}

	return outPath, nil
}

func OverlayEndTime(dialogues []config.DialogueItem, index int) float64 {
	dialogue := dialogues[index]

	if dialogue.EndTimestamp > dialogue.Timestamp {
		end := dialogue.EndTimestamp
		if index < len(dialogues)-1 {
			nextStart := dialogues[index+1].Timestamp
			if end >= nextStart {
				return max(dialogue.Timestamp+0.1, nextStart-0.05)
			}
		}
		return end
	}

	if dialogue.Duration > 0 {
		end := dialogue.Timestamp + dialogue.Duration
		if index < len(dialogues)-1 {
			nextStart := dialogues[index+1].Timestamp
			if end >= nextStart {
				return max(dialogue.Timestamp+0.1, nextStart-0.05)
			}
		}
		return end
	}

	if index < len(dialogues)-1 {
		nextStart := dialogues[index+1].Timestamp
		return max(dialogue.Timestamp+0.1, nextStart-0.05)
	}

	return dialogue.Timestamp + 2.0
}

func ExportVideo(videoPath string, dialoguesJSON string, targetOutputPath string) (string, error) {
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

	tempDir, err := os.MkdirTemp("", "comicify_export_*")
	if err != nil {
		return "", fmt.Errorf("lỗi tạo thư mục tạm: %v", err)
	}
	defer os.RemoveAll(tempDir)

	vW, vH := GetVideoDimensions(videoPath)

	for i, d := range dialogues {
		_, err := RenderSpeechBubblePNG(d, i, tempDir, vW, vH)
		if err != nil {
			return "", fmt.Errorf("lỗi render khung thoại %d: %v", i, err)
		}
	}

	outputPath := strings.TrimSpace(targetOutputPath)
	if outputPath == "" {
		outputFilename := fmt.Sprintf("video_translated_%d.mp4", time.Now().UnixNano())
		outputPath = filepath.Join(filepath.Dir(videoPath), outputFilename)
	}

	args := []string{"-y", "-i", videoPath}
	for i := range dialogues {
		pngPath := filepath.Join(tempDir, fmt.Sprintf("bubble_%d.png", i))
		args = append(args, "-i", pngPath)
	}

	var filterStr strings.Builder
	lastLabel := "[0:v]"
	for i, d := range dialogues {
		currentLabel := fmt.Sprintf("[v%d]", i+1)
		endSec := OverlayEndTime(dialogues, i)
		filterStr.WriteString(fmt.Sprintf("%s[%d:v]overlay=0:0:enable='between(t,%.2f,%.2f)'%s; ", lastLabel, i+1, d.Timestamp, endSec, currentLabel))
		lastLabel = currentLabel
	}

	filterComplex := strings.TrimRight(filterStr.String(), "; ")
	args = append(args, "-filter_complex", filterComplex, "-map", lastLabel, "-map", "0:a?", "-c:a", "copy")

	gpuEncoders := []string{"h264_nvenc", "h264_qsv", "libx264"}
	var lastErr error

	for _, encoder := range gpuEncoders {
		cmdArgs := append(args, "-c:v", encoder, "-preset", "fast", outputPath)
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
