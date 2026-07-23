package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"translate-video/pkg/config"
	"translate-video/pkg/export"
	"translate-video/pkg/ocr"
	"translate-video/pkg/pipeline"
	"translate-video/pkg/translation"
)

type App struct {
	ctx        context.Context
	scanCancel context.CancelFunc
	scanMutex  sync.Mutex
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) CancelScan() {
	a.scanMutex.Lock()
	defer a.scanMutex.Unlock()
	if a.scanCancel != nil {
		a.scanCancel()
		a.scanCancel = nil
	}
}

func (a *App) SelectVideoFile() (string, error) {
	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Chọn Video Cần Dịch",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Video Files (*.mp4;*.mkv;*.avi;*.mov;*.webm)",
				Pattern:     "*.mp4;*.mkv;*.avi;*.mov;*.webm;*.flv",
			},
		},
	})
	if err != nil {
		return "", err
	}
	if filePath != "" {
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return "", fmt.Errorf("file không tồn tại: %s", filePath)
		}
	}
	return filePath, nil
}

func (a *App) SelectSaveLocation(defaultName string) (string, error) {
	if defaultName == "" {
		defaultName = "translated_video.mp4"
	}
	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Chọn vị trí và đặt tên file video xuất",
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{
				DisplayName: "MP4 Video (*.mp4)",
				Pattern:     "*.mp4",
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("lỗi chọn vị trí lưu: %v", err)
	}
	return savePath, nil
}

func (a *App) CheckSystemHealth(apiKey string) config.HealthStatus {
	status := config.HealthStatus{
		OCROnline: false,
		OCREngine: "Offline",
		HasAPIKey: false,
	}

	apiKey = strings.TrimSpace(apiKey)
	if apiKey != "" {
		status.HasAPIKey = true
	}

	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:8000/health")
	if err == nil && resp.StatusCode == http.StatusOK {
		var ocrHealth struct {
			Status string `json:"status"`
			Engine string `json:"engine"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&ocrHealth); err == nil {
			status.OCROnline = (ocrHealth.Status == "online")
			status.OCREngine = ocrHealth.Engine
		}
		resp.Body.Close()
	}

	return status
}

type ScanProgress struct {
	CurrentFrame int    `json:"currentFrame"`
	TotalFrames  int    `json:"totalFrames"`
	FoundCount   int    `json:"foundCount"`
	Status       string `json:"status"`
}

func (a *App) StartFullPipelineScan(cfg config.ScanConfig) ([]config.DialogueItem, error) {
	if cfg.Threshold == 0 {
		cfg.Threshold = 240
	}

	scanCtx, cancel := context.WithCancel(a.ctx)
	a.scanMutex.Lock()
	a.scanCancel = cancel
	a.scanMutex.Unlock()

	defer func() {
		a.scanMutex.Lock()
		a.scanCancel = nil
		a.scanMutex.Unlock()
	}()

	onProgress := func(curr, total, found int, status string) {
		runtime.EventsEmit(a.ctx, "scan-progress", ScanProgress{
			CurrentFrame: curr,
			TotalFrames:  total,
			FoundCount:   found,
			Status:       status,
		})
	}

	return pipeline.ScanFullFramePipeline(scanCtx, cfg, onProgress)
}

func (a *App) FilterTrashWithAI(apiKey string, dialogues []config.DialogueItem) ([]config.DialogueItem, error) {
	return translation.FilterTrashDialogues(a.ctx, apiKey, dialogues)
}

func (a *App) AnalyzeContextAndGlossary(apiKey string, dialogues []config.DialogueItem) ([]config.GlossaryEntry, error) {
	return translation.AnalyzeStoryContextAndGlossary(a.ctx, apiKey, dialogues)
}

func (a *App) TranslateDialogues(apiKey string, dialogues []config.DialogueItem, glossaryJSON string) ([]config.DialogueItem, error) {
	if len(dialogues) == 0 {
		return dialogues, nil
	}

	glossaryPrompt := ""
	if strings.TrimSpace(glossaryJSON) != "" {
		var entries []config.GlossaryEntry
		if err := json.Unmarshal([]byte(glossaryJSON), &entries); err == nil && len(entries) > 0 {
			var sb strings.Builder
			sb.WriteString("\n\nBẢNG XƯNG HÔ & TÊN RIÊNG BẮT BUỘC TUÂN THỦ:\n")
			for _, e := range entries {
				sb.WriteString(fmt.Sprintf("- \"%s\" => \"%s\"\n", e.Source, e.Target))
			}
			glossaryPrompt = sb.String()
		}
	}

	for i := range dialogues {
		d := &dialogues[i]
		if d.Original == "" || d.IsTrash {
			continue
		}

		prompt := fmt.Sprintf(`Dịch câu thoại truyện tranh sau từ Tiếng Trung/Tiếng Anh sang Tiếng Việt tự nhiên:%s
Thoại gốc: "%s"

CHỈ trả về kết quả tiếng Việt dịch xong. KHÔNG thêm giải thích hay ngoặc kép.`, glossaryPrompt, d.Original)

		res, err := translation.CallGeminiAPIWithContext(a.ctx, apiKey, prompt)
		if err == nil && res != "" {
			cleaned, failed := translation.CleanTranslation(res)
			d.Translated = cleaned
			d.TranslationFailed = failed
		} else {
			d.TranslationFailed = true
		}
	}

	return dialogues, nil
}

func (a *App) ExportVideo(videoPath string, dialoguesJSON string, targetOutputPath string) (string, error) {
	return export.ExportVideo(videoPath, dialoguesJSON, targetOutputPath)
}

func (a *App) SnapSingleDialogueBBox(videoPath string, dialogue config.DialogueItem, threshold uint8, apiKey string) (config.DialogueItem, error) {
	if videoPath == "" {
		return dialogue, fmt.Errorf("chưa chọn file video")
	}

	tempDir, err := os.MkdirTemp("", "comicify_snap_*")
	if err != nil {
		return dialogue, fmt.Errorf("lỗi tạo thư mục tạm: %v", err)
	}
	defer os.RemoveAll(tempDir)

	framePath := filepath.Join(tempDir, "frame_snap.jpg")
	midTime := dialogue.Timestamp
	if dialogue.EndTimestamp > dialogue.Timestamp {
		midTime = (dialogue.Timestamp + dialogue.EndTimestamp) / 2.0
	} else if dialogue.Duration > 0 {
		midTime = dialogue.Timestamp + (dialogue.Duration / 2.0)
	}

	cmd := exec.Command("ffmpeg", "-y", "-ss", fmt.Sprintf("%.3f", midTime), "-i", videoPath, "-vframes", "1", "-q:v", "2", framePath)
	if err := cmd.Run(); err != nil {
		return dialogue, fmt.Errorf("lỗi trích xuất frame: %v", err)
	}

	reqPaths := []string{framePath}
	resMap, ocrErr := ocr.CallPythonOCRFullFrameBatch(a.ctx, reqPaths, "chi_sim")
	if ocrErr == nil {
		if items, ok := resMap[framePath]; ok && len(items) > 0 {
			mergedItems := ocr.MergeVerticallyAdjacentLines(items)
			centerCurrentX := dialogue.BBox.X + dialogue.BBox.Width/2.0
			centerCurrentY := dialogue.BBox.Y + dialogue.BBox.Height/2.0

			minDist := 1e9
			var matchedItem *ocr.OCRFullFrameItem

			for i := range mergedItems {
				itemX := float64(mergedItems[i].BBox[0]) + float64(mergedItems[i].BBox[2])/2.0
				itemY := float64(mergedItems[i].BBox[1]) + float64(mergedItems[i].BBox[3])/2.0
				dist := (itemX-centerCurrentX)*(itemX-centerCurrentX) + (itemY-centerCurrentY)*(itemY-centerCurrentY)
				if dist < minDist {
					minDist = dist
					matchedItem = &mergedItems[i]
				}
			}

			if matchedItem != nil {
				textBBox := config.BBox{
					X:      float64(matchedItem.BBox[0]),
					Y:      float64(matchedItem.BBox[1]),
					Width:  float64(matchedItem.BBox[2]),
					Height: float64(matchedItem.BBox[3]),
				}
				padY := max(10.0, textBBox.Height*0.15)
				padX := max(15.0, textBBox.Width*0.08)
				dialogue.BBox = config.BBox{
					X:      max(0, textBBox.X-padX),
					Y:      max(0, textBBox.Y-padY),
					Width:  textBBox.Width + 2*padX,
					Height: textBBox.Height + 2*padY,
				}
				return dialogue, nil
			}
		}
	}

	return dialogue, nil
}
