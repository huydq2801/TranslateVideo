package detector

import (
	"crypto/md5"
	"fmt"
	"hash/crc32"
	"image"
	"golang.org/x/image/draw"

	"translate-video/pkg/config"
)

func AverageHash(img image.Image) uint64 {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return 0
	}

	var grays [64]uint32
	var sum uint64

	for y := 0; y < 8; y++ {
		sy := bounds.Min.Y + (y * h / 8)
		for x := 0; x < 8; x++ {
			sx := bounds.Min.X + (x * w / 8)
			r, g, b, _ := img.At(sx, sy).RGBA()
			gray := (r*77 + g*150 + b*29) >> 16
			grays[y*8+x] = gray
			sum += uint64(gray)
		}
	}

	avg := uint32(sum / 64)
	var hash uint64
	for i := 0; i < 64; i++ {
		if grays[i] >= avg {
			hash |= (1 << i)
		}
	}
	return hash
}

func HammingDistance(h1, h2 uint64) int {
	diff := h1 ^ h2
	count := 0
	for diff > 0 {
		count += int(diff & 1)
		diff >>= 1
	}
	return count
}

func ExactFrameHash(img image.Image) uint64 {
	switch t := img.(type) {
	case *image.YCbCr:
		h1 := uint64(crc32.ChecksumIEEE(t.Y))
		h2 := uint64(crc32.ChecksumIEEE(t.Cb))
		h3 := uint64(crc32.ChecksumIEEE(t.Cr))
		return (h1 << 32) ^ (h2 << 16) ^ h3
	case *image.RGBA:
		return uint64(crc32.ChecksumIEEE(t.Pix))
	case *image.NRGBA:
		return uint64(crc32.ChecksumIEEE(t.Pix))
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	buf := make([]byte, w*h*3)
	idx := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			buf[idx] = byte(r >> 8)
			buf[idx+1] = byte(g >> 8)
			buf[idx+2] = byte(b >> 8)
			idx += 3
		}
	}
	return uint64(crc32.ChecksumIEEE(buf))
}

func FastFrameMD5(img image.Image) string {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return ""
	}

	sampleBuf := make([]byte, 16*16*3)
	idx := 0
	stepX := max(1, w/16)
	stepY := max(1, h/16)

	for y := 0; y < 16; y++ {
		sy := bounds.Min.Y + y*stepY
		for x := 0; x < 16; x++ {
			sx := bounds.Min.X + x*stepX
			r, g, b, _ := img.At(sx, sy).RGBA()
			sampleBuf[idx] = byte(r >> 8)
			sampleBuf[idx+1] = byte(g >> 8)
			sampleBuf[idx+2] = byte(b >> 8)
			idx += 3
		}
	}
	return fmt.Sprintf("%x", md5.Sum(sampleBuf))
}

func MakeThumb(img image.Image) []uint8 {
	const thumbSize = 16
	thumb := image.NewRGBA(image.Rect(0, 0, thumbSize, thumbSize))
	draw.BiLinear.Scale(thumb, thumb.Bounds(), img, img.Bounds(), draw.Over, nil)
	pix := make([]uint8, thumbSize*thumbSize)
	for i := 0; i < thumbSize*thumbSize; i++ {
		r := uint32(thumb.Pix[i*4])
		g := uint32(thumb.Pix[i*4+1])
		b := uint32(thumb.Pix[i*4+2])
		pix[i] = uint8((r*299 + g*587 + b*114) / 1000)
	}
	return pix
}

func ThumbDiff(t1, t2 []uint8) float64 {
	if len(t1) != len(t2) || len(t1) == 0 {
		return 1.0
	}
	var sumDiff float64
	for i := range t1 {
		diff := float64(t1[i]) - float64(t2[i])
		if diff < 0 {
			diff = -diff
		}
		sumDiff += diff
	}
	return sumDiff / (float64(len(t1)) * 255.0)
}

func CropImage(img image.Image, bbox config.BBox) image.Image {
	bounds := img.Bounds()
	x0 := max(bounds.Min.X, int(bbox.X))
	y0 := max(bounds.Min.Y, int(bbox.Y))
	x1 := min(bounds.Max.X, int(bbox.X+bbox.Width))
	y1 := min(bounds.Max.Y, int(bbox.Y+bbox.Height))

	if x1 <= x0 || y1 <= y0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}

	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	if si, ok := img.(subImager); ok {
		return si.SubImage(image.Rect(x0, y0, x1, y1))
	}

	dst := image.NewRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	draw.Draw(dst, dst.Bounds(), img, image.Pt(x0, y0), draw.Src)
	return dst
}
