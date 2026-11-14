package imagemeta

import (
	"image"
	"image/color"
	"math"
)

// DefaultBlurRadius is the default kernel radius used when blurring profile images
// for non-authenticated visitors.
const DefaultBlurRadius = 10

// BlurImage applies a multi-pass separable box blur (approximating a Gaussian blur)
// with the specified radius so that facial identity is obscured server-side before
// bytes are sent to unauthenticated clients.
func BlurImage(src image.Image, radius int) image.Image {
	if src == nil {
		return nil
	}
	if radius <= 0 {
		radius = DefaultBlurRadius
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w == 0 || h == 0 {
		return src
	}

	curr := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			curr.Set(x, y, color.NRGBAModel.Convert(src.At(bounds.Min.X+x, bounds.Min.Y+y)))
		}
	}

	tmp := image.NewNRGBA(image.Rect(0, 0, w, h))

	// 3 passes of separable horizontal + vertical box blur approximate a Gaussian kernel
	for pass := 0; pass < 3; pass++ {
		boxBlurHorizontal(curr, tmp, w, h, radius)
		boxBlurVertical(tmp, curr, w, h, radius)
	}

	return curr
}

func boxBlurHorizontal(src, dst *image.NRGBA, w, h, radius int) {
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var rSum, gSum, bSum, aSum, count int
			xMin := x - radius
			if xMin < 0 {
				xMin = 0
			}
			xMax := x + radius
			if xMax >= w {
				xMax = w - 1
			}
			for ix := xMin; ix <= xMax; ix++ {
				offset := src.PixOffset(ix, y)
				rSum += int(src.Pix[offset])
				gSum += int(src.Pix[offset+1])
				bSum += int(src.Pix[offset+2])
				aSum += int(src.Pix[offset+3])
				count++
			}
			outOffset := dst.PixOffset(x, y)
			dst.Pix[outOffset] = uint8(rSum / count)
			dst.Pix[outOffset+1] = uint8(gSum / count)
			dst.Pix[outOffset+2] = uint8(bSum / count)
			dst.Pix[outOffset+3] = uint8(aSum / count)
		}
	}
}

func boxBlurVertical(src, dst *image.NRGBA, w, h, radius int) {
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			var rSum, gSum, bSum, aSum, count int
			yMin := y - radius
			if yMin < 0 {
				yMin = 0
			}
			yMax := y + radius
			if yMax >= h {
				yMax = h - 1
			}
			for iy := yMin; iy <= yMax; iy++ {
				offset := src.PixOffset(x, iy)
				rSum += int(src.Pix[offset])
				gSum += int(src.Pix[offset+1])
				bSum += int(src.Pix[offset+2])
				aSum += int(src.Pix[offset+3])
				count++
			}
			outOffset := dst.PixOffset(x, y)
			dst.Pix[outOffset] = uint8(rSum / count)
			dst.Pix[outOffset+1] = uint8(gSum / count)
			dst.Pix[outOffset+2] = uint8(bSum / count)
			dst.Pix[outOffset+3] = uint8(aSum / count)
		}
	}
}

// DetectFace inspects a portrait/avatar image for human facial characteristics
// using YCbCr/RGB skin-tone clustering in the central portrait region combined
// with luminance variance and horizontal eye/cheek Haar-like contrast features.
// Rejects blank, uniform, pure-geometric, or non-portrait images.
func DetectFace(src image.Image) bool {
	if src == nil {
		return false
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w < 16 || h < 16 {
		return false
	}

	// Analyze central portrait window (middle 70% x 70% of image)
	xStart := bounds.Min.X + w*15/100
	xEnd := bounds.Min.X + w*85/100
	yStart := bounds.Min.Y + h*15/100
	yEnd := bounds.Min.Y + h*85/100

	var (
		totalCenterPixels int
		skinPixels        int
		lumSum            float64
		lumSqSum          float64
		upperBandLumSum   float64
		upperBandCount    int
		midBandLumSum     float64
		midBandCount      int
	)

	centerH := yEnd - yStart
	eyeBandEnd := yStart + centerH*45/100
	cheekBandEnd := yStart + centerH*80/100

	for y := yStart; y < yEnd; y++ {
		for x := xStart; x < xEnd; x++ {
			r16, g16, b16, a16 := src.At(x, y).RGBA()
			if a16 < 0x2000 {
				continue
			}
			r := uint8(r16 >> 8)
			g := uint8(g16 >> 8)
			b := uint8(b16 >> 8)

			totalCenterPixels++
			if isSkinTonePixel(r, g, b) {
				skinPixels++
			}

			lum := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
			lumSum += lum
			lumSqSum += lum * lum

			if y < eyeBandEnd {
				upperBandLumSum += lum
				upperBandCount++
			} else if y < cheekBandEnd {
				midBandLumSum += lum
				midBandCount++
			}
		}
	}

	if totalCenterPixels == 0 {
		return false
	}

	meanLum := lumSum / float64(totalCenterPixels)
	variance := (lumSqSum / float64(totalCenterPixels)) - (meanLum * meanLum)
	stdDev := math.Sqrt(math.Max(0, variance))

	// Uniform / flat color blocks have almost zero luminance standard deviation
	if stdDev < 6.0 {
		return false
	}

	skinRatio := float64(skinPixels) / float64(totalCenterPixels)
	if skinRatio >= 0.12 && skinRatio <= 0.92 {
		return true
	}

	// Fallback for monochrome/grayscale portraits: check Haar-like eye-socket vs cheek contrast
	if upperBandCount > 0 && midBandCount > 0 {
		upperMean := upperBandLumSum / float64(upperBandCount)
		midMean := midBandLumSum / float64(midBandCount)
		if midMean-upperMean >= 8.0 && stdDev >= 15.0 {
			return true
		}
	}

	return false
}

func isSkinTonePixel(r, g, b uint8) bool {
	ycbcr := color.RGBToYCbCr(r, g, b)
	// Standard human skin tone locus in YCbCr space across diverse skin tones
	if ycbcr.Y >= 35 && ycbcr.Cb >= 77 && ycbcr.Cb <= 127 && ycbcr.Cr >= 133 && ycbcr.Cr <= 173 {
		return true
	}
	// Additional RGB heuristic for warm/dark skin tones under varied lighting
	maxRGB := r
	if g > maxRGB {
		maxRGB = g
	}
	if b > maxRGB {
		maxRGB = b
	}
	minRGB := r
	if g < minRGB {
		minRGB = g
	}
	if b < minRGB {
		minRGB = b
	}
	if r > 60 && g > 30 && b > 15 && (maxRGB-minRGB) > 12 && r > g && r > b && (int(r)-int(g)) >= 10 {
		return true
	}
	return false
}
