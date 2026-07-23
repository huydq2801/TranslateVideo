package pipeline

import (
	"math"
	"sort"

	"translate-video/pkg/config"
	"translate-video/pkg/translation"
)

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

func BoxesOverlap(b1, b2 config.BBox) bool {
	return SpatialIoU(b1, b2) >= 0.15
}

func IsDuplicateBubble(b1, b2 config.BBox, tolerance float64) bool {
	dx := math.Abs(b1.X - b2.X)
	dy := math.Abs(b1.Y - b2.Y)
	dw := math.Abs(b1.Width - b2.Width)
	dh := math.Abs(b1.Height - b2.Height)
	return dx <= tolerance && dy <= tolerance && dw <= tolerance && dh <= tolerance
}

func CalculateRepresentativeBBox(bboxes []config.BBox) config.BBox {
	if len(bboxes) == 0 {
		return config.BBox{}
	}
	var sumX, sumY, sumW, sumH float64
	for _, b := range bboxes {
		sumX += b.X
		sumY += b.Y
		sumW += b.Width
		sumH += b.Height
	}
	n := float64(len(bboxes))
	return config.BBox{
		X:      sumX / n,
		Y:      sumY / n,
		Width:  sumW / n,
		Height: sumH / n,
	}
}

func ComputeCompositeScore(box config.BBox, frameIdx int) float64 {
	areaScore := math.Min(1.0, (box.Width*box.Height)/80000.0)
	frameScore := math.Max(0.0, 1.0-float64(frameIdx)*0.01)
	return areaScore*0.7 + frameScore*0.3
}

func ClusterCandidates(candidates []config.CandidateItem, cfg config.ScanConfig, ocrResults map[int]string) []config.DialogueItem {
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
			overlap := BoxesOverlap(cand.BBox, lastCand.BBox)

			if sim >= 0.75 && (overlap || IsDuplicateBubble(cand.BBox, lastCand.BBox, 60.0)) {
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
		bboxes := make([]config.BBox, len(cls.Candidates))
		textCounts := make(map[string]int)

		for i, c := range cls.Candidates {
			bboxes[i] = c.BBox
			textCounts[c.Original]++
		}

		avgBBox := CalculateRepresentativeBBox(bboxes)

		bestText := cls.Candidates[0].Original
		maxCount := 0
		for text, cnt := range textCounts {
			if cnt > maxCount {
				maxCount = cnt
				bestText = text
			}
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
			EndTimestamp: cls.EndTime + cfg.Interval,
			Duration:     (cls.EndTime + cfg.Interval) - cls.StartTime,
			Original:     bestText,
			BBox:         expandedBBox,
		})
	}

	return dialogues
}

func ClusterFrameTimestamps(candidates []config.CandidateItem, sampleInterval float64) []config.DialogueItem {
	cfg := config.ScanConfig{Interval: sampleInterval}
	return ClusterCandidates(candidates, cfg, nil)
}
