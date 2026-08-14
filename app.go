package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
	scanCtx    context.Context
	scanMutex  sync.Mutex
	ocrCmd     *exec.Cmd
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.startPythonOCRServer()
}

func (a *App) shutdown(ctx context.Context) {
	if a.ocrCmd != nil && a.ocrCmd.Process != nil {
		_ = a.ocrCmd.Process.Kill()
	}
}

func (a *App) startPythonOCRServer() {
	// Pre-check if server is already running on http://127.0.0.1:8000
	client := http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get("http://127.0.0.1:8000/health")
	if err == nil && resp.StatusCode == http.StatusOK {
		resp.Body.Close()
		return
	}

	pythonExec := "python"
	if _, err := exec.LookPath("python"); err != nil {
		if _, errPy := exec.LookPath("py"); errPy == nil {
			pythonExec = "py"
		}
	}

	execDir, _ := os.Getwd()
	candidates := []string{
		filepath.Join(execDir, "python_ocr", "server.py"),
		filepath.Join(filepath.Dir(execDir), "python_ocr", "server.py"),
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "python_ocr", "server.py"),
			filepath.Join(filepath.Dir(exeDir), "python_ocr", "server.py"),
			filepath.Join(filepath.Dir(filepath.Dir(exeDir)), "python_ocr", "server.py"),
		)
	}

	serverScript := ""
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			serverScript = c
			break
		}
	}
	if serverScript == "" {
		return
	}

	cmd := exec.Command(pythonExec, serverScript)
	cmd.Dir = filepath.Dir(filepath.Dir(serverScript))
	pipeline.SetHideWindow(cmd)
	if err := cmd.Start(); err == nil {
		a.ocrCmd = cmd

		// Wait briefly in background for server to be ready and notify frontend
		go func() {
			for i := 0; i < 15; i++ {
				time.Sleep(500 * time.Millisecond)
				r, err := client.Get("http://127.0.0.1:8000/health")
				if err == nil && r.StatusCode == http.StatusOK {
					r.Body.Close()
					if a.ctx != nil {
						runtime.EventsEmit(a.ctx, "system-health-changed", true)
					}
					break
				}
			}
		}()
	}
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

func (a *App) SelectJSONFile() (string, error) {
	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Chọn File JSON Dự Phòng",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "JSON Files (*.json)",
				Pattern:     "*.json",
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
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("lỗi đọc file: %v", err)
		}
		return string(data), nil
	}
	return "", nil
}

func (a *App) SaveJSONFile(jsonContent string, defaultName string) (string, error) {
	if defaultName == "" {
		defaultName = "backup_dialogues.json"
	}
	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Lưu File JSON Dự Phòng",
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{
				DisplayName: "JSON File (*.json)",
				Pattern:     "*.json",
			},
		},
	})
	if err != nil || savePath == "" {
		return "", err
	}
	err = os.WriteFile(savePath, []byte(jsonContent), 0644)
	if err != nil {
		return "", fmt.Errorf("lỗi ghi file: %v", err)
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

func (a *App) createScanContext() (context.Context, func()) {
	a.scanMutex.Lock()
	if a.scanCancel != nil {
		a.scanCancel()
		a.scanCancel = nil
		a.scanCtx = nil
	}
	scanCtx, cancel := context.WithCancel(a.ctx)
	a.scanCancel = cancel
	a.scanCtx = scanCtx
	a.scanMutex.Unlock()

	cleanup := func() {
		cancel()
		a.scanMutex.Lock()
		if a.scanCtx == scanCtx {
			a.scanCancel = nil
			a.scanCtx = nil
		}
		a.scanMutex.Unlock()
	}
	return scanCtx, cleanup
}

func (a *App) StartFullPipelineScan(cfg config.ScanConfig) ([]config.DialogueItem, error) {
	if cfg.Threshold == 0 {
		cfg.Threshold = 240
	}

	scanCtx, cleanup := a.createScanContext()
	defer cleanup()

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
	scanCtx, cleanup := a.createScanContext()
	defer cleanup()

	onProgress := func(curr, total int, status string) {
		runtime.EventsEmit(a.ctx, "scan-progress", ScanProgress{
			CurrentFrame: curr,
			TotalFrames:  total,
			FoundCount:   curr,
			Status:       status,
		})
	}

	return translation.FilterTrashDialogues(scanCtx, apiKey, dialogues, onProgress)
}

func (a *App) AnalyzeContextAndGlossary(apiKey string, dialogues []config.DialogueItem) (config.TranslationContextConfig, error) {
	scanCtx, cleanup := a.createScanContext()
	defer cleanup()

	onProgress := func(curr, total int, status string) {
		runtime.EventsEmit(a.ctx, "scan-progress", ScanProgress{
			CurrentFrame: curr,
			TotalFrames:  total,
			FoundCount:   curr,
			Status:       status,
		})
	}

	return translation.AnalyzeStoryContextAndGlossary(scanCtx, apiKey, dialogues, onProgress)
}

func (a *App) BulkReplacePronouns(dialogues []config.DialogueItem, replacements []config.GlossaryEntry) []config.DialogueItem {
	return translation.BulkReplacePronouns(dialogues, replacements)
}

func (a *App) buildGlossaryPrompt(contextJSON string) string {
	if strings.TrimSpace(contextJSON) == "" {
		return ""
	}
	var ctxConfig config.TranslationContextConfig
	var oldEntries []config.GlossaryEntry

	if err := json.Unmarshal([]byte(contextJSON), &ctxConfig); err == nil && (ctxConfig.PresetStyle != "" || ctxConfig.PronounRules != "" || len(ctxConfig.CharacterPairs) > 0 || len(ctxConfig.GlossaryEntries) > 0) {
		var sb strings.Builder
		sb.WriteString("\n\nQUY TẮC XƯNG HÔ & THUẬT NGỮ CỐ ĐỊNH BẮT BUỘC TUÂN THỦ (MANDATORY LOCALIZATION RULES):\n")
		if ctxConfig.PresetStyle != "" {
			sb.WriteString(fmt.Sprintf("- Phong cách xưng hô chủ đạo: %s\n", ctxConfig.PresetStyle))
		}
		if len(ctxConfig.CharacterPairs) > 0 {
			sb.WriteString("- Xưng hô nhân vật:\n")
			for _, cp := range ctxConfig.CharacterPairs {
				if cp.Source != "" && cp.Target != "" {
					sb.WriteString(fmt.Sprintf("  + %s => Xưng hô: %s\n", cp.Source, cp.Target))
				}
			}
		}
		if len(ctxConfig.GlossaryEntries) > 0 {
			sb.WriteString("- Bảng tên riêng & thuật ngữ cố định:\n")
			for _, ge := range ctxConfig.GlossaryEntries {
				if ge.Source != "" && ge.Target != "" {
					sb.WriteString(fmt.Sprintf("  + \"%s\" => \"%s\"\n", ge.Source, ge.Target))
				}
			}
		}
		if strings.TrimSpace(ctxConfig.PronounRules) != "" {
			sb.WriteString(fmt.Sprintf("- Chỉ đạo xưng hô đặc biệt: %s\n", strings.TrimSpace(ctxConfig.PronounRules)))
		}
		return sb.String()
	} else if errOld := json.Unmarshal([]byte(contextJSON), &oldEntries); errOld == nil && len(oldEntries) > 0 {
		var sb strings.Builder
		sb.WriteString("\n\nBẢNG XƯNG HÔ & TÊN RIÊNG BẮT BUỘC TUÂN THỦ:\n")
		for _, e := range oldEntries {
			sb.WriteString(fmt.Sprintf("- \"%s\" => \"%s\"\n", e.Source, e.Target))
		}
		return sb.String()
	}
	return ""
}

func (a *App) TranslateDialogues(apiKey string, dialogues []config.DialogueItem, contextJSON string) ([]config.DialogueItem, error) {
	if len(dialogues) == 0 {
		return dialogues, nil
	}

	scanCtx, cleanup := a.createScanContext()
	defer cleanup()

	glossaryPrompt := a.buildGlossaryPrompt(contextJSON)

	onProgress := func(curr, total int, status string) {
		runtime.EventsEmit(a.ctx, "scan-progress", ScanProgress{
			CurrentFrame: curr,
			TotalFrames:  total,
			FoundCount:   curr,
			Status:       status,
		})
	}

	// Pass 1: Translate dialogues with context windows
	translated, err := translation.TranslateDialoguesBatch(scanCtx, apiKey, dialogues, glossaryPrompt, onProgress)
	if err != nil {
		return dialogues, err
	}

	// Pass 2: Re-read full translated script and polish/refine for smooth, continuous flow
	polished, errPolish := translation.RefineFullScriptCoherence(scanCtx, apiKey, translated, glossaryPrompt, onProgress)
	if errPolish == nil && len(polished) > 0 {
		return polished, nil
	}

	return translated, nil
}

func (a *App) RefineFullScriptTranslation(apiKey string, dialogues []config.DialogueItem, contextJSON string) ([]config.DialogueItem, error) {
	if len(dialogues) == 0 {
		return dialogues, nil
	}

	scanCtx, cleanup := a.createScanContext()
	defer cleanup()

	glossaryPrompt := a.buildGlossaryPrompt(contextJSON)

	onProgress := func(curr, total int, status string) {
		runtime.EventsEmit(a.ctx, "scan-progress", ScanProgress{
			CurrentFrame: curr,
			TotalFrames:  total,
			FoundCount:   curr,
			Status:       status,
		})
	}

	return translation.RefineFullScriptCoherence(scanCtx, apiKey, dialogues, glossaryPrompt, onProgress)
}

func (a *App) TranslateSingleDialogue(apiKey string, dialogue config.DialogueItem, contextJSON string) (config.DialogueItem, error) {
	if strings.TrimSpace(dialogue.Original) == "" {
		return dialogue, nil
	}

	scanCtx, cleanup := a.createScanContext()
	defer cleanup()

	glossaryPrompt := ""
	if strings.TrimSpace(contextJSON) != "" {
		var ctxConfig config.TranslationContextConfig
		if err := json.Unmarshal([]byte(contextJSON), &ctxConfig); err == nil {
			var sb strings.Builder
			sb.WriteString("\n\nQUY TẮC XƯNG HÔ & THUẬT NGỮ CỐ ĐỊNH BẮT BUỘC TUÂN THỦ:\n")
			if ctxConfig.PresetStyle != "" {
				sb.WriteString(fmt.Sprintf("- Phong cách xưng hô chủ đạo: %s\n", ctxConfig.PresetStyle))
			}
			if len(ctxConfig.CharacterPairs) > 0 {
				sb.WriteString("- Xưng hô nhân vật:\n")
				for _, cp := range ctxConfig.CharacterPairs {
					if cp.Source != "" && cp.Target != "" {
						sb.WriteString(fmt.Sprintf("  + %s => %s\n", cp.Source, cp.Target))
					}
				}
			}
			if len(ctxConfig.GlossaryEntries) > 0 {
				sb.WriteString("- Bảng tên riêng:\n")
				for _, ge := range ctxConfig.GlossaryEntries {
					if ge.Source != "" && ge.Target != "" {
						sb.WriteString(fmt.Sprintf("  + \"%s\" => \"%s\"\n", ge.Source, ge.Target))
					}
				}
			}
			if strings.TrimSpace(ctxConfig.PronounRules) != "" {
				sb.WriteString(fmt.Sprintf("- Quy tắc đặc biệt: %s\n", strings.TrimSpace(ctxConfig.PronounRules)))
			}
			glossaryPrompt = sb.String()
		}
	}

	translatedText, err := translation.TranslateSingleDialogueItem(scanCtx, apiKey, dialogue.Original, glossaryPrompt)
	if err != nil {
		return dialogue, err
	}

	dialogue.Translated = translatedText
	dialogue.TranslationFailed = false
	return dialogue, nil
}

func (a *App) ExportVideo(videoPath string, dialoguesJSON string, targetOutputPath string) (string, error) {
	return export.ExportVideoWithOptions(videoPath, dialoguesJSON, targetOutputPath, "bubble")
}

func (a *App) ExportVideoWithOptions(videoPath string, dialoguesJSON string, targetOutputPath string, subtitleStyle string) (string, error) {
	return export.ExportVideoWithOptions(videoPath, dialoguesJSON, targetOutputPath, subtitleStyle)
}

func (a *App) ExportSRT(dialoguesJSON string, defaultName string) (string, error) {
	if defaultName == "" {
		defaultName = "subtitles_vietsub.srt"
	}
	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Chọn vị trí lưu file phụ đề SRT",
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{
				DisplayName: "SubRip Subtitles (*.srt)",
				Pattern:     "*.srt",
			},
		},
	})
	if err != nil || savePath == "" {
		return "", err
	}
	return export.ExportSRT(dialoguesJSON, savePath)
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
	pipeline.SetHideWindow(cmd)
	if err := cmd.Run(); err != nil {
		return dialogue, fmt.Errorf("lỗi trích xuất frame: %v", err)
	}

	w, h := pipeline.GetVideoDimensions(videoPath)
	if w <= 0 {
		w = 1920
	}
	if h <= 0 {
		h = 1080
	}

	// === AI PATH: Gemini Vision BBox ===
	if strings.TrimSpace(apiKey) != "" {
		scanCtx, cleanup := a.createScanContext()
		defer cleanup()

		imageBytes, readErr := os.ReadFile(framePath)
		if readErr == nil && len(imageBytes) > 0 {
			visionResult, visionErr := translation.CallGeminiVisionBBox(scanCtx, apiKey, imageBytes, dialogue.Original, w, h)
			if visionErr == nil && visionResult.Found {
				dialogue.BBox = config.BBox{
					X:      math.Max(0, visionResult.X),
					Y:      math.Max(0, visionResult.Y),
					Width:  math.Min(float64(w)-visionResult.X, visionResult.Width),
					Height: math.Min(float64(h)-visionResult.Y, visionResult.Height),
				}
				return dialogue, nil
			}
		}
	}

	// === FALLBACK: RapidOCR text-based matching ===
	reqPaths := []string{framePath}
	resMap, ocrErr := ocr.CallPythonOCRFullFrameBatch(a.ctx, reqPaths, "chi_sim")
	if ocrErr == nil {
		if items, ok := resMap[framePath]; ok && len(items) > 0 {
			mergedItems := ocr.MergeVerticallyAdjacentLines(items)
			centerCurrentX := dialogue.BBox.X + dialogue.BBox.Width/2.0
			centerCurrentY := dialogue.BBox.Y + dialogue.BBox.Height/2.0

			minDist := 1e9
			bestSim := 0.0
			var matchedItem *ocr.OCRFullFrameItem

			for i := range mergedItems {
				itemText := mergedItems[i].Text
				sim := translation.LevenshteinSimilarity(dialogue.Original, itemText)

				itemX := float64(mergedItems[i].BBox[0]) + float64(mergedItems[i].BBox[2])/2.0
				itemY := float64(mergedItems[i].BBox[1]) + float64(mergedItems[i].BBox[3])/2.0
				dist := (itemX-centerCurrentX)*(itemX-centerCurrentX) + (itemY-centerCurrentY)*(itemY-centerCurrentY)

				if sim > bestSim {
					bestSim = sim
					minDist = dist
					matchedItem = &mergedItems[i]
				} else if math.Abs(sim-bestSim) < 0.05 && dist < minDist {
					minDist = dist
					matchedItem = &mergedItems[i]
				}
			}

			if matchedItem != nil && (bestSim >= 0.35 || len(mergedItems) == 1) {
				textBBox := config.BBox{
					X:      float64(matchedItem.BBox[0]),
					Y:      float64(matchedItem.BBox[1]),
					Width:  float64(matchedItem.BBox[2]),
					Height: float64(matchedItem.BBox[3]),
				}
				padY := math.Max(4.0, textBBox.Height*0.06)
				padX := math.Max(12.0, textBBox.Width*0.12)
				newX := math.Max(0, textBBox.X-padX)
				newY := math.Max(0, textBBox.Y-padY)
				newW := math.Min(float64(w)-newX, textBBox.Width+2*padX)
				newH := math.Min(float64(h)-newY, textBBox.Height+2*padY)
				dialogue.BBox = config.BBox{
					X:      newX,
					Y:      newY,
					Width:  newW,
					Height: newH,
				}
				return dialogue, nil
			}
		}
	}

	return dialogue, nil
}

func (a *App) SnapAllDialoguesBBoxes(videoPath string, dialogues []config.DialogueItem, apiKey string) ([]config.DialogueItem, error) {
	if videoPath == "" || len(dialogues) == 0 {
		return dialogues, nil
	}

	scanCtx, cleanup := a.createScanContext()
	defer cleanup()

	useAI := strings.TrimSpace(apiKey) != ""

	w, h := pipeline.GetVideoDimensions(videoPath)
	if w <= 0 {
		w = 1920
	}
	if h <= 0 {
		h = 1080
	}

	tempDir, err := os.MkdirTemp("", "comicify_snap_all_*")
	if err != nil {
		return dialogues, fmt.Errorf("lỗi tạo thư mục tạm: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Step 1: Filter and collect task list
	type frameTask struct {
		index     int
		framePath string
	}
	var tasks []frameTask
	for i := range dialogues {
		if !dialogues[i].IsTrash && dialogues[i].Original != "" {
			fPath := filepath.Join(tempDir, fmt.Sprintf("frame_%d.jpg", i))
			tasks = append(tasks, frameTask{index: i, framePath: fPath})
		}
	}

	if len(tasks) == 0 {
		return dialogues, nil
	}

	// Run parallel ffmpeg extraction (max 6 workers)
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for _, t := range tasks {
		select {
		case <-scanCtx.Done():
			return dialogues, scanCtx.Err()
		default:
		}

		wg.Add(1)
		go func(task frameTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			d := dialogues[task.index]
			midTime := d.Timestamp
			if d.EndTimestamp > d.Timestamp {
				midTime = (d.Timestamp + d.EndTimestamp) / 2.0
			} else if d.Duration > 0 {
				midTime = d.Timestamp + (d.Duration / 2.0)
			}

			cmd := exec.Command("ffmpeg", "-y", "-ss", fmt.Sprintf("%.3f", midTime), "-i", videoPath, "-vframes", "1", "-q:v", "2", task.framePath)
			pipeline.SetHideWindow(cmd)
			_ = cmd.Run()
		}(t)
	}
	wg.Wait()

	// Step 2: Snap processing (AI or collect for OCR Batch)
	needOCR := make([]frameTask, 0, len(tasks))

	for idx, task := range tasks {
		select {
		case <-scanCtx.Done():
			return dialogues, scanCtx.Err()
		default:
		}

		i := task.index
		method := "OCR"
		if useAI {
			method = "AI"
		}
		runtime.EventsEmit(a.ctx, "scan-progress", ScanProgress{
			CurrentFrame: idx + 1,
			TotalFrames:  len(tasks),
			FoundCount:   idx + 1,
			Status:       fmt.Sprintf("🤖 [%s] Đang tự động căn vị trí hộp thoại đè thoại gốc: %d/%d (%.0f%%)...", method, idx+1, len(tasks), float64(idx+1)/float64(len(tasks))*100),
		})

		snappedByAI := false
		if useAI {
			imageBytes, readErr := os.ReadFile(task.framePath)
			if readErr == nil && len(imageBytes) > 0 {
				visionResult, visionErr := translation.CallGeminiVisionBBox(scanCtx, apiKey, imageBytes, dialogues[i].Original, w, h)
				if visionErr == nil && visionResult.Found {
					dialogues[i].BBox = config.BBox{
						X:      math.Max(0, visionResult.X),
						Y:      math.Max(0, visionResult.Y),
						Width:  math.Min(float64(w)-visionResult.X, visionResult.Width),
						Height: math.Min(float64(h)-visionResult.Y, visionResult.Height),
					}
					snappedByAI = true
				}
			}
		}

		if !snappedByAI {
			needOCR = append(needOCR, task)
		}
	}

	// Step 3: Fast Batch OCR for all items that didn't get snapped by AI
	if len(needOCR) > 0 {
		select {
		case <-scanCtx.Done():
			return dialogues, scanCtx.Err()
		default:
		}

		ocrPaths := make([]string, len(needOCR))
		for k, task := range needOCR {
			ocrPaths[k] = task.framePath
		}

		resMap, ocrErr := ocr.CallPythonOCRFullFrameBatch(scanCtx, ocrPaths, "chi_sim")
		if ocrErr == nil {
			for _, task := range needOCR {
				i := task.index
				if items, ok := resMap[task.framePath]; ok && len(items) > 0 {
					mergedItems := ocr.MergeVerticallyAdjacentLines(items)
					centerCurrentX := dialogues[i].BBox.X + dialogues[i].BBox.Width/2.0
					centerCurrentY := dialogues[i].BBox.Y + dialogues[i].BBox.Height/2.0

					minDist := 1e9
					bestSim := 0.0
					var matchedItem *ocr.OCRFullFrameItem

					for j := range mergedItems {
						itemText := mergedItems[j].Text
						sim := translation.LevenshteinSimilarity(dialogues[i].Original, itemText)

						itemX := float64(mergedItems[j].BBox[0]) + float64(mergedItems[j].BBox[2])/2.0
						itemY := float64(mergedItems[j].BBox[1]) + float64(mergedItems[j].BBox[3])/2.0
						dist := (itemX-centerCurrentX)*(itemX-centerCurrentX) + (itemY-centerCurrentY)*(itemY-centerCurrentY)

						if sim > bestSim {
							bestSim = sim
							minDist = dist
							matchedItem = &mergedItems[j]
						} else if math.Abs(sim-bestSim) < 0.05 && dist < minDist {
							minDist = dist
							matchedItem = &mergedItems[j]
						}
					}

					if matchedItem != nil && (bestSim >= 0.35 || len(mergedItems) == 1) {
						textBBox := config.BBox{
							X:      float64(matchedItem.BBox[0]),
							Y:      float64(matchedItem.BBox[1]),
							Width:  float64(matchedItem.BBox[2]),
							Height: float64(matchedItem.BBox[3]),
						}
						padY := math.Max(4.0, textBBox.Height*0.06)
						padX := math.Max(12.0, textBBox.Width*0.12)
						newX := math.Max(0, textBBox.X-padX)
						newY := math.Max(0, textBBox.Y-padY)
						newW := math.Min(float64(w)-newX, textBBox.Width+2*padX)
						newH := math.Min(float64(h)-newY, textBBox.Height+2*padY)

						dialogues[i].BBox = config.BBox{
							X:      newX,
							Y:      newY,
							Width:  newW,
							Height: newH,
						}
					}
				}
			}
		}
	}

	return dialogues, nil
}

func (a *App) RefineDialogueTimestamp(videoPath string, dialogue config.DialogueItem, interval float64) (config.DialogueItem, error) {
	return pipeline.RefineSingleDialogueBoundary(a.ctx, videoPath, dialogue, interval)
}

func (a *App) RefineAllDialoguesTimestamps(videoPath string, dialogues []config.DialogueItem, interval float64) ([]config.DialogueItem, error) {
	scanCtx, cleanup := a.createScanContext()
	defer cleanup()

	onProgress := func(curr, total, found int, status string) {
		runtime.EventsEmit(a.ctx, "scan-progress", ScanProgress{
			CurrentFrame: curr,
			TotalFrames:  total,
			FoundCount:   found,
			Status:       status,
		})
	}

	return pipeline.RefineDialoguesBoundaries(scanCtx, videoPath, dialogues, interval, onProgress)
}
