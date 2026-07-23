package export

import (
	_ "embed"
	"os"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
)

//go:embed fonts/fallback.ttf
var fallbackFontBytes []byte

func LoadFontWithEmbeddedFallback(dc *gg.Context, fontSize float64) {
	fontCandidates := []string{
		"C:\\Windows\\Fonts\\arial.ttf",
		"C:\\Windows\\Fonts\\segoeui.ttf",
		"C:\\Windows\\Fonts\\tahoma.ttf",
		"/System/Library/Fonts/Helvetica.ttc",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}

	loaded := false
	for _, fPath := range fontCandidates {
		if _, err := os.Stat(fPath); err == nil {
			if err := dc.LoadFontFace(fPath, fontSize); err == nil {
				loaded = true
				break
			}
		}
	}

	if !loaded && len(fallbackFontBytes) > 0 {
		f, err := truetype.Parse(fallbackFontBytes)
		if err == nil {
			face := truetype.NewFace(f, &truetype.Options{
				Size: fontSize,
			})
			dc.SetFontFace(face)
		}
	}
}
