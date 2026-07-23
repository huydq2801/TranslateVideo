package pipeline

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"translate-video/pkg/config"
	"translate-video/pkg/detector"
)

func ExtractCropJPEGsAccurate(videoPath string, bbox config.BBox, winStart, winEnd float64, outDir string) ([]string, error) {
	dur := winEnd - winStart
	if dur <= 0.03 {
		return nil, nil
	}
	w := int(bbox.Width)
	h := int(bbox.Height)
	x := int(bbox.X)
	y := int(bbox.Y)
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	outPattern := filepath.Join(outDir, "f_%03d.jpg")
	filterStr := fmt.Sprintf("crop=%d:%d:%d:%d,fps=30", w, h, x, y)
	cmd := exec.Command("ffmpeg", "-y", "-ss", fmt.Sprintf("%.3f", winStart), "-i", videoPath, "-t", fmt.Sprintf("%.3f", dur), "-vf", filterStr, "-q:v", "2", outPattern)
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	files, _ := filepath.Glob(filepath.Join(outDir, "f_*.jpg"))
	sort.Strings(files)
	return files, nil
}

func RefineStartBoundaryAccurate(videoPath string, d *config.DialogueItem, sampleInterval float64, threshold uint8, tempDir string) {
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

	files, err := ExtractCropJPEGsAccurate(videoPath, d.BBox, winStart, winEnd, startTempDir)
	if err == nil && len(files) > 0 {
		for idx, fPath := range files {
			if detector.ProbeFrameForBubble(fPath, threshold) {
				d.Timestamp = winStart + float64(idx)/30.0
				break
			}
		}
	}
}

func RefineEndBoundaryAccurate(videoPath string, d *config.DialogueItem, sampleInterval float64, threshold uint8, tempDir string) {
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

	files, err := ExtractCropJPEGsAccurate(videoPath, d.BBox, winStart, winEnd, endTempDir)
	if err == nil && len(files) > 0 {
		for idx := len(files) - 1; idx >= 0; idx-- {
			if detector.ProbeFrameForBubble(files[idx], threshold) {
				d.EndTimestamp = winStart + float64(idx)/30.0
				d.Duration = math.Max(0.2, d.EndTimestamp-d.Timestamp)
				break
			}
		}
	}
}

func RefineDialogueBoundaries(videoPath string, d *config.DialogueItem, interval float64, threshold uint8, tempDir string) {
	endWinStart := math.Max(d.Timestamp, d.EndTimestamp-interval)
	endWinEnd := d.EndTimestamp + interval
	endDur := endWinEnd - endWinStart

	if endDur > 0.03 {
		endTempDir, err := os.MkdirTemp(tempDir, "refine_end_*")
		if err == nil {
			defer os.RemoveAll(endTempDir)
			vfFilter := fmt.Sprintf("crop=%d:%d:%d:%d,fps=30", int(d.BBox.Width), int(d.BBox.Height), max(0, int(d.BBox.X)), max(0, int(d.BBox.Y)))
			outPattern := filepath.Join(endTempDir, "f_%03d.jpg")
			cmd := exec.Command("ffmpeg", "-y", "-hwaccel", "auto", "-ss", fmt.Sprintf("%.3f", endWinStart), "-i", videoPath, "-t", fmt.Sprintf("%.3f", endDur), "-vf", vfFilter, outPattern)
			if err := cmd.Run(); err == nil {
				files, _ := filepath.Glob(filepath.Join(endTempDir, "f_*.jpg"))
				if len(files) > 0 {
					refinedEnd := 0.0
					for idx := len(files) - 1; idx >= 0; idx-- {
						if detector.ProbeFrameForBubble(files[idx], threshold) {
							refinedEnd = endWinStart + float64(idx)/30.0
							break
						}
					}
					ApplyRefinedEnd(d, refinedEnd)
				}
			}
		}
	}
}

func ApplyRefinedEnd(d *config.DialogueItem, refinedEnd float64) {
	if refinedEnd <= d.Timestamp {
		return
	}
	d.EndTimestamp = refinedEnd
	d.Duration = d.EndTimestamp - d.Timestamp
}
