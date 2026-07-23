package detector

import (
	"image"
	"image/color"
	"os"
	"sort"
	"sync"

	"translate-video/pkg/config"
)

var visitedPool = sync.Pool{
	New: func() interface{} {
		return make([]bool, 0)
	},
}

func getVisitedBuffer(size int) []bool {
	buf := visitedPool.Get().([]bool)
	if cap(buf) < size {
		return make([]bool, size)
	}
	buf = buf[:size]
	for i := range buf {
		buf[i] = false
	}
	return buf
}

func releaseVisitedBuffer(buf []bool) {
	visitedPool.Put(buf)
}

type FastLumaReader struct {
	yCbCr *image.YCbCr
	rgba  *image.RGBA
	nrgba *image.NRGBA
	img   image.Image
	minX  int
	minY  int
}

func NewFastLumaReader(img image.Image) *FastLumaReader {
	bounds := img.Bounds()
	r := &FastLumaReader{
		img:  img,
		minX: bounds.Min.X,
		minY: bounds.Min.Y,
	}
	switch t := img.(type) {
	case *image.YCbCr:
		r.yCbCr = t
	case *image.RGBA:
		r.rgba = t
	case *image.NRGBA:
		r.nrgba = t
	}
	return r
}

func (r *FastLumaReader) IsWhite(x, y int, thresh uint8) bool {
	if r.yCbCr != nil {
		off := (y-r.minY)*r.yCbCr.YStride + (x - r.minX)
		return r.yCbCr.Y[off] >= thresh
	}
	if r.rgba != nil {
		off := (y-r.minY)*r.rgba.Stride + (x-r.minX)*4
		r8, g8, b8 := r.rgba.Pix[off], r.rgba.Pix[off+1], r.rgba.Pix[off+2]
		return r8 >= thresh-15 && g8 >= thresh-15 && b8 >= thresh-15
	}
	if r.nrgba != nil {
		off := (y-r.minY)*r.nrgba.Stride + (x-r.minX)*4
		r8, g8, b8 := r.nrgba.Pix[off], r.nrgba.Pix[off+1], r.nrgba.Pix[off+2]
		return r8 >= thresh-15 && g8 >= thresh-15 && b8 >= thresh-15
	}
	cr, cg, cb, _ := r.img.At(x, y).RGBA()
	r8, g8, b8 := uint8(cr>>8), uint8(cg>>8), uint8(cb>>8)
	return r8 >= thresh-15 && g8 >= thresh-15 && b8 >= thresh-15
}

func (r *FastLumaReader) SampleRGB(x, y int) SampleRGB {
	if r.yCbCr != nil {
		yi := r.yCbCr.YOffset(x, y)
		ci := r.yCbCr.COffset(x, y)
		cr, cg, cb := color.YCbCrToRGB(r.yCbCr.Y[yi], r.yCbCr.Cb[ci], r.yCbCr.Cr[ci])
		return SampleRGB{cr, cg, cb}
	}
	if r.rgba != nil {
		off := (y-r.minY)*r.rgba.Stride + (x-r.minX)*4
		return SampleRGB{r.rgba.Pix[off], r.rgba.Pix[off+1], r.rgba.Pix[off+2]}
	}
	if r.nrgba != nil {
		off := (y-r.minY)*r.nrgba.Stride + (x-r.minX)*4
		return SampleRGB{r.nrgba.Pix[off], r.nrgba.Pix[off+1], r.nrgba.Pix[off+2]}
	}
	cr, cg, cb, _ := r.img.At(x, y).RGBA()
	return SampleRGB{uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8)}
}

type bubbleColourProfile uint8

const (
	bubbleColourNone bubbleColourProfile = iota
	bubbleColourWhite
	bubbleColourBlack
	bubbleColourBlue
	bubbleColourGreen
	bubbleColourGrey
)

type SampleRGB struct{ R, G, B uint8 }

type colouredBubbleCandidate struct {
	box   config.BBox
	score float64
}

func DetectSpeechBubbles(img image.Image, threshold uint8) []config.BBox {
	bounds := img.Bounds()
	width := bounds.Max.X - bounds.Min.X
	height := bounds.Max.Y - bounds.Min.Y

	if width == 0 || height == 0 {
		return nil
	}

	totalPixels := width * height
	visited := getVisitedBuffer(totalPixels)
	defer releaseVisitedBuffer(visited)

	var whiteBubbles []config.BBox
	var colourBubbles []colouredBubbleCandidate
	lumaReader := NewFastLumaReader(img)

	idx := func(x, y int) int { return y*width + x }

	minW, minH := width/35, height/45
	if minW < 12 {
		minW = 12
	}
	if minH < 12 {
		minH = 12
	}
	whiteMaxW := int(float64(width) * 0.88)
	whiteMaxH := int(float64(height) * 0.65)
	whiteMaxArea := int(float64(width*height) * 0.45)

	for y := bounds.Min.Y; y < bounds.Max.Y; y += 2 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 2 {
			ix := idx(x-bounds.Min.X, y-bounds.Min.Y)
			if visited[ix] {
				continue
			}

			profile := classifyBubbleColour(lumaReader, x, y, threshold)
			if profile != bubbleColourNone {
				minX, maxX := x, x
				minY, maxY := y, y
				pixelCount := 0

				componentPixels := make([]int, 0, 64)
				queue := [][2]int{{x, y}}
				visited[ix] = true
				componentPixels = append(componentPixels, ix)

				for len(queue) > 0 {
					curr := queue[0]
					queue = queue[1:]
					cx, cy := curr[0], curr[1]
					pixelCount++

					if cx < minX {
						minX = cx
					}
					if cx > maxX {
						maxX = cx
					}
					if cy < minY {
						minY = cy
					}
					if cy > maxY {
						maxY = cy
					}

					neighbors := [4][2]int{
						{cx + 1, cy}, {cx - 1, cy},
						{cx, cy + 1}, {cx, cy - 1},
					}

					for _, n := range neighbors {
						nx, ny := n[0], n[1]
						if nx >= bounds.Min.X && nx < bounds.Max.X && ny >= bounds.Min.Y && ny < bounds.Max.Y {
							nix := idx(nx-bounds.Min.X, ny-bounds.Min.Y)
							if !visited[nix] {
								nProfile := classifyBubbleColour(lumaReader, nx, ny, threshold)
								if nProfile == profile {
									visited[nix] = true
									componentPixels = append(componentPixels, nix)
									queue = append(queue, [2]int{nx, ny})
								} else if profile == bubbleColourWhite && (nProfile == bubbleColourBlack || nProfile == bubbleColourNone) {
									nc := lumaReader.SampleRGB(nx, ny)
									if Luma(nc) <= 35 {
										visited[nix] = true
										componentPixels = append(componentPixels, nix)
										queue = append(queue, [2]int{nx, ny})
									}
								}
							}
						}
					}
				}

				w := maxX - minX
				h := maxY - minY
				bboxArea := w * h
				if bboxArea == 0 {
					for _, pix := range componentPixels {
						visited[pix] = false
					}
					continue
				}

				fillRatio := float64(pixelCount) / float64(bboxArea)
				box := config.BBox{
					X:      float64(minX),
					Y:      float64(minY),
					Width:  float64(w),
					Height: float64(h),
				}
				seedColour := SampleImageRGB(img, x, y)

				if profile == bubbleColourWhite {
					isInsideColored := isWhiteTextInsideColoredBalloon(lumaReader, minX, minY, maxX, maxY, threshold)
					isAcceptedWhite := w >= minW && h >= minH && w <= whiteMaxW && h <= whiteMaxH && bboxArea <= whiteMaxArea && fillRatio >= 0.20 && !isInsideColored
					if isAcceptedWhite {
						whiteBubbles = append(whiteBubbles, box)
					} else {
						for _, pix := range componentPixels {
							visited[pix] = false
						}
					}
					if (w <= whiteMaxW && h <= whiteMaxH && bboxArea <= whiteMaxArea && !isInsideColored) || !isOffWhiteGrey(seedColour, threshold) {
						continue
					}
				}

				aspectRatio := float64(w) / float64(h)
				if w < minW || h < minH || h > int(float64(height)*0.75) || bboxArea > int(float64(width*height)*0.55) || aspectRatio < 0.35 || aspectRatio > 10.0 || fillRatio < 0.25 {
					continue
				}
				if !HasContrastingText(lumaReader, minX, minY, maxX+1, maxY+1, seedColour) {
					continue
				}
				colourBubbles = append(colourBubbles, colouredBubbleCandidate{box: box, score: fillRatio * float64(bboxArea)})
			}
		}
	}

	sort.Slice(colourBubbles, func(i, j int) bool { return colourBubbles[i].score > colourBubbles[j].score })
	if len(colourBubbles) > 5 {
		colourBubbles = colourBubbles[:5]
	}
	for _, candidate := range colourBubbles {
		if !OverlapsExistingBubble(candidate.box, whiteBubbles) {
			whiteBubbles = append(whiteBubbles, candidate.box)
		}
	}

	return whiteBubbles
}

func ProbeFrameForBubble(framePath string, threshold uint8) bool {
	file, err := os.Open(framePath)
	if err != nil {
		return false
	}
	img, _, err := image.Decode(file)
	file.Close()
	if err != nil {
		return false
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return false
	}

	reader := NewFastLumaReader(img)
	const gridCols = 15
	const gridRows = 9
	stepX := max(1, w/gridCols)
	stepY := max(1, h/gridRows)

	baseColor := reader.SampleRGB(bounds.Min.X+2, bounds.Min.Y+2)
	baseLuma := Luma(baseColor)
	contrastHits, bgHits := 0, 0

	for gy := 0; gy < gridRows; gy++ {
		for gx := 0; gx < gridCols; gx++ {
			px := bounds.Min.X + gx*stepX + stepX/2
			py := bounds.Min.Y + gy*stepY + stepY/2
			if px >= bounds.Max.X || py >= bounds.Max.Y {
				continue
			}
			c := reader.SampleRGB(px, py)
			cLuma := Luma(c)
			if MaxChannelDelta(c, baseColor) < 40 && AbsInt(cLuma-baseLuma) < 35 {
				bgHits++
			} else if MaxChannelDelta(c, baseColor) >= 60 || AbsInt(cLuma-baseLuma) >= 45 {
				contrastHits++
			}
		}
	}

	total := gridCols * gridRows
	return bgHits >= (total*35)/100 && contrastHits >= 2
}

func classifyBubbleColour(reader *FastLumaReader, x, y int, threshold uint8) bubbleColourProfile {
	if reader.IsWhite(x, y, threshold) {
		return bubbleColourWhite
	}
	c := SampleImageRGB(reader.img, x, y)
	brightness := Luma(c)
	spread := ChannelSpread(c)
	if brightness <= 55 && spread <= 55 {
		return bubbleColourBlack
	}
	if c.B >= 130 && int(c.B)-int(c.R) >= 55 && int(c.B)-int(c.G) >= 15 {
		return bubbleColourBlue
	}
	if c.G >= 100 && int(c.G)-int(c.R) >= 40 && int(c.G)-int(c.B) >= 15 {
		return bubbleColourGreen
	}
	if brightness >= 115 && brightness < int(threshold)-10 && spread <= 35 {
		return bubbleColourGrey
	}
	return bubbleColourNone
}

func ChannelSpread(c SampleRGB) int {
	minC, maxC := int(c.R), int(c.R)
	for _, v := range []uint8{c.G, c.B} {
		if int(v) < minC {
			minC = int(v)
		}
		if int(v) > maxC {
			maxC = int(v)
		}
	}
	return maxC - minC
}

func isOffWhiteGrey(c SampleRGB, threshold uint8) bool {
	return Luma(c) >= 115 && Luma(c) < int(threshold) && ChannelSpread(c) <= 35
}

func SampleImageRGB(img image.Image, x, y int) SampleRGB {
	r, g, b, _ := img.At(x, y).RGBA()
	return SampleRGB{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
}

func MaxChannelDelta(a, b SampleRGB) int {
	dr := AbsInt(int(a.R) - int(b.R))
	dg := AbsInt(int(a.G) - int(b.G))
	db := AbsInt(int(a.B) - int(b.B))
	if dg > dr {
		dr = dg
	}
	if db > dr {
		dr = db
	}
	return dr
}

func Luma(c SampleRGB) int { return (299*int(c.R) + 587*int(c.G) + 114*int(c.B)) / 1000 }

func HasContrastingText(reader *FastLumaReader, minX, minY, maxX, maxY int, base SampleRGB) bool {
	bounds := reader.img.Bounds()
	if maxX > bounds.Max.X {
		maxX = bounds.Max.X
	}
	if maxY > bounds.Max.Y {
		maxY = bounds.Max.Y
	}
	samples, contrast := 0, 0
	baseLuma := Luma(base)
	for y := minY + 2; y < maxY-2; y += 2 {
		for x := minX + 2; x < maxX-2; x += 2 {
			samples++
			c := reader.SampleRGB(x, y)
			if MaxChannelDelta(c, base) >= 60 || AbsInt(Luma(c)-baseLuma) >= 45 {
				contrast++
			}
		}
	}
	return samples > 0 && contrast >= 4
}

func isWhiteTextInsideColoredBalloon(reader *FastLumaReader, minX, minY, maxX, maxY int, threshold uint8) bool {
	w := maxX - minX
	h := maxY - minY
	if w >= 80 || h >= 35 {
		return false
	}

	bounds := reader.img.Bounds()
	pad := 5
	x0 := max(bounds.Min.X, minX-pad)
	y0 := max(bounds.Min.Y, minY-pad)
	x1 := min(bounds.Max.X-1, maxX+pad)
	y1 := min(bounds.Max.Y-1, maxY+pad)

	borderPoints := [8][2]int{
		{x0, y0}, {minX + w/2, y0}, {x1, y0},
		{x0, minY + h/2}, {x1, minY + h/2},
		{x0, y1}, {minX + w/2, y1}, {x1, y1},
	}

	coloredHits := 0
	for _, p := range borderPoints {
		prof := classifyBubbleColour(reader, p[0], p[1], threshold)
		if prof == bubbleColourGreen || prof == bubbleColourBlue || prof == bubbleColourGrey {
			coloredHits++
		}
	}
	return coloredHits >= 4
}

func AbsInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func OverlapsExistingBubble(candidate config.BBox, existing []config.BBox) bool {
	for _, box := range existing {
		left := max(candidate.X, box.X)
		top := max(candidate.Y, box.Y)
		right := min(candidate.X+candidate.Width, box.X+box.Width)
		bottom := min(candidate.Y+candidate.Height, box.Y+box.Height)
		if right <= left || bottom <= top {
			continue
		}
		intersection := (right - left) * (bottom - top)
		candidateArea := candidate.Width * candidate.Height
		boxArea := box.Width * box.Height
		if boxArea >= candidateArea*0.35 && (intersection/candidateArea > 0.70 || intersection/boxArea > 0.70) {
			return true
		}
	}
	return false
}
