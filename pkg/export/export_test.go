package export

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"translate-video/pkg/config"
)

func TestFormatSRTTimestamp(t *testing.T) {
	cases := []struct {
		seconds  float64
		expected string
	}{
		{0.0, "00:00:00,000"},
		{1.5, "00:00:01,500"},
		{65.234, "00:01:05,234"},
		{3661.123, "01:01:01,123"},
		{-5.0, "00:00:00,000"},
	}

	for _, c := range cases {
		actual := FormatSRTTimestamp(c.seconds)
		if actual != c.expected {
			t.Errorf("FormatSRTTimestamp(%f) = %q; expected %q", c.seconds, actual, c.expected)
		}
	}
}

func TestExportSRT(t *testing.T) {
	dialogues := []config.DialogueItem{
		{
			ID:           1,
			Timestamp:    1.0,
			EndTimestamp: 3.5,
			Original:     "Hello there",
			Translated:   "Xin chào bạn",
			IsTrash:      false,
		},
		{
			ID:           2,
			Timestamp:    4.0,
			EndTimestamp: 6.0,
			Original:     "29:30",
			Translated:   "29:30",
			IsTrash:      true, // Trash item should be ignored in SRT
		},
		{
			ID:           3,
			Timestamp:    7.0,
			EndTimestamp: 9.5,
			Original:     "How are you?",
			Translated:   "Bạn khỏe không?",
			IsTrash:      false,
		},
	}

	jsonBytes, err := json.Marshal(dialogues)
	if err != nil {
		t.Fatalf("Failed to marshal dialogues: %v", err)
	}

	tempSRT := "test_output.srt"
	defer os.Remove(tempSRT)

	outPath, err := ExportSRT(string(jsonBytes), tempSRT)
	if err != nil {
		t.Fatalf("ExportSRT failed: %v", err)
	}

	contentBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("Failed to read generated SRT file: %v", err)
	}

	content := string(contentBytes)

	// Verify content
	if !strings.Contains(content, "Xin chào bạn") {
		t.Errorf("SRT content missing translated dialogue 1")
	}
	if strings.Contains(content, "29:30") {
		t.Errorf("SRT content should NOT include trash item '29:30'")
	}
	if !strings.Contains(content, "Bạn khỏe không?") {
		t.Errorf("SRT content missing translated dialogue 3")
	}
	if !strings.Contains(content, "00:00:01,000 -->") {
		t.Errorf("SRT timestamp format mismatch")
	}
}
