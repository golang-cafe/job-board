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
// profile images for unauthenticated visitors.
const DefaultBlurRadius = 8

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

// ContainsHumanFace analyzes an image using a multi-cue Viola-Jones / Haar-like
// facial structure detector combined with YCbCr/RGB skin-tone & grayscale
// portrait analysis in pure Go.
func ContainsHumanFace(img image.Image) bool {
	if img == nil {
		return false
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 16 || h < 16 {
		return false
	}

	const normSize = 64
	lum := make([][]float64, normSize)
	skin := make([][]bool, normSize)
	sat := make([][]float64, normSize)
	for y := 0; y < normSize; y++ {
		lum[y] = make([]float64, normSize)
		skin[y] = make([]bool, normSize)
		sat[y] = make([]float64, normSize)
	}

	var sumL, sumL2 float64
	var skinCount, colorfulCount int
	totalPixels := float64(normSize * normSize)

	for ny := 0; ny < normSize; ny++ {
		sy := b.Min.Y + (ny*h)/normSize
		for nx := 0; nx < normSize; nx++ {
			sx := b.Min.X + (nx*w)/normSize
			r16, g16, bl16, a16 := img.At(sx, sy).RGBA()
			if a16 < 0x2000 {
				continue
			}
			r := float64(r16 >> 8)
			g := float64(g16 >> 8)
			bl := float64(bl16 >> 8)

			yVal := 0.299*r + 0.587*g + 0.114*bl
			lum[ny][nx] = yVal
			sumL += yVal
			sumL2 += yVal * yVal

			maxC := math.Max(r, math.Max(g, bl))
			minC := math.Min(r, math.Min(g, bl))
			if maxC > 0 {
				sat[ny][nx] = (maxC - minC) / maxC
			}
			if maxC-minC > 12 {
				colorfulCount++
			}

			if isHumanSkinPixel(r, g, bl) {
				skin[ny][nx] = true
				skinCount++
			}
		}
	}

	meanL := sumL / totalPixels
	varianceL := (sumL2 / totalPixels) - (meanL * meanL)
	stdL := math.Sqrt(math.Max(0, varianceL))

	// Reject flat / solid / nearly uniform images.
	if stdL < 8.0 {
		return false
	}

	// Compute integral images for fast Haar-like region evaluation.
	intLum := buildIntegralFloat(lum, normSize, normSize)
	intSkin := buildIntegralBool(skin, normSize, normSize)

	isGrayscalePortrait := float64(colorfulCount)/totalPixels < 0.08

	// Scan candidate face windows across scales and positions.
	scales := []int{24, 30, 36, 44, 52}
	for _, winSize := range scales {
		step := 3
		for wy := 2; wy+winSize <= normSize-2; wy += step {
			for wx := 2; wx+winSize <= normSize-2; wx += step {
				if evaluateFaceWindow(lum, intLum, intSkin, wx, wy, winSize, isGrayscalePortrait) {
					return true
				}
			}
		}
	}

	return false
}

// isHumanSkinPixel detects human skin tones across diverse Fitzpatrick types
// (very fair to deep dark brown/ebony) using YCbCr + RGB chromaticity rules.
func isHumanSkinPixel(r, g, b float64) bool {
	y := 16.0 + (65.481*r+128.553*g+24.966*b)/255.0
	cb := 128.0 + (-37.797*r-74.203*g+112.0*b)/255.0
	cr := 128.0 + (112.0*r-93.786*g-18.214*b)/255.0

	if y < 25 || y > 248 {
		return false
	}
	// Broad YCbCr skin cluster covering light, medium, olive, and dark skin tones.
	if cb >= 75 && cb <= 135 && cr >= 130 && cr <= 180 {
		if r >= g*0.85 && g >= b*0.75 && (r-b) >= 8 {
			return true
		}
	}
	// Deep / dark skin tone cluster where luminance is lower and warm red/yellow dominates blue.
	if y >= 25 && y <= 120 && cb >= 85 && cb <= 140 && cr >= 122 && cr <= 165 {
		if r > g && g >= b && (r-b) >= 6 {
			return true
		}
	}
	return false
}

func evaluateFaceWindow(
	lum [][]float64,
	intLum [][]float64,
	intSkin [][]int,
	wx, wy, w int,
	isGrayscale bool,
) bool {
	area := float64(w * w)
	winMean := rectSumFloat(intLum, wx, wy, wx+w, wy+w) / area
	if winMean < 18 || winMean > 242 {
		return false
	}

	// 1. Skin coverage inside the candidate face box (for color images).
	skinPixels := rectSumInt(intSkin, wx, wy, wx+w, wy+w)
	skinRatio := float64(skinPixels) / area
	if !isGrayscale && skinRatio < 0.22 {
		return false
	}

	// 2. Local variance & edge density inside the window:
	// A real face has internal contrast (eyes, nose, mouth, hairline) but is not pure binary text/checkerboard.
	var varSum, gradSum float64
	var highGradCount int
	for y := wy; y < wy+w; y++ {
		for x := wx; x < wx+w; x++ {
			d := lum[y][x] - winMean
			varSum += d * d
			if x+1 < wx+w && y+1 < wy+w {
				gx := math.Abs(lum[y][x+1] - lum[y][x])
				gy := math.Abs(lum[y+1][x] - lum[y][x])
				g := gx + gy
				gradSum += g
				if g > 14 {
					highGradCount++
				}
			}
		}
	}
	winStd := math.Sqrt(varSum / area)
	if winStd < 9.0 || winStd > 95.0 {
		return false
	}
	edgeRatio := float64(highGradCount) / area
	if edgeRatio < 0.04 || edgeRatio > 0.85 {
		return false
	}

	// 3. Haar-like facial geometry features:
	// Divide window vertically into:
	// - Forehead: [0.08 .. 0.28]
	// - Eye band: [0.25 .. 0.48]
	// - Cheek/Nose bridge band: [0.48 .. 0.68]
	// - Mouth/Chin band: [0.68 .. 0.90]
	x1 := wx + w/8
	x2 := wx + w - w/8
	if x2 <= x1+2 {
		return false
	}

	foreY1, foreY2 := wy+int(0.08*float64(w)), wy+int(0.26*float64(w))
	eyeY1, eyeY2 := wy+int(0.26*float64(w)), wy+int(0.48*float64(w))
	cheekY1, cheekY2 := wy+int(0.48*float64(w)), wy+int(0.68*float64(w))
	mouthY1, mouthY2 := wy+int(0.68*float64(w)), wy+int(0.90*float64(w))

	foreMean := rectMeanFloat(intLum, x1, foreY1, x2, foreY2)
	eyeMean := rectMeanFloat(intLum, x1, eyeY1, x2, eyeY2)
	cheekMean := rectMeanFloat(intLum, x1, cheekY1, x2, cheekY2)
	mouthMean := rectMeanFloat(intLum, x1, mouthY1, x2, mouthY2)

	// Eye region is characteristically darker than cheeks/nose bridge or forehead,
	// or has distinct left/right ocular sockets separated by a brighter nasal bridge.
	leftEyeX1, leftEyeX2 := wx+int(0.15*float64(w)), wx+int(0.42*float64(w))
	bridgeX1, bridgeX2 := wx+int(0.42*float64(w)), wx+int(0.58*float64(w))
	rightEyeX1, rightEyeX2 := wx+int(0.58*float64(w)), wx+int(0.85*float64(w))

	leftEyeMean := rectMeanFloat(intLum, leftEyeX1, eyeY1, leftEyeX2, eyeY2)
	bridgeMean := rectMeanFloat(intLum, bridgeX1, eyeY1, bridgeX2, eyeY2)
	rightEyeMean := rectMeanFloat(intLum, rightEyeX1, eyeY1, rightEyeX2, eyeY2)

	eyeVsCheek := cheekMean - eyeMean
	eyeVsForehead := foreMean - eyeMean
	bridgeVsEyes := bridgeMean - 0.5*(leftEyeMean+rightEyeMean)
	cheekVsMouth := cheekMean - mouthMean

	haarScore := 0
	if eyeVsCheek >= 2.5 {
		haarScore++
	}
	if eyeVsForehead >= 2.0 {
		haarScore++
	}
	if bridgeVsEyes >= 1.5 {
		haarScore++
	}
	if cheekVsMouth >= 1.5 {
		haarScore++
	}

	if haarScore < 2 {
		return false
	}

	// 4. Bilateral horizontal symmetry check between left and right halves of the face window.
	var symDiff float64
	var symCount int
	half := w / 2
	for y := wy + w/8; y < wy+w-w/8; y++ {
		for dx := 0; dx < half; dx++ {
			lx := wx + dx
			rx := wx + w - 1 - dx
			symDiff += math.Abs(lum[y][lx] - lum[y][rx])
			symCount++
		}
	}
	if symCount > 0 {
		meanSymDiff := symDiff / float64(symCount)
		if meanSymDiff > 55.0 {
			return false
		}
	}

	return true
}

func buildIntegralFloat(grid [][]float64, w, h int) [][]float64 {
	integral := make([][]float64, h+1)
	for y := 0; y <= h; y++ {
		integral[y] = make([]float64, w+1)
	}
	for y := 1; y <= h; y++ {
		var rowSum float64
		for x := 1; x <= w; x++ {
			rowSum += grid[y-1][x-1]
			integral[y][x] = integral[y-1][x] + rowSum
		}
	}
	return integral
}

func buildIntegralBool(grid [][]bool, w, h int) [][]int {
	integral := make([][]int, h+1)
	for y := 0; y <= h; y++ {
		integral[y] = make([]int, w+1)
	}
	for y := 1; y <= h; y++ {
		var rowSum int
		for x := 1; x <= w; x++ {
			if grid[y-1][x-1] {
				rowSum++
			}
			integral[y][x] = integral[y-1][x] + rowSum
		}
	}
	return integral
}

func rectSumFloat(integral [][]float64, x1, y1, x2, y2 int) float64 {
	if x2 <= x1 || y2 <= y1 {
		return 0
	}
	return integral[y2][x2] - integral[y1][x2] - integral[y2][x1] + integral[y1][x1]
}

func rectMeanFloat(integral [][]float64, x1, y1, x2, y2 int) float64 {
	if x2 <= x1 || y2 <= y1 {
		return 0
	}
	area := float64((x2 - x1) * (y2 - y1))
	return rectSumFloat(integral, x1, y1, x2, y2) / area
}

func rectSumInt(integral [][]int, x1, y1, x2, y2 int) int {
	if x2 <= x1 || y2 <= y1 {
		return 0
	}
	return integral[y2][x2] - integral[y1][x2] - integral[y2][x1] + integral[y1][x1]
}

// BlurImage applies a 3-pass separable box blur (approximating a Gaussian blur)
// to obscure facial details for non-logged-in visitors.
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

	cur := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cur.Set(x, y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}

	tmp := image.NewNRGBA(image.Rect(0, 0, w, h))
	for pass := 0; pass < 3; pass++ {
		boxBlurHorizontal(cur, tmp, w, h, radius)
		boxBlurVertical(tmp, cur, w, h, radius)
	}
	return cur
}

// BlurImageBytes decodes an image, applies BlurImage, and re-encodes it into
// the specified mediaType ("image/png", "image/jpeg", or "image/jpg").
func BlurImageBytes(data []byte, mediaType string, radius int) ([]byte, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	blurred := BlurImage(img, radius)
	buf := new(bytes.Buffer)
	switch mediaType {
	case "image/png":
		if err := png.Encode(buf, blurred); err != nil {
			return nil, err
		}
	case "image/jpeg", "image/jpg":
		if err := jpeg.Encode(buf, blurred, &jpeg.Options{Quality: 85}); err != nil {
			return nil, err
		}
	default:
		if format == "png" {
			if err := png.Encode(buf, blurred); err != nil {
				return nil, err
			}
		} else {
			if err := jpeg.Encode(buf, blurred, &jpeg.Options{Quality: 85}); err != nil {
				return nil, err
			}
		}
	}
	return buf.Bytes(), nil
}

func boxBlurHorizontal(src, dst *image.NRGBA, w, h, radius int) {
	kernelSize := uint32(2*radius + 1)
	for y := 0; y < h; y++ {
		var sumR, sumG, sumB, sumA uint32
		for k := -radius; k <= radius; k++ {
			cx := clampIndex(k, w)
			c := src.NRGBAAt(cx, y)
			sumR += uint32(c.R)
			sumG += uint32(c.G)
			sumB += uint32(c.B)
			sumA += uint32(c.A)
		}
		for x := 0; x < w; x++ {
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(sumR / kernelSize),
				G: uint8(sumG / kernelSize),
				B: uint8(sumB / kernelSize),
				A: uint8(sumA / kernelSize),
			})
			leftIdx := clampIndex(x-radius, w)
			rightIdx := clampIndex(x+radius+1, w)
			leftC := src.NRGBAAt(leftIdx, y)
			rightC := src.NRGBAAt(rightIdx, y)
			sumR = sumR - uint32(leftC.R) + uint32(rightC.R)
			sumG = sumG - uint32(leftC.G) + uint32(rightC.G)
			sumB = sumB - uint32(leftC.B) + uint32(rightC.B)
			sumA = sumA - uint32(leftC.A) + uint32(rightC.A)
		}
	}
}

func boxBlurVertical(src, dst *image.NRGBA, w, h, radius int) {
	kernelSize := uint32(2*radius + 1)
	for x := 0; x < w; x++ {
		var sumR, sumG, sumB, sumA uint32
		for k := -radius; k <= radius; k++ {
			cy := clampIndex(k, h)
			c := src.NRGBAAt(x, cy)
			sumR += uint32(c.R)
			sumG += uint32(c.G)
			sumB += uint32(c.B)
			sumA += uint32(c.A)
		}
		for y := 0; y < h; y++ {
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(sumR / kernelSize),
				G: uint8(sumG / kernelSize),
				B: uint8(sumB / kernelSize),
				A: uint8(sumA / kernelSize),
			})
			topIdx := clampIndex(y-radius, h)
			botIdx := clampIndex(y+radius+1, h)
			topC := src.NRGBAAt(x, topIdx)
			botC := src.NRGBAAt(x, botIdx)
			sumR = sumR - uint32(topC.R) + uint32(botC.R)
			sumG = sumG - uint32(topC.G) + uint32(botC.G)
			sumB = sumB - uint32(topC.B) + uint32(botC.B)
			sumA = sumA - uint32(topC.A) + uint32(botC.A)
		}
	}
}

func clampIndex(idx, maxLen int) int {
	if idx < 0 {
		return 0
	}
	if idx >= maxLen {
		return maxLen - 1
	}
	return idx
}
