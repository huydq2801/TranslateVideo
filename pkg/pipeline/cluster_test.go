package pipeline

import (
	"testing"

	"translate-video/pkg/config"
)

func TestClusterFrameTimestampsFlickerGap(t *testing.T) {
	cands := []config.CandidateItem{
		{FrameIndex: 1, TimeSec: 0.0, Original: "这...", BBox: config.BBox{X: 100, Y: 100, Width: 80, Height: 40}},
		{FrameIndex: 2, TimeSec: 0.3, Original: "这...", BBox: config.BBox{X: 100, Y: 100, Width: 80, Height: 40}},
		{FrameIndex: 4, TimeSec: 0.9, Original: "气...", BBox: config.BBox{X: 101, Y: 100, Width: 80, Height: 40}},
	}

	dialogues := ClusterFrameTimestamps(cands, 0.30)
	if len(dialogues) != 2 {
		t.Fatalf("Expected 2 dialogues for different text across gap, got %d", len(dialogues))
	}
}

func TestClusterFrameTimestampsIdenticalFlickerGap(t *testing.T) {
	cands := []config.CandidateItem{
		{FrameIndex: 1, TimeSec: 0.0, Original: "这...", BBox: config.BBox{X: 100, Y: 100, Width: 80, Height: 40}},
		{FrameIndex: 2, TimeSec: 0.3, Original: "这...", BBox: config.BBox{X: 100, Y: 100, Width: 80, Height: 40}},
		{FrameIndex: 4, TimeSec: 0.9, Original: "这...", BBox: config.BBox{X: 101, Y: 100, Width: 80, Height: 40}},
	}

	dialogues := ClusterFrameTimestamps(cands, 0.30)
	if len(dialogues) != 1 {
		t.Fatalf("Expected 1 clustered dialogue (handling flicker gap), got %d", len(dialogues))
	}

	d := dialogues[0]
	if d.Timestamp != 0.0 {
		t.Errorf("Expected Timestamp 0.0, got %f", d.Timestamp)
	}
	if d.EndTimestamp != 1.20 {
		t.Errorf("Expected EndTimestamp 1.20, got %f", d.EndTimestamp)
	}
}

func TestBoxesOverlapAndIsDuplicateBubble(t *testing.T) {
	b1 := config.BBox{X: 10, Y: 10, Width: 100, Height: 50}
	b2 := config.BBox{X: 15, Y: 15, Width: 100, Height: 50}
	b3 := config.BBox{X: 500, Y: 500, Width: 100, Height: 50}

	if !BoxesOverlap(b1, b2) {
		t.Fatal("BoxesOverlap should return true for overlapping bboxes")
	}
	if BoxesOverlap(b1, b3) {
		t.Fatal("BoxesOverlap should return false for distant bboxes")
	}

	if !IsDuplicateBubble(b1, b2, 10.0) {
		t.Fatal("IsDuplicateBubble should return true within tolerance")
	}
	if IsDuplicateBubble(b1, b3, 10.0) {
		t.Fatal("IsDuplicateBubble should return false outside tolerance")
	}
}
