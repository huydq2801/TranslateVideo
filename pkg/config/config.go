package config

import "fmt"

type BBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type CandidateItem struct {
	FrameIndex int
	TimeSec    float64
	Original   string
	BBox       BBox
}

type DialogueItem struct {
	ID                int     `json:"id"`
	FrameIndex        int     `json:"frameIndex"`
	Timestamp         float64 `json:"timestamp"`
	Duration          float64 `json:"duration,omitempty"`
	EndTimestamp      float64 `json:"endTimestamp,omitempty"`
	Original          string  `json:"original"`
	Translated        string  `json:"translated"`
	TranslationFailed bool    `json:"translationFailed"`
	IsTrash           bool    `json:"isTrash,omitempty"`
	BBox              BBox    `json:"bbox"`
}

type HealthStatus struct {
	OCROnline bool   `json:"ocrOnline"`
	OCREngine string `json:"ocrEngine"`
	HasAPIKey bool   `json:"hasApiKey"`
	ErrorMsg  string `json:"errorMsg"`
}

type ScanConfig struct {
	VideoPath  string  `json:"videoPath"`
	Interval   float64 `json:"interval"`
	Threshold  uint8   `json:"threshold"`
	SourceLang string  `json:"sourceLang"`
	GeminiKey  string  `json:"geminiKey"`
}

func FormatTimeSec(seconds float64) string {
	m := int(seconds) / 60
	s := int(seconds) % 60
	ms := int((seconds - float64(int(seconds))) * 10)
	return fmt.Sprintf("%02d:%02d.%d", m, s, ms)
}
