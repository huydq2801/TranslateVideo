package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"translate-video/pkg/translation"
)

type OCRFullFrameItem struct {
	Text  string `json:"text"`
	Score float64 `json:"score"`
	BBox  [4]int  `json:"bbox"`
}

type OCRFullFrameBatchResult struct {
	Path    string             `json:"path"`
	Success bool               `json:"success"`
	Items   []OCRFullFrameItem `json:"items"`
	Error   string             `json:"error"`
}

type OCRFullFrameBatchResponse struct {
	Results []OCRFullFrameBatchResult `json:"results"`
}

var HTTPClient = &http.Client{
	Timeout: 180 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	},
}

func MergeVerticallyAdjacentLines(items []OCRFullFrameItem) []OCRFullFrameItem {
	if len(items) <= 1 {
		return items
	}

	// Sort by Y first, then X to process row by row left to right
	sort.Slice(items, func(i, j int) bool {
		yi, yj := items[i].BBox[1], items[j].BBox[1]
		if math.Abs(float64(yi-yj)) <= 12 { // same row tolerance
			return items[i].BBox[0] < items[j].BBox[0]
		}
		return yi < yj
	})

	// Pass 1: Merge same-row fragments broken by inline icons/emojis.
	// Two items are "same row" when their Y centers are within ~60% of the shorter height
	// and the horizontal gap between them is ≤80px (icon width).
	var sameRowMerged []OCRFullFrameItem
	for _, item := range items {
		if len(sameRowMerged) == 0 {
			sameRowMerged = append(sameRowMerged, item)
			continue
		}
		prev := &sameRowMerged[len(sameRowMerged)-1]

		prevY1, prevH := prev.BBox[1], prev.BBox[3]
		currY1, currH := item.BBox[1], item.BBox[3]
		prevCenterY := float64(prevY1) + float64(prevH)/2.0
		currCenterY := float64(currY1) + float64(currH)/2.0
		minH := math.Min(float64(prevH), float64(currH))

		prevX2 := prev.BBox[0] + prev.BBox[2]
		currX1 := item.BBox[0]
		horizGap := currX1 - prevX2 // pixels between right edge of prev and left edge of curr

		sameRow := math.Abs(prevCenterY-currCenterY) <= minH*0.6
		adjacentH := horizGap >= -10 && horizGap <= 80 // icon typically ≤80px wide

		if sameRow && adjacentH {
			// Join the two fragments back into one item
			if translation.IsCJK(prev.Text) || translation.IsCJK(item.Text) {
				prev.Text = strings.TrimSpace(prev.Text) + strings.TrimSpace(item.Text)
			} else {
				prev.Text = strings.TrimSpace(prev.Text) + " " + strings.TrimSpace(item.Text)
			}
			newX2 := max(prevX2, item.BBox[0]+item.BBox[2])
			newY1 := min(prev.BBox[1], item.BBox[1])
			newY2 := max(prev.BBox[1]+prev.BBox[3], item.BBox[1]+item.BBox[3])
			prev.BBox = [4]int{prev.BBox[0], newY1, newX2 - prev.BBox[0], newY2 - newY1}
			continue
		}
		sameRowMerged = append(sameRowMerged, item)
	}

	// Pass 2: Merge vertically adjacent items (original logic)
	var merged []OCRFullFrameItem
	visited := make([]bool, len(sameRowMerged))

	for i := 0; i < len(sameRowMerged); i++ {
		if visited[i] {
			continue
		}
		curr := sameRowMerged[i]
		visited[i] = true

		for j := i + 1; j < len(sameRowMerged); j++ {
			if visited[j] {
				continue
			}
			next := sameRowMerged[j]

			currX1, currY1, currW, currH := curr.BBox[0], curr.BBox[1], curr.BBox[2], curr.BBox[3]
			nextX1, nextY1, nextW, nextH := next.BBox[0], next.BBox[1], next.BBox[2], next.BBox[3]

			currX2 := currX1 + currW
			currY2 := currY1 + currH
			nextX2 := nextX1 + nextW
			nextY2 := nextY1 + nextH

			vertGap := nextY1 - currY2
			currCenterX := float64(currX1+currX2) / 2.0
			nextCenterX := float64(nextX1+nextX2) / 2.0
			centerDiff := math.Abs(currCenterX - nextCenterX)
			minW := math.Min(float64(currW), float64(nextW))

			if vertGap >= -20 && vertGap <= 50 && centerDiff <= math.Max(150.0, minW*1.2) {
				if translation.IsCJK(curr.Text) || translation.IsCJK(next.Text) {
					curr.Text = strings.TrimSpace(curr.Text) + strings.TrimSpace(next.Text)
				} else {
					curr.Text = strings.TrimSpace(curr.Text) + " " + strings.TrimSpace(next.Text)
				}

				minX := min(currX1, nextX1)
				minY := min(currY1, nextY1)
				maxX := max(currX2, nextX2)
				maxY := max(currY2, nextY2)
				curr.BBox = [4]int{minX, minY, maxX - minX, maxY - minY}
				visited[j] = true
			}
		}
		merged = append(merged, curr)
	}

	return merged
}

func CallPythonOCRFullFrameBatch(ctx context.Context, paths []string, lang string) (map[string][]OCRFullFrameItem, error) {
	if len(paths) == 0 {
		return map[string][]OCRFullFrameItem{}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	reqBody := struct {
		Paths []string `json:"paths"`
		Lang  string   `json:"lang"`
	}{
		Paths: paths,
		Lang:  lang,
	}

	buf, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:8000/ocr_full_frame_batch", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OCR FullFrame HTTP status: %d", resp.StatusCode)
	}

	var batchResp OCRFullFrameBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		return nil, err
	}

	resultMap := make(map[string][]OCRFullFrameItem)
	for _, r := range batchResp.Results {
		if r.Success {
			resultMap[r.Path] = r.Items
		}
	}
	return resultMap, nil
}
