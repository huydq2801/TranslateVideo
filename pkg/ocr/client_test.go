package ocr

import (
	"testing"
)

func TestMergeVerticallyAdjacentLines(t *testing.T) {
	// Case 1: Empty or single item
	if len(MergeVerticallyAdjacentLines(nil)) != 0 {
		t.Errorf("Expected empty slice for nil input")
	}

	single := []OCRFullFrameItem{{Text: "Hello", Score: 0.9, BBox: [4]int{100, 100, 200, 30}}}
	if len(MergeVerticallyAdjacentLines(single)) != 1 {
		t.Errorf("Expected 1 item for single input")
	}

	// Case 2: Same row items with small horizontal gap (text broken by inline icon)
	sameRow := []OCRFullFrameItem{
		{Text: "Tôi là", Score: 0.95, BBox: [4]int{100, 200, 80, 25}},
		{Text: "người dịch", Score: 0.92, BBox: [4]int{210, 200, 120, 25}},
	}
	mergedRow := MergeVerticallyAdjacentLines(sameRow)
	if len(mergedRow) != 1 {
		t.Fatalf("Expected 1 merged same-row item, got %d", len(mergedRow))
	}
	if mergedRow[0].Text != "Tôi là người dịch" {
		t.Errorf("Expected merged text 'Tôi là người dịch', got %q", mergedRow[0].Text)
	}

	// Case 3: Vertically adjacent lines (multi-line speech bubble)
	stackedLines := []OCRFullFrameItem{
		{Text: "Dòng một của câu thoại", Score: 0.95, BBox: [4]int{100, 100, 250, 30}},
		{Text: "Dòng hai tiếp theo", Score: 0.90, BBox: [4]int{100, 135, 230, 30}},
	}
	mergedStacked := MergeVerticallyAdjacentLines(stackedLines)
	if len(mergedStacked) != 1 {
		t.Fatalf("Expected 1 merged multi-line item, got %d", len(mergedStacked))
	}
	if mergedStacked[0].BBox[3] <= 30 {
		t.Errorf("Expected merged height > 30, got %d", mergedStacked[0].BBox[3])
	}
}
