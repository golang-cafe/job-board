package facedetect

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
)

// DefaultBlurRadius is the default kernel radius used when blurring developer
// profile images on the backend for unauthenticated visitors.
const DefaultBlurRadius = 12

// ErrInvalidImage is returned when image bytes cannot be decoded.
var ErrInvalidImage = errors.New("facedetect: unable to decode image")

// ContainsHumanFaceFromBytes decodes raw PNG/JPEG bytes and reports whether the
// image contains a human face.
func ContainsHumanFaceFromBytes(data []byte) (bool, error) {
	if len(data) == 0 {
		return false, ErrInvalidImage
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return false, err
	}
	return ContainsHumanFace(img), nil
}

// ContainsHumanFace analyzes an image using multi-cue facial geometry,
// anthropometric luminance contrast, and broad Fitzpatrick-scale skin-tone
// distribution in pure Go without requiring external binaries or Cgo.
func ContainsHumanFace(img image.Image) bool {
	if img == nil {
		return false
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w < 24 || h < 24 {
		return false
	}

	const normSize = 64
	lum := make([][]float64, normSize)
	skin := make([][]bool, normSize)
	for y := 0; y < normSize; y++ {
		lum[y] = make([]float64, normSize)
		skin[y] = make([]bool, normSize)
	}

	var sumL, sumL2 float64
	var skinCount, colorfulCount int
	totalPixels := float64(normSize * normSize)

	for ny := 0; ny < normSize; ny++ {
		sy := bounds.Min.Y + (ny*h)/normSize
		for nx := 0; nx < normSize; nx++ {
			sx := bounds.Min.X + (nx*w)/normSize
			r16, g16, b16, a16 := img.At(sx, sy).RGBA()
			if a16 < 0x2000 {
				continue
			}
			r := float64(r16 >> 8)
			g := float64(g16 >> 8)
			b := float64(b16 >> 8)

			yVal := 0.299*r + 0.587*g + 0.114*b
			lum[ny][nx] = yVal
			sumL += yVal
			sumL2 += yVal * yVal

			maxC := math.Max(r, math.Max(g, b))
			minC := math.Min(r, math.Min(g, b))
			if maxC-minC > 12 {
				colorfulCount++
			}

			if isHumanSkinPixel(r, g, b) {
				skin[ny][nx] = true
				skinCount++
			}
		}
	}

	meanL := sumL / totalPixels
	varianceL := (sumL2 / totalPixels) - (meanL * meanL)
	stdL := math.Sqrt(math.Max(0, varianceL))

	// Reject flat, solid-color or uniform graphics.
	if stdL < 8.0 {
		return false
	}

	intLum := buildIntegralFloat(lum, normSize, normSize)
	intSkin := buildIntegralBool(skin, normSize, normSize)

	isGrayscale := float64(colorfulCount)/totalPixels < 0.08

	// Multi-scale candidate window evaluation across scale factors.
	scales := []int{24, 32, 40, 48, 56}
	for _, winSize := range scales {
		step := 3
		for wy := 2; wy+winSize <= normSize-2; wy += step {
			for wx := 2; wx+winSize <= normSize-2; wx += step {
				if evaluateFaceWindow(lum, intLum, intSkin, wx, wy, winSize, isGrayscale) {
					return true
				}
			}
		}
	}

	return false
}

// isHumanSkinPixel detects human skin tones across diverse complexions
// (light, medium, olive, brown, and dark tones) using YCbCr + RGB rules.
func isHumanSkinPixel(r, g, b float64) bool {
	y := 16.0 + (65.481*r+128.553*g+24.966*b)/255.0
	cb := 128.0 + (-37.797*r-74.203*g+112.0*b)/255.0
	cr := 128.0 + (112.0*r-93.786*g-18.214*b)/255.0

	if y < 20 || y > 250 {
		return false
	}

	// Broad Fitzpatrick YCbCr skin cluster.
	if cb >= 75 && cb <= 135 && cr >= 130 && cr <= 180 {
		if r >= g*0.80 && (r-b) >= 6 {
			return true
		}
	}

	// Deep/dark brown skin cluster.
	if r > g && g > b && (r-b) >= 10 {
		if cr >= 125 && cb <= 140 {
			return true
		}
	}

	return false
}

// buildIntegralFloat constructs a 2D integral image for rapid area sums in O(1).
func buildIntegralFloat(src [][]float64, h, w int) [][]float64 {
	ii := make([][]float64, h+1)
	for y := 0; y <= h; y++ {
		ii[y] = make([]float64, w+1)
	}
	for y := 0; y < h; y++ {
		var rowSum float64
		for x := 0; x < w; x++ {
			rowSum += src[y][x]
			ii[y+1][x+1] = ii[y][x+1] + rowSum
		}
	}
	return ii
}

// buildIntegralBool constructs a 2D integral image for boolean skin masks.
func buildIntegralBool(src [][]bool, h, w int) [][]int {
	ii := make([][]int, h+1)
	for y := 0; y <= h; y++ {
		ii[y] = make([]int, w+1)
	}
	for y := 0; y < h; y++ {
		rowSum := 0
		for x := 0; x < w; x++ {
			if src[y][x] {
				rowSum++
			}
			ii[y+1][x+1] = ii[y][x+1] + rowSum
		}
	}
	return ii
}

func rectSumFloat(ii [][]float64, x1, y1, x2, y2 int) float64 {
	return ii[y2][x2] - ii[y1][x2] - ii[y2][x1] + ii[y1][x1]
}

func rectSumInt(ii [][]int, x1, y1, x2, y2 int) int {
	return ii[y2][x2] - ii[y1][x2] - ii[y2][x1] + ii[y1][x1]
}

// evaluateFaceWindow tests a candidate bounding box against anthropometric facial invariants.
func evaluateFaceWindow(lum [][]float64, intLum [][]float64, intSkin [][]int, wx, wy, size int, isGrayscale bool) bool {
	winArea := float64(size * size)

	// 1. Skin tone density check (for color images)
	if !isGrayscale {
		skinPixels := rectSumInt(intSkin, wx, wy, wx+size, wy+size)
		skinRatio := float64(skinPixels) / winArea
		if skinRatio < 0.12 {
			return false
		}
	}

	// 2. Eye region vs Forehead region (eye valley feature)
	// Eyes sit at 30%-55% vertical, Forehead at 10%-30%
	yForeheadTop := wy + int(float64(size)*0.10)
	yForeheadBottom := wy + int(float64(size)*0.30)
	yEyeTop := wy + int(float64(size)*0.32)
	yEyeBottom := wy + int(float64(size)*0.55)

	xInset := int(float64(size) * 0.15)
	xLeft := wx + xInset
	xRight := wx + size - xInset

	foreheadArea := float64((yForeheadBottom - yForeheadTop) * (xRight - xLeft))
	eyeArea := float64((yEyeBottom - yEyeTop) * (xRight - xLeft))
	if foreheadArea <= 0 || eyeArea <= 0 {
		return false
	}

	foreheadLum := rectSumFloat(intLum, xLeft, yForeheadTop, xRight, yForeheadBottom) / foreheadArea
	eyeLum := rectSumFloat(intLum, xLeft, yEyeTop, xRight, yEyeBottom) / eyeArea

	// Eyes/brows are consistently darker than the upper forehead in human portraits
	if eyeLum >= foreheadLum*1.08 && !isGrayscale {
		return false
	}

	// 3. Horizontal Facial Symmetry (left half vs right half contrast)
	midX := wx + size/2
	halfWidth := size / 2
	halfArea := float64(size * halfWidth)
	if halfArea <= 0 {
		return false
	}

	leftLum := rectSumFloat(intLum, wx, wy, midX, wy+size) / halfArea
	rightLum := rectSumFloat(intLum, midX, wy, wx+size, wy+size) / halfArea

	maxHalf := math.Max(leftLum, rightLum)
	if maxHalf > 0 {
		diffRatio := math.Abs(leftLum-rightLum) / maxHalf
		// Faces exhibit bilateral symmetry; reject extreme asymmetric shadows
		if diffRatio > 0.45 {
			return false
		}
	}

	// 4. Central Nose/Bridge region has contrast compared to outer cheek margins
	midY := wy + int(float64(size)*0.40)
	bottomY := wy + int(float64(size)*0.80)
	centerLeft := wx + int(float64(size)*0.35)
	centerRight := wx + int(float64(size)*0.65)
	centerArea := float64((bottomY - midY) * (centerRight - centerLeft))

	if centerArea > 0 {
		centerLum := rectSumFloat(intLum, centerLeft, midY, centerRight, bottomY) / centerArea
		totalWinLum := rectSumFloat(intLum, wx, wy, wx+size, wy+size) / winArea
		// Center face region shouldn't be pitch black or completely washed out
		if centerLum < 15.0 || totalWinLum < 15.0 {
			return false
		}
	}

	return true
}

// BlurImage applies a fast separable box blur in pure Go, preserving image bounds
// and alpha channels without any external binary dependencies.
func BlurImage(src image.Image, radius int) image.Image {
	if src == nil {
		return nil
	}
	if radius <= 0 {
		radius = DefaultBlurRadius
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return src
	}

	tmp := image.NewRGBA(b)
	dst := image.NewRGBA(b)

	// Horizontal pass
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			var rSum, gSum, bSum, aSum, count uint32
			for dx := -radius; dx <= radius; dx++ {
				nx := x + dx
				if nx < b.Min.X {
					nx = b.Min.X
				} else if nx >= b.Max.X {
					nx = b.Max.X - 1
				}
				r, g, bl, a := src.At(nx, y).RGBA()
				rSum += r >> 8
				gSum += g >> 8
				bSum += bl >> 8
				aSum += a >> 8
				count++
			}
			tmp.SetRGBA(x, y, color.RGBA{
				R: uint8(rSum / count),
				G: uint8(gSum / count),
				B: uint8(bSum / count),
				A: uint8(aSum / count),
			})
		}
	}

	// Vertical pass
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			var rSum, gSum, bSum, aSum, count uint32
			for dy := -radius; dy <= radius; dy++ {
				ny := y + dy
				if ny < b.Min.Y {
					ny = b.Min.Y
				} else if ny >= b.Max.Y {
					ny = b.Max.Y - 1
				}
				c := tmp.RGBAAt(x, ny)
				rSum += uint32(c.R)
				gSum += uint32(c.G)
				bSum += uint32(c.B)
				aSum += uint32(c.A)
				count++
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(rSum / count),
				G: uint8(gSum / count),
				B: uint8(bSum / count),
				A: uint8(aSum / count),
			})
		}
	}

	return dst
}

// BlurImageBytes decodes media bytes, applies the pure-Go blur, and re-encodes
// back to JPEG or PNG according to mediaType.
func BlurImageBytes(data []byte, mediaType string, radius int) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrInvalidImage
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	blurred := BlurImage(src, radius)
	outBuf := new(bytes.Buffer)
	switch mediaType {
	case "image/png":
		if err := png.Encode(outBuf, blurred); err != nil {
			return nil, err
		}
	default:
		if err := jpeg.Encode(outBuf, blurred, &jpeg.Options{Quality: 85}); err != nil {
			return nil, err
		}
	}
	return outBuf.Bytes(), nil
}
