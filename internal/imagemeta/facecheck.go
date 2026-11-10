package imagemeta

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
)

// BlurImage applies a box blur to the provided image bytes and returns
// the re-encoded JPEG. This is used to obscure developer profile photos
// for non-logged-in visitors.
func BlurImage(data []byte, blurRadius int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	blurred := boxBlur(src, blurRadius)

	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, blurred, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// boxBlur performs a separable box blur (horizontal + vertical pass).
// Pure Go, no C bindings required.
func boxBlur(src image.Image, radius int) image.Image {
	b := src.Bounds()
	w := b.Dx()
	h := b.Dy()

	if radius < 1 {
		radius = 1
	}

	// Convert to RGBA for uniform processing
	rgba := image.NewRGBA(b)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			rgba.SetRGBA(b.Min.X+x, b.Min.Y+y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)})
		}
	}

	// Horizontal pass
	tmp := make([]uint32, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var rSum, gSum, bSum, aSum uint32
			count := 0
			for dx := -radius; dx <= radius; dx++ {
				xx := x + dx
				if xx < 0 || xx >= w {
					continue
				}
				offset := (y*w + xx) * 4
				rSum += uint32(rgba.Pix[offset])
				gSum += uint32(rgba.Pix[offset+1])
				bSum += uint32(rgba.Pix[offset+2])
				aSum += uint32(rgba.Pix[offset+3])
				count++
			}
			offset := (y*w + x) * 4
			tmp[offset] = rSum / uint32(count)
			tmp[offset+1] = gSum / uint32(count)
			tmp[offset+2] = bSum / uint32(count)
			tmp[offset+3] = aSum / uint32(count)
		}
	}

	// Vertical pass
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var rSum, gSum, bSum, aSum uint32
			count := 0
			for dy := -radius; dy <= radius; dy++ {
				yy := y + dy
				if yy < 0 || yy >= h {
					continue
				}
				offset := (yy*w + x) * 4
				rSum += tmp[offset]
				gSum += tmp[offset+1]
				bSum += tmp[offset+2]
				aSum += tmp[offset+3]
				count++
			}
			offset := (y*w + x) * 4
			rgba.Pix[offset] = uint8(rSum / uint32(count))
			rgba.Pix[offset+1] = uint8(gSum / uint32(count))
			rgba.Pix[offset+2] = uint8(bSum / uint32(count))
			rgba.Pix[offset+3] = uint8(aSum / uint32(count))
		}
	}

	return rgba
}

// HasLikelyFace performs a lightweight heuristic check on image bytes to
// determine whether the image likely contains a human face. It uses a
// skin-tone pixel ratio test: if a significant portion of the central
// region contains pixels within typical human skin-tone ranges, the image
// is flagged as likely containing a face.
//
// This is intentionally simple (no C bindings, no ML model) and serves as
// a spam filter — it will not catch every face, but it will reject obvious
// non-face images (logos, landscapes, solid colors).
func HasLikelyFace(data []byte) (bool, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return false, err
	}

	b := src.Bounds()
	w := b.Dx()
	h := b.Dy()

	// Check the central 60% of the image (where a face would typically be)
	cx1 := w * 20 / 100
	cy1 := h * 20 / 100
	cx2 := w * 80 / 100
	y2 := h * 80 / 100

	skinPixels := 0
	totalPixels := 0

	for y := cy1; y < y2; y += 2 {
		for x := cx1; x < cx2; x += 2 {
			r, g, bl, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			r = uint32(r >> 8)
			g = uint32(g >> 8)
			bl = uint32(bl >> 8)

			totalPixels++

			// Simple skin-tone detection in RGB space.
			// Covers a range of human skin tones:
			// - R > 95, G > 40, B > 20 (basic brightness)
			// - R > G > B (skin is warmer than blue)
			// - R - B > 15 (redness over blue)
			// - |R - G| < 180 (not too saturated)
			if r > 95 && g > 40 && bl > 20 &&
				r > g && g > bl &&
				int(r)-int(bl) > 15 &&
				abs(int(r)-int(g)) < 180 {
				skinPixels++
			}
		}
	}

	if totalPixels == 0 {
		return false, nil
	}

	ratio := float64(skinPixels) / float64(totalPixels)
	// If more than 8% of the central region is skin-toned, likely a face
	return ratio > 0.08, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Ensure png import is used (for potential PNG input)
var _ = png.Decode
