package pipeline

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"translate-video/pkg/config"
	"translate-video/pkg/ocr"
	"translate-video/pkg/translation"
)

type ProgressCallback func(current, total, found int, status string)

func GetVideoDuration(videoPath string) float64 {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprintwrappers=1:nokey=1", videoPath)
	SetHideWindow(cmd)
	out, err := cmd.Output()
	if err == nil {
		dur, errParse := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		if errParse == nil && dur > 0 {
			return dur
		}
	}
	return 0
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

func ExtractFramesWithFFmpeg(ctx context.Context, videoPath string, interval float64, videoType string, outDir string, onProgress ProgressCallback) (int, error) {
	if interval <= 0 {
		interval = 0.30
	}
	outPattern := filepath.Join(outDir, "frame_%06d.jpg")

	filterStr := fmt.Sprintf("fps=1/%.3f,scale='min(iw,1080)':-1", interval)
	if videoType == "sims_en" {
		// Crop bottom 28% ROI for fixed bottom subtitles (y: 72% -> 100%)
		filterStr = fmt.Sprintf("fps=1/%.3f,crop=iw:ih*0.28:0:ih*0.72,scale='min(iw,1080)':-1", interval)
	}

	duration := GetVideoDuration(videoPath)
	estimatedTotal := 0
	if duration > 0 {
		estimatedTotal = int(math.Ceil(duration / interval))
	}
	if estimatedTotal <= 0 {
		estimatedTotal = 100
	}

	if onProgress != nil {
		onProgress(0, estimatedTotal, 0, fmt.Sprintf("[1/2] Đang trích xuất khung hình qua GPU: 0/%d (0%%)...", estimatedTotal))
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-nostdin", "-threads", "4", "-i", videoPath, "-vf", filterStr, "-q:v", "2", outPattern)
	SetHideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("lỗi khởi chạy FFmpeg: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			files, _ := filepath.Glob(filepath.Join(outDir, "frame_*.jpg"))
			if len(files) > 0 {
				if onProgress != nil {
					onProgress(len(files), len(files), 0, fmt.Sprintf("[1/2] Đã trích xuất %d khung hình!", len(files)))
				}
				return len(files), nil
			}
			if err != nil {
				return 0, fmt.Errorf("lỗi FFmpeg: %v", err)
			}
			return 0, fmt.Errorf("không tìm thấy khung hình nào sau khi chạy FFmpeg")

		case <-ticker.C:
			files, _ := filepath.Glob(filepath.Join(outDir, "frame_*.jpg"))
			curr := len(files)
			tot := estimatedTotal
			if curr > tot {
				tot = curr + 5
			}
			if onProgress != nil && tot > 0 {
				pct := math.Min(99, math.Round((float64(curr)/float64(tot))*100))
				onProgress(curr, tot, 0, fmt.Sprintf("[1/2] Đang trích xuất khung hình qua GPU: %d/%d (%.0f%%)...", curr, tot, pct))
			}

		case <-ctx.Done():
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			return 0, ctx.Err()
		}
	}
}

func SpatialIoU(b1, b2 config.BBox) float64 {
	left := math.Max(b1.X, b2.X)
	top := math.Max(b1.Y, b2.Y)
	right := math.Min(b1.X+b1.Width, b2.X+b2.Width)
	bottom := math.Min(b1.Y+b1.Height, b2.Y+b2.Height)

	if right <= left || bottom <= top {
		return 0.0
	}

	intersection := (right - left) * (bottom - top)
	area1 := b1.Width * b1.Height
	area2 := b2.Width * b2.Height
	union := area1 + area2 - intersection

	if union <= 0 {
		return 0.0
	}
	return intersection / union
}

func ClusterFrameTimestamps(candidates []config.CandidateItem, sampleInterval float64) []config.DialogueItem {
	if len(candidates) == 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].TimeSec < candidates[j].TimeSec
	})

	maxGapTolerance := math.Max(0.60, sampleInterval*1.5)

	type Cluster struct {
		Candidates []config.CandidateItem
		StartTime  float64
		EndTime    float64
	}

	var clusters []*Cluster

	for _, cand := range candidates {
		normCandText := translation.NormalizeOCRText(cand.Original)
		if normCandText == "" {
			continue
		}

		matched := false
		for i := len(clusters) - 1; i >= 0; i-- {
			cls := clusters[i]
			lastCand := cls.Candidates[len(cls.Candidates)-1]
			timeDiff := cand.TimeSec - cls.EndTime

			if timeDiff > maxGapTolerance+1e-4 {
				break
			}

			sim := translation.LevenshteinSimilarity(cand.Original, lastCand.Original)
			overlap := SpatialIoU(cand.BBox, lastCand.BBox) >= 0.15

			if sim >= 0.75 && overlap {
				cls.Candidates = append(cls.Candidates, cand)
				cls.EndTime = cand.TimeSec
				matched = true
				break
			}
		}

		if !matched {
			clusters = append(clusters, &Cluster{
				Candidates: []config.CandidateItem{cand},
				StartTime:  cand.TimeSec,
				EndTime:    cand.TimeSec,
			})
		}
	}

	var dialogues []config.DialogueItem
	for idx, cls := range clusters {
		var sumX, sumY, sumW, sumH float64
		bestText := cls.Candidates[0].Original
		for _, c := range cls.Candidates {
			sumX += c.BBox.X
			sumY += c.BBox.Y
			sumW += c.BBox.Width
			sumH += c.BBox.Height
			if len(c.Original) > len(bestText) {
				bestText = c.Original
			}
		}
		n := float64(len(cls.Candidates))
		avgBBox := config.BBox{
			X:      sumX / n,
			Y:      sumY / n,
			Width:  sumW / n,
			Height: sumH / n,
		}

		padY := math.Max(4.0, avgBBox.Height*0.05)
		padX := math.Max(12.0, avgBBox.Width*0.12)

		expandedBBox := config.BBox{
			X:      math.Max(0, avgBBox.X-padX),
			Y:      math.Max(0, avgBBox.Y-padY),
			Width:  avgBBox.Width + 2*padX,
			Height: avgBBox.Height + 2*padY,
		}

		startTime := math.Max(0.0, cls.StartTime-0.30)
		endTime := cls.EndTime + sampleInterval + 0.30
		duration := math.Max(0.50, endTime-startTime)

		dialogues = append(dialogues, config.DialogueItem{
			ID:           idx + 1,
			FrameIndex:   cls.Candidates[0].FrameIndex,
			Timestamp:    startTime,
			EndTimestamp: endTime,
			Duration:     duration,
			Original:     bestText,
			BBox:         expandedBBox,
			IsTrash:      translation.HeuristicIsTrash(bestText),
		})
	}

	return dialogues
}

func RefineStartBoundaryAccurate(ctx context.Context, videoPath string, d *config.DialogueItem, sampleInterval float64, tempDir string) {
	winStart := math.Max(0.0, d.Timestamp-sampleInterval)
	winEnd := d.Timestamp + 0.15
	if winEnd-winStart <= 0.03 {
		return
	}

	startTempDir, err := os.MkdirTemp(tempDir, "refine_start_*")
	if err != nil {
		return
	}
	defer os.RemoveAll(startTempDir)

	origW, origH := GetVideoDimensions(videoPath)
	cX := max(0, int(d.BBox.X)-30)
	cY := max(0, int(d.BBox.Y)-30)
	cW := int(d.BBox.Width) + 60
	cH := int(d.BBox.Height) + 60
	if cX+cW > origW {
		cW = origW - cX
	}
	if cY+cH > origH {
		cH = origH - cY
	}
	if cW <= 20 || cH <= 20 {
		return
	}

	dur := winEnd - winStart
	outPattern := filepath.Join(startTempDir, "f_%03d.jpg")
	filterStr := fmt.Sprintf("crop=%d:%d:%d:%d,fps=30", cW, cH, cX, cY)

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-nostdin", "-ss", fmt.Sprintf("%.3f", winStart), "-i", videoPath, "-t", fmt.Sprintf("%.3f", dur), "-vf", filterStr, "-q:v", "2", outPattern)
	SetHideWindow(cmd)
	if err := cmd.Run(); err != nil {
		return
	}

	files, _ := filepath.Glob(filepath.Join(startTempDir, "f_*.jpg"))
	if len(files) == 0 {
		return
	}
	sort.Strings(files)

	lang := "chi_sim"
	if !translation.IsCJK(d.Original) {
		lang = "en"
	}
	resMap, err := ocr.CallPythonOCRFullFrameBatch(ctx, files, lang)
	if err == nil {
		for idx, fPath := range files {
			if items, ok := resMap[fPath]; ok && len(items) > 0 {
				d.Timestamp = winStart + float64(idx)/30.0
				break
			}
		}
	}
}

func RefineEndBoundaryAccurate(ctx context.Context, videoPath string, d *config.DialogueItem, sampleInterval float64, tempDir string) {
	winStart := math.Max(d.Timestamp, d.EndTimestamp-sampleInterval)
	winEnd := d.EndTimestamp + sampleInterval
	if winEnd-winStart <= 0.03 {
		return
	}

	endTempDir, err := os.MkdirTemp(tempDir, "refine_end_*")
	if err != nil {
		return
	}
	defer os.RemoveAll(endTempDir)

	origW, origH := GetVideoDimensions(videoPath)
	cX := max(0, int(d.BBox.X)-30)
	cY := max(0, int(d.BBox.Y)-30)
	cW := int(d.BBox.Width) + 60
	cH := int(d.BBox.Height) + 60
	if cX+cW > origW {
		cW = origW - cX
	}
	if cY+cH > origH {
		cH = origH - cY
	}
	if cW <= 20 || cH <= 20 {
		return
	}

	dur := winEnd - winStart
	outPattern := filepath.Join(endTempDir, "f_%03d.jpg")
	filterStr := fmt.Sprintf("crop=%d:%d:%d:%d,fps=30", cW, cH, cX, cY)

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-nostdin", "-ss", fmt.Sprintf("%.3f", winStart), "-i", videoPath, "-t", fmt.Sprintf("%.3f", dur), "-vf", filterStr, "-q:v", "2", outPattern)
	SetHideWindow(cmd)
	if err := cmd.Run(); err != nil {
		return
	}

	files, _ := filepath.Glob(filepath.Join(endTempDir, "f_*.jpg"))
	if len(files) == 0 {
		return
	}
	sort.Strings(files)

	lang := "chi_sim"
	if !translation.IsCJK(d.Original) {
		lang = "en"
	}
	resMap, err := ocr.CallPythonOCRFullFrameBatch(ctx, files, lang)
	if err == nil {
		for idx := len(files) - 1; idx >= 0; idx-- {
			fPath := files[idx]
			if items, ok := resMap[fPath]; ok && len(items) > 0 {
				d.EndTimestamp = winStart + float64(idx)/30.0
				d.Duration = math.Max(0.2, d.EndTimestamp-d.Timestamp)
				break
			}
		}
	}
}

func ScanFullFramePipeline(ctx context.Context, cfg config.ScanConfig, onProgress ProgressCallback) ([]config.DialogueItem, error) {
	if cfg.VideoPath == "" {
		return nil, fmt.Errorf("chưa chọn file video")
	}

	// Pre-flight check: ensure Python OCR sidecar server (127.0.0.1:8000) is running
	client := &http.Client{Timeout: 2 * time.Second}
	resp, healthErr := client.Get("http://127.0.0.1:8000/health")
	if healthErr != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, fmt.Errorf("Server Python OCR chưa hoạt động tại http://127.0.0.1:8000.\nVui lòng mở terminal và chạy lệnh: python python_ocr/server.py")
	}
	resp.Body.Close()

	tempDir, err := os.MkdirTemp("", "comicify_fullframe_*")
	if err != nil {
		return nil, fmt.Errorf("lỗi tạo thư mục tạm: %v", err)
	}
	defer os.RemoveAll(tempDir)

	totalFrames, err := ExtractFramesWithFFmpeg(ctx, cfg.VideoPath, cfg.Interval, cfg.VideoType, tempDir, onProgress)
	if err != nil || totalFrames == 0 {
		return nil, fmt.Errorf("không trích xuất được khung hình nào từ video")
	}

	files, err := filepath.Glob(filepath.Join(tempDir, "frame_*.jpg"))
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("không tìm thấy file frame JPEG trong thư mục tạm")
	}
	sort.Strings(files)

	origW, origH := GetVideoDimensions(cfg.VideoPath)
	scaleRatio := 1.0
	if origW > 1080 {
		scaleRatio = float64(origW) / 1080.0
	}

	// If Sims English mode, calculate the vertical offset (crop begins at y: 72%)
	yOffset := 0.0
	sourceLang := cfg.SourceLang
	if cfg.VideoType == "sims_en" {
		yOffset = float64(origH) * 0.72
		if sourceLang == "" || sourceLang == "chi_sim" {
			sourceLang = "en"
		}
	}

	batchSize := 200
	var candidates []config.CandidateItem
	consecutiveErrors := 0

	for i := 0; i < len(files); i += batchSize {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		end := i + batchSize
		if end > len(files) {
			end = len(files)
		}
		subPaths := files[i:end]

		resMap, ocrErr := ocr.CallPythonOCRFullFrameBatch(ctx, subPaths, sourceLang)
		if ocrErr != nil {
			consecutiveErrors++
			if consecutiveErrors >= 5 {
				return nil, fmt.Errorf("Lỗi kết nối Python OCR (127.0.0.1:8000): %v.\nVui lòng kiểm tra lại Python server", ocrErr)
			}
		} else {
			consecutiveErrors = 0
			for _, p := range subPaths {
				items, ok := resMap[p]
				if ok && len(items) > 0 {
					base := filepath.Base(p)
					numStr := strings.TrimSuffix(strings.TrimPrefix(base, "frame_"), ".jpg")
					fIdx, _ := strconv.Atoi(numStr)
					timeSec := float64(fIdx-1) * cfg.Interval

					merged := ocr.MergeVerticallyAdjacentLines(items)
					for _, m := range merged {
						norm := translation.NormalizeOCRText(m.Text)
						if norm == "" {
							continue
						}
						bbox := config.BBox{
							X:      float64(m.BBox[0]) * scaleRatio,
							Y:      (float64(m.BBox[1]) * scaleRatio) + yOffset,
							Width:  float64(m.BBox[2]) * scaleRatio,
							Height: float64(m.BBox[3]) * scaleRatio,
						}
						candidates = append(candidates, config.CandidateItem{
							FrameIndex: fIdx,
							TimeSec:    timeSec,
							Original:   m.Text,
							BBox:       bbox,
						})
					}
				}
			}
		}

		// Always clean up processed batch image files immediately to prevent disk exhaustion
		for _, p := range subPaths {
			_ = os.Remove(p)
		}

		if onProgress != nil {
			pct := math.Min(99, math.Round((float64(end)/float64(totalFrames))*100))
			onProgress(end, totalFrames, len(candidates), fmt.Sprintf("[1/2] Quét DirectML GPU OCR: %d/%d khung hình (%.0f%% - tìm thấy %d thoại)...", end, totalFrames, pct, len(candidates)))
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("Không tìm thấy câu thoại nào trong %d khung hình video", totalFrames)
	}

	dialogues := ClusterFrameTimestamps(candidates, cfg.Interval)

	sort.Slice(dialogues, func(i, j int) bool {
		return dialogues[i].Timestamp < dialogues[j].Timestamp
	})

	for i := range dialogues {
		dialogues[i].ID = i + 1
	}

	if onProgress != nil {
		onProgress(totalFrames, totalFrames, len(dialogues), fmt.Sprintf("Hoàn thành quét thoại! Tìm thấy %d câu thoại.", len(dialogues)))
	}

	runtime.GC()
	debug.FreeOSMemory()

	return dialogues, nil
}

func RefineSingleDialogueBoundary(ctx context.Context, videoPath string, d config.DialogueItem, sampleInterval float64) (config.DialogueItem, error) {
	if sampleInterval <= 0 {
		sampleInterval = 0.30
	}
	tempDir, err := os.MkdirTemp("", "comicify_refine_single_*")
	if err != nil {
		return d, err
	}
	defer os.RemoveAll(tempDir)

	RefineStartBoundaryAccurate(ctx, videoPath, &d, sampleInterval, tempDir)
	RefineEndBoundaryAccurate(ctx, videoPath, &d, sampleInterval, tempDir)
	d.Timestamp = math.Max(0.0, d.Timestamp-0.30)
	d.EndTimestamp = d.EndTimestamp + 0.30
	d.Duration = math.Max(0.50, d.EndTimestamp-d.Timestamp)
	return d, nil
}

func RefineDialoguesBoundaries(ctx context.Context, videoPath string, dialogues []config.DialogueItem, sampleInterval float64, onProgress ProgressCallback) ([]config.DialogueItem, error) {
	if sampleInterval <= 0 {
		sampleInterval = 0.30
	}
	if len(dialogues) == 0 {
		return dialogues, nil
	}

	tempDir, err := os.MkdirTemp("", "comicify_refine_all_*")
	if err != nil {
		return dialogues, err
	}
	defer os.RemoveAll(tempDir)

	totalDialogues := len(dialogues)
	if onProgress != nil {
		onProgress(0, totalDialogues, totalDialogues, fmt.Sprintf("Tinh chỉnh mốc thời gian 30 FPS (0/%d thoại)...", totalDialogues))
	}

	numWorkers := runtime.NumCPU() - 2
	if numWorkers > 6 {
		numWorkers = 6
	}
	if numWorkers > totalDialogues {
		numWorkers = totalDialogues
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	jobs := make(chan int, totalDialogues)
	for i := 0; i < totalDialogues; i++ {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	var completedCount int
	var mu sync.Mutex

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				RefineStartBoundaryAccurate(ctx, videoPath, &dialogues[i], sampleInterval, tempDir)
				RefineEndBoundaryAccurate(ctx, videoPath, &dialogues[i], sampleInterval, tempDir)
				dialogues[i].Duration = dialogues[i].EndTimestamp - dialogues[i].Timestamp

				mu.Lock()
				completedCount++
				currCompleted := completedCount
				if onProgress != nil {
					onProgress(currCompleted, totalDialogues, totalDialogues, fmt.Sprintf("Tinh chỉnh mốc thời gian 30 FPS: %d/%d thoại...", currCompleted, totalDialogues))
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	sort.Slice(dialogues, func(i, j int) bool {
		return dialogues[i].Timestamp < dialogues[j].Timestamp
	})

	for i := range dialogues {
		dialogues[i].ID = i + 1
	}

	runtime.GC()
	debug.FreeOSMemory()

	return dialogues, nil
}
