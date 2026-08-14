package pipeline

import (
	"testing"
	"translate-video/pkg/config"
)

func TestSpatialIoU(t *testing.T) {
	// Identical bboxes
	b1 := config.BBox{X: 100, Y: 100, Width: 200, Height: 50}
	if iou := SpatialIoU(b1, b1); iou != 1.0 {
		t.Errorf("SpatialIoU with identical bboxes should be 1.0, got %f", iou)
	}

	// Completely separated bboxes
	b2 := config.BBox{X: 500, Y: 500, Width: 200, Height: 50}
	if iou := SpatialIoU(b1, b2); iou != 0.0 {
		t.Errorf("SpatialIoU with disjoint bboxes should be 0.0, got %f", iou)
	}

	// Partially overlapping bboxes
	b3 := config.BBox{X: 150, Y: 100, Width: 200, Height: 50}
	iou := SpatialIoU(b1, b3)
	if iou <= 0.0 || iou >= 1.0 {
		t.Errorf("SpatialIoU with overlapping bboxes should be between 0 and 1, got %f", iou)
	}
}

func TestClusterFrameTimestamps(t *testing.T) {
	candidates := []config.CandidateItem{
		// Cluster 1: Frame 1-3 (same text, overlapping bbox)
		{FrameIndex: 1, TimeSec: 0.0, Original: "Hello there", BBox: config.BBox{X: 200, Y: 600, Width: 300, Height: 40}},
		{FrameIndex: 2, TimeSec: 0.5, Original: "Hello there!", BBox: config.BBox{X: 200, Y: 600, Width: 300, Height: 40}},
		{FrameIndex: 3, TimeSec: 1.0, Original: "Hello there", BBox: config.BBox{X: 200, Y: 600, Width: 300, Height: 40}},

		// Gap of 3 seconds -> New cluster
		// Cluster 2: Frame 10-11
		{FrameIndex: 10, TimeSec: 4.5, Original: "How are you doing today?", BBox: config.BBox{X: 250, Y: 600, Width: 400, Height: 40}},
		{FrameIndex: 11, TimeSec: 5.0, Original: "How are you doing today?", BBox: config.BBox{X: 250, Y: 600, Width: 400, Height: 40}},

		// Cluster 3: Watermark (should be marked as Trash)
		{FrameIndex: 20, TimeSec: 9.0, Original: "29:30", BBox: config.BBox{X: 1100, Y: 20, Width: 100, Height: 30}},
	}

	dialogues := ClusterFrameTimestamps(candidates, 0.50)

	if len(dialogues) != 3 {
		t.Fatalf("Expected 3 clustered dialogue items, got %d", len(dialogues))
	}

	// Check Cluster 1
	d1 := dialogues[0]
	if d1.ID != 1 {
		t.Errorf("Expected d1.ID to be 1, got %d", d1.ID)
	}
	// Best text selection should pick "Hello there!" (longest)
	if d1.Original != "Hello there!" {
		t.Errorf("Expected d1.Original to be 'Hello there!', got %q", d1.Original)
	}
	if d1.Timestamp > 0.0 {
		t.Errorf("Expected d1.Timestamp to start near 0.0, got %f", d1.Timestamp)
	}
	if d1.IsTrash {
		t.Errorf("Expected d1 not to be trash")
	}

	// Check Cluster 2
	d2 := dialogues[1]
	if d2.ID != 2 {
		t.Errorf("Expected d2.ID to be 2, got %d", d2.ID)
	}
	if d2.Original != "How are you doing today?" {
		t.Errorf("Expected d2.Original to be 'How are you doing today?', got %q", d2.Original)
	}
	if d2.IsTrash {
		t.Errorf("Expected d2 not to be trash")
	}

	// Check Cluster 3 (Trash item)
	d3 := dialogues[2]
	if !d3.IsTrash {
		t.Errorf("Expected timestamp '29:30' to be marked as IsTrash = true")
	}
}
