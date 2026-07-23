package pipeline

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	"translate-video/pkg/config"
	"translate-video/pkg/ocr"
	"translate-video/pkg/translation"
)

type ProgressCallback func(current, total, found int, status string)

func ExtractFramesWithFFmpeg(ctx context.Context, videoPath string, interval float64, outDir string) (int, error) {
	if interval <= 0 {
		interval = 0.30
	}
	outPattern := filepath.Join(outDir, "frame_%05d.jpg")
	filterStr := fmt.Sprintf("fps=1/%.3f", interval)

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-hwaccel", "auto", "-i", videoPath, "-vf", filterStr, "-q:v", "2", outPattern)
	SetHideWindow(cmd)
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("lỗi trích xuất khung hình FFmpeg: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(outDir, "frame_*.jpg"))
	if err != nil {
		return 0, err
	}
	return len(files), nil
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

	const maxGapTolerance = 0.60

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
				continue
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
		for _, c := range cls.Candidates {
			sumX += c.BBox.X
			sumY += c.BBox.Y
			sumW += c.BBox.Width
			sumH += c.BBox.Height
		}
		n := float64(len(cls.Candidates))
		avgBBox := config.BBox{
			X:      sumX / n,
			Y:      sumY / n,
			Width:  sumW / n,
			Height: sumH / n,
		}

		padY := math.Max(10.0, avgBBox.Height*0.15)
		padX := math.Max(15.0, avgBBox.Width*0.08)

		expandedBBox := config.BBox{
			X:      math.Max(0, avgBBox.X-padX),
			Y:      math.Max(0, avgBBox.Y-padY),
			Width:  avgBBox.Width + 2*padX,
			Height: avgBBox.Height + 2*padY,
		}

		dialogues = append(dialogues, config.DialogueItem{
			ID:           idx + 1,
			FrameIndex:   cls.Candidates[0].FrameIndex,
			Timestamp:    cls.StartTime,
			EndTimestamp: cls.EndTime + sampleInterval,
			Duration:     (cls.EndTime + sampleInterval) - cls.StartTime,
			Original:     cls.Candidates[0].Original,
			BBox:         expandedBBox,
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

	dur := winEnd - winStart
	outPattern := filepath.Join(startTempDir, "f_%03d.jpg")
	filterStr := fmt.Sprintf("crop=%d:%d:%d:%d,fps=30", int(d.BBox.Width), int(d.BBox.Height), max(0, int(d.BBox.X)), max(0, int(d.BBox.Y)))

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-ss", fmt.Sprintf("%.3f", winStart), "-i", videoPath, "-t", fmt.Sprintf("%.3f", dur), "-vf", filterStr, "-q:v", "2", outPattern)
	SetHideWindow(cmd)
	if err := cmd.Run(); err != nil {
		return
	}

	files, _ := filepath.Glob(filepath.Join(startTempDir, "f_*.jpg"))
	if len(files) == 0 {
		return
	}
	sort.Strings(files)

	resMap, err := ocr.CallPythonOCRFullFrameBatch(ctx, files, "chi_sim")
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

	dur := winEnd - winStart
	outPattern := filepath.Join(endTempDir, "f_%03d.jpg")
	filterStr := fmt.Sprintf("crop=%d:%d:%d:%d,fps=30", int(d.BBox.Width), int(d.BBox.Height), max(0, int(d.BBox.X)), max(0, int(d.BBox.Y)))

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-ss", fmt.Sprintf("%.3f", winStart), "-i", videoPath, "-t", fmt.Sprintf("%.3f", dur), "-vf", filterStr, "-q:v", "2", outPattern)
	SetHideWindow(cmd)
	if err := cmd.Run(); err != nil {
		return
	}

	files, _ := filepath.Glob(filepath.Join(endTempDir, "f_*.jpg"))
	if len(files) == 0 {
		return
	}
	sort.Strings(files)

	resMap, err := ocr.CallPythonOCRFullFrameBatch(ctx, files, "chi_sim")
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

	tempDir, err := os.MkdirTemp("", "comicify_fullframe_*")
	if err != nil {
		return nil, fmt.Errorf("lỗi tạo thư mục tạm: %v", err)
	}
	defer os.RemoveAll(tempDir)

	if onProgress != nil {
		onProgress(0, 100, 0, "[1/2] Đang trích xuất khung hình từ video qua GPU...")
	}

	totalFrames, err := ExtractFramesWithFFmpeg(ctx, cfg.VideoPath, cfg.Interval, tempDir)
	if err != nil || totalFrames == 0 {
		return nil, fmt.Errorf("không trích xuất được khung hình nào từ video")
	}

	files, err := filepath.Glob(filepath.Join(tempDir, "frame_*.jpg"))
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("không tìm thấy file frame JPEG trong thư mục tạm")
	}
	sort.Strings(files)

	batchSize := 16
	var candidates []config.CandidateItem

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

		resMap, ocrErr := ocr.CallPythonOCRFullFrameBatch(ctx, subPaths, cfg.SourceLang)
		if ocrErr != nil {
			continue
		}

		for _, p := range subPaths {
			items, ok := resMap[p]
			if !ok || len(items) == 0 {
				continue
			}

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
					X:      float64(m.BBox[0]),
					Y:      float64(m.BBox[1]),
					Width:  float64(m.BBox[2]),
					Height: float64(m.BBox[3]),
				}
				candidates = append(candidates, config.CandidateItem{
					FrameIndex: fIdx,
					TimeSec:    timeSec,
					Original:   m.Text,
					BBox:       bbox,
				})
			}
		}

		if onProgress != nil {
			onProgress(end, totalFrames, len(candidates), fmt.Sprintf("Quét Full-Frame: %d/%d khung hình", end, totalFrames))
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	dialogues := ClusterFrameTimestamps(candidates, cfg.Interval)

	if onProgress != nil {
		onProgress(totalFrames, totalFrames, len(dialogues), "[2/2] Tinh chỉnh mốc thời gian 30 FPS...")
	}

	for i := range dialogues {
		RefineStartBoundaryAccurate(ctx, cfg.VideoPath, &dialogues[i], cfg.Interval, tempDir)
		RefineEndBoundaryAccurate(ctx, cfg.VideoPath, &dialogues[i], cfg.Interval, tempDir)
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
