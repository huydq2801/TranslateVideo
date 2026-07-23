package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"

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
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("lỗi trích xuất khung hình FFmpeg: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(outDir, "frame_*.jpg"))
	if err != nil {
		return 0, err
	}
	return len(files), nil
}

func ScanFullFramePipeline(ctx context.Context, cfg config.ScanConfig, onProgress ProgressCallback) ([]config.DialogueItem, error) {
	if cfg.VideoPath == "" {
		return nil, fmt.Errorf("chưa chọn file video")
	}

	cachePath := config.GetCachePath(cfg.VideoPath)
	if cache, err := config.LoadCache(cachePath); err == nil {
		if config.ConfigMatches(cache.Config, cfg) && len(cache.Dialogues) > 0 {
			if onProgress != nil {
				onProgress(cache.Total, cache.Total, len(cache.Dialogues), "Tải danh sách thoại từ bộ nhớ đệm (Cache)")
			}
			return cache.Dialogues, nil
		}
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
	type BatchJob struct {
		Paths      []string
		Indices    []int
		Timestamps []float64
	}

	var jobs []BatchJob
	for i := 0; i < len(files); i += batchSize {
		end := i + batchSize
		if end > len(files) {
			end = len(files)
		}
		subPaths := files[i:end]
		indices := make([]int, len(subPaths))
		timestamps := make([]float64, len(subPaths))

		for k, p := range subPaths {
			base := filepath.Base(p)
			numStr := strings.TrimSuffix(strings.TrimPrefix(base, "frame_"), ".jpg")
			idx, _ := strconv.Atoi(numStr)
			indices[k] = idx
			timestamps[k] = float64(idx-1) * cfg.Interval
		}
		jobs = append(jobs, BatchJob{Paths: subPaths, Indices: indices, Timestamps: timestamps})
	}

	var candidates []config.CandidateItem
	var candMutex sync.Mutex

	for jobIdx, job := range jobs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resMap, ocrErr := ocr.CallPythonOCRFullFrameBatch(ctx, job.Paths, cfg.SourceLang)
		if ocrErr != nil {
			continue
		}

		for k, p := range job.Paths {
			items, ok := resMap[p]
			if !ok || len(items) == 0 {
				continue
			}

			merged := ocr.MergeVerticallyAdjacentLines(items)
			timeSec := job.Timestamps[k]
			fIdx := job.Indices[k]

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
				candMutex.Lock()
				candidates = append(candidates, config.CandidateItem{
					FrameIndex: fIdx,
					TimeSec:    timeSec,
					Original:   m.Text,
					BBox:       bbox,
				})
				candCount := len(candidates)
				candMutex.Unlock()

				if onProgress != nil {
					status := fmt.Sprintf("[1/2] Quét Full-Frame [%s]: %s", config.FormatTimeSec(timeSec), m.Text)
					onProgress((jobIdx+1)*batchSize, totalFrames, candCount, status)
				}
			}
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
		RefineStartBoundaryAccurate(cfg.VideoPath, &dialogues[i], cfg.Interval, cfg.Threshold, tempDir)
		RefineEndBoundaryAccurate(cfg.VideoPath, &dialogues[i], cfg.Interval, cfg.Threshold, tempDir)
	}

	sort.Slice(dialogues, func(i, j int) bool {
		return dialogues[i].Timestamp < dialogues[j].Timestamp
	})

	for i := range dialogues {
		dialogues[i].ID = i + 1
	}

	config.SaveCache(cachePath, cfg, dialogues, totalFrames, totalFrames)

	runtime.GC()
	debug.FreeOSMemory()

	return dialogues, nil
}
