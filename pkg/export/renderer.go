package export

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fogleman/gg"
	"translate-video/pkg/config"
)

func RenderSpeechBubblePNG(d config.DialogueItem, index int, outDir string, canvasWidth, canvasHeight int) (string, error) {
	if canvasWidth <= 0 || canvasHeight <= 0 {
		canvasWidth, canvasHeight = 1920, 1080
	}

	dc := gg.NewContext(canvasWidth, canvasHeight)

	bbox := d.BBox
	text := d.Translated
	if text == "" {
		text = d.Original
	}

	x := bbox.X
	y := bbox.Y
	w := bbox.Width
	h := bbox.Height
	r := 15.0

	dc.SetHexColor("#ffffff")
	dc.DrawRoundedRectangle(x, y, w, h, r)
	dc.Fill()

	dc.SetHexColor("#000000")
	fontSize := 32.0 * (float64(canvasHeight) / 1080.0)
	if fontSize < 16 {
		fontSize = 16
	}

	LoadFontWithEmbeddedFallback(dc, fontSize)

	words := strings.Fields(text)
	var lines []string
	var currentLine string
	maxWidth := w * 0.85

	for _, word := range words {
		testLine := word
		if currentLine != "" {
			testLine = currentLine + " " + word
		}
		tw, _ := dc.MeasureString(testLine)
		if tw > maxWidth && currentLine != "" {
			lines = append(lines, currentLine)
			currentLine = word
		} else {
			currentLine = testLine
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	lineHeight := fontSize * 1.3
	startY := y + (h / 2.0) - (float64(len(lines)-1) * lineHeight / 2.0)

	for i, line := range lines {
		dc.DrawStringAnchored(line, x+(w/2.0), startY+(float64(i)*lineHeight), 0.5, 0.5)
	}

	filename := fmt.Sprintf("bubble_%d.png", index)
	outPath := filepath.Join(outDir, filename)
	err := dc.SavePNG(outPath)
	if err != nil {
		return "", err
	}

	return outPath, nil
}
