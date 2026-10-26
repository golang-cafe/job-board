package imagemeta

import (
	"image"
	"image/color"
	"math"
)

const (
	gridSize       = 64
	minStdDev      = 12.0
	minSymmetry    = 0.55
	defaultBlurRad = 8
)

// HasHumanFace evaluates whether an uploaded avatar image contains a human face
// using a pure-Go integral-image Haar cascade and portrait structure check.
func HasHumanFace(img image.Image) bool {
	if img == nil {
		return false
	}
	b := img.Bounds()
	if b.Dx() < 24 || b.Dy() < 24 {
		return false
	}
	luma, skinRatio := extractNormalizedGrid(img)
	mean, stdDev := computeGridStats(luma)
	if stdDev < minStdDev || mean < 15.0 || mean > 245.0 {
		return false
	}
	symmetry := computeHorizontalSymmetry(luma)
	if symmetry < minSymmetry {
		return false
	}
	integral := buildIntegralImage(luma)
	haarMatched := scanHaarFaceWindows(integral)
	return haarMatched && (skinRatio >= 0.08 || stdDev >= 22.0)
}

func extractNormalizedGrid(img image.Image) ([]float64, float64) {
	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()
	luma := make([]float64, gridSize*gridSize)
	skinPixels := 0
	totalCenter := 0
	for gy := 0; gy < gridSize; gy++ {
		sy := b.Min.Y + (gy*h)/gridSize
		for gx := 0; gx < gridSize; gx++ {
			sx := b.Min.X + (gx*w)/gridSize
			r16, g16, b16, _ := img.At(sx, sy).RGBA()
			rf := float64(r16 >> 8)
			gf := float64(g16 >> 8)
			bf := float64(b16 >> 8)
			luma[gy*gridSize+gx] = 0.299*rf + 0.587*gf + 0.114*bf
			if gx >= 12 && gx < 52 && gy >= 10 && gy < 54 {
				totalCenter++
				if isPortraitTone(rf, gf, bf) {
					skinPixels++
				}
			}
		}
	}
	if totalCenter == 0 {
		return luma, 0
	}
	return luma, float64(skinPixels) / float64(totalCenter)
}

func isPortraitTone(r, g, b float64) bool {
	cb := 128.0 - 0.168736*r - 0.331264*g + 0.5*b
	cr := 128.0 + 0.5*r - 0.418688*g - 0.081312*b
	return cb >= 75.0 && cb <= 140.0 && cr >= 128.0 && cr <= 180.0
}

func computeGridStats(luma []float64) (float64, float64) {
	n := float64(len(luma))
	if n == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, v := range luma {
		sum += v
	}
	mean := sum / n
	varSum := 0.0
	for _, v := range luma {
		d := v - mean
		varSum += d * d
	}
	return mean, math.Sqrt(varSum / n)
}

func computeHorizontalSymmetry(luma []float64) float64 {
	diffSum := 0.0
	count := 0.0
	for y := 8; y < 56; y++ {
		for x := 8; x < 32; x++ {
			left := luma[y*gridSize+x]
			right := luma[y*gridSize+(gridSize-1-x)]
			diffSum += math.Abs(left - right)
			count += 1.0
		}
	}
	if count == 0 {
		return 0
	}
	avgDiff := diffSum / count
	score := 1.0 - (avgDiff / 128.0)
	if score < 0 {
		return 0
	}
	return score
}

func buildIntegralImage(luma []float64) []float64 {
	stride := gridSize + 1
	integral := make([]float64, stride*stride)
	for y := 1; y <= gridSize; y++ {
		rowSum := 0.0
		for x := 1; x <= gridSize; x++ {
			rowSum += luma[(y-1)*gridSize+(x-1)]
			integral[y*stride+x] = integral[(y-1)*stride+x] + rowSum
		}
	}
	return integral
}

func rectAvg(integral []float64, x1, y1, x2, y2 int) float64 {
	stride := gridSize + 1
	area := float64((x2 - x1) * (y2 - y1))
	if area <= 0 {
		return 0
	}
	sum := integral[y2*stride+x2] - integral[y1*stride+x2] - integral[y2*stride+x1] + integral[y1*stride+x1]
	return sum / area
}

func scanHaarFaceWindows(integral []float64) bool {
	sizes := []int{28, 36, 44}
	for _, sz := range sizes {
		for wy := 4; wy+sz <= gridSize-4; wy += 4 {
			for wx := 4; wx+sz <= gridSize-4; wx += 4 {
				if evalHaarWindow(integral, wx, wy, sz) {
					return true
				}
			}
		}
	}
	return false
}

func evalHaarWindow(integral []float64, wx, wy, sz int) bool {
	foreheadY1 := wy + sz/8
	foreheadY2 := wy + (sz*3)/10
	eyeY1 := foreheadY2
	eyeY2 := wy + (sz*5)/10
	cheekY1 := eyeY2
	cheekY2 := wy + (sz*7)/10
	xLeft := wx + sz/6
	xRight := wx + sz - sz/6
	xMidL := wx + (sz*2)/5
	xMidR := wx + (sz*3)/5

	eyeAvg := rectAvg(integral, xLeft, eyeY1, xRight, eyeY2)
	cheekAvg := rectAvg(integral, xLeft, cheekY1, xRight, cheekY2)
	foreheadAvg := rectAvg(integral, xLeft, foreheadY1, xRight, foreheadY2)
	bridgeAvg := rectAvg(integral, xMidL, eyeY1, xMidR, eyeY2)

	eyesDarkerThanCheeks := cheekAvg-eyeAvg >= 2.5
	eyesDarkerThanForehead := foreheadAvg-eyeAvg >= 1.5
	bridgeContrast := bridgeAvg-eyeAvg >= 1.0
	return (eyesDarkerThanCheeks && eyesDarkerThanForehead) || (eyesDarkerThanCheeks && bridgeContrast)
}

// BlurImage applies a multi-pass separable box blur to obscure developer profile images for unauthenticated users.
func BlurImage(src image.Image, radius int) image.Image {
	if src == nil {
		return nil
	}
	if radius <= 0 {
		radius = defaultBlurRad
	}
	b := src.Bounds()
	w := b.Dx()
	h := b.Dy()
	curr := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			curr.Set(x, y, color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)))
		}
	}
	for pass := 0; pass < 3; pass++ {
		horiz := blurHorizontal(curr, w, h, radius)
		curr = blurVertical(horiz, w, h, radius)
	}
	return curr
}

func blurHorizontal(src *image.NRGBA, w, h, radius int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var rSum, gSum, bSum, aSum, count uint32
			for k := -radius; k <= radius; k++ {
				nx := clampInt(x+k, 0, w-1)
				c := src.NRGBAAt(nx, y)
				rSum += uint32(c.R)
				gSum += uint32(c.G)
				bSum += uint32(c.B)
				aSum += uint32(c.A)
				count++
			}
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(rSum / count),
				G: uint8(gSum / count),
				B: uint8(bSum / count),
				A: uint8(aSum / count),
			})
		}
	}
	return dst
}

func blurVertical(src *image.NRGBA, w, h, radius int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var rSum, gSum, bSum, aSum, count uint32
			for k := -radius; k <= radius; k++ {
				ny := clampInt(y+k, 0, h-1)
				c := src.NRGBAAt(x, ny)
				rSum += uint32(c.R)
				gSum += uint32(c.G)
				bSum += uint32(c.B)
				aSum += uint32(c.A)
				count++
			}
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(rSum / count),
				G: uint8(gSum / count),
				B: uint8(bSum / count),
				A: uint8(aSum / count),
			})
		}
	}
	return dst
}

func clampInt(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}
