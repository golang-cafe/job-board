package facedetect

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"
)

func generateSyntheticFaceImage(w, h int, skinColor color.NRGBA) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	bg := color.NRGBA{R: 230, G: 235, B: 245, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, bg)
		}
	}

	cx, cy := float64(w)/2.0, float64(h)/2.0
	rx, ry := float64(w)*0.34, float64(h)*0.40

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx := (float64(x) - cx) / rx
			dy := (float64(y) - cy) / ry
			dist2 := dx*dx + dy*dy
			if dist2 <= 1.0 {
				// Subtle radial shading for realistic 3D facial contour
				shade := 1.0 - 0.15*dist2
				r := uint8(math.Min(255, float64(skinColor.R)*shade))
				g := uint8(math.Min(255, float64(skinColor.G)*shade))
				b := uint8(math.Min(255, float64(skinColor.B)*shade))
				img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: 255})
			}
		}
	}

	// Hair / top crown
	for y := int(cy - ry*1.05); y < int(cy-ry*0.58); y++ {
		for x := int(cx - rx*0.95); x < int(cx+rx*0.95); x++ {
			if x >= 0 && x < w && y >= 0 && y < h {
				img.SetNRGBA(x, y, color.NRGBA{R: 35, G: 28, B: 24, A: 255})
			}
		}
	}

	// Left and right eyes + eyebrows
	eyeY := int(cy - ry*0.18)
	leftEyeX := int(cx - rx*0.36)
	rightEyeX := int(cx + rx*0.36)
	eyeRx := int(math.Max(3, float64(w)*0.07))
	eyeRy := int(math.Max(2, float64(h)*0.045))

	for _, ex := range []int{leftEyeX, rightEyeX} {
		// Eyebrow
		for y := eyeY - eyeRy*2 - 2; y < eyeY-eyeRy; y++ {
			for x := ex - eyeRx - 1; x <= ex+eyeRx+1; x++ {
				if x >= 0 && x < w && y >= 0 && y < h {
					img.SetNRGBA(x, y, color.NRGBA{R: 40, G: 30, B: 25, A: 255})
				}
			}
		}
		// Eye socket & pupil
		for y := eyeY - eyeRy; y <= eyeY+eyeRy; y++ {
			for x := ex - eyeRx; x <= ex+eyeRx; x++ {
				if x >= 0 && x < w && y >= 0 && y < h {
					img.SetNRGBA(x, y, color.NRGBA{R: 25, G: 25, B: 30, A: 255})
				}
			}
		}
	}

	// Nose bridge highlight and nostrils
	for y := eyeY; y < int(cy+ry*0.18); y++ {
		for x := int(cx - 2); x <= int(cx+2); x++ {
			if x >= 0 && x < w && y >= 0 && y < h {
				c := img.NRGBAAt(x, y)
				img.SetNRGBA(x, y, color.NRGBA{
					R: uint8(math.Min(255, float64(c.R)+15)),
					G: uint8(math.Min(255, float64(c.G)+12)),
					B: uint8(math.Min(255, float64(c.B)+10)),
					A: 255,
				})
			}
		}
	}

	// Mouth
	mouthY := int(cy + ry*0.42)
	mouthHalfW := int(rx * 0.38)
	mouthHalfH := int(math.Max(2, float64(h)*0.035))
	for y := mouthY - mouthHalfH; y <= mouthY+mouthHalfH; y++ {
		for x := int(cx) - mouthHalfW; x <= int(cx)+mouthHalfW; x++ {
			if x >= 0 && x < w && y >= 0 && y < h {
				img.SetNRGBA(x, y, color.NRGBA{R: 135, G: 55, B: 55, A: 255})
			}
		}
	}

	return img
}

func TestContainsHumanFace_DetectsDiverseSkinTones(t *testing.T) {
	tones := []struct {
		name  string
		color color.NRGBA
	}{
		{"light_skin", color.NRGBA{R: 235, G: 190, B: 160, A: 255}},
		{"medium_olive_skin", color.NRGBA{R: 198, G: 145, B: 108, A: 255}},
		{"deep_brown_skin", color.NRGBA{R: 125, G: 82, B: 58, A: 255}},
		{"grayscale_portrait", color.NRGBA{R: 190, G: 190, B: 190, A: 255}},
	}

	for _, tc := range tones {
		t.Run(tc.name, func(t *testing.T) {
			faceImg := generateSyntheticFaceImage(128, 128, tc.color)
			if !ContainsHumanFace(faceImg) {
				t.Fatalf("expected ContainsHumanFace to return true for %s", tc.name)
			}

			buf := new(bytes.Buffer)
			if err := png.Encode(buf, faceImg); err != nil {
				t.Fatalf("failed to encode png: %v", err)
			}
			ok, err := ContainsHumanFaceFromBytes(buf.Bytes())
			if err != nil {
				t.Fatalf("unexpected error from ContainsHumanFaceFromBytes: %v", err)
			}
			if !ok {
				t.Fatalf("expected ContainsHumanFaceFromBytes to return true for %s", tc.name)
			}
		})
	}
}

func TestContainsHumanFace_RejectsNonFaceImages(t *testing.T) {
	// 1. Solid color image
	solid := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			solid.SetNRGBA(x, y, color.NRGBA{R: 220, G: 175, B: 145, A: 255})
		}
	}
	if ContainsHumanFace(solid) {
		t.Fatalf("expected solid skin-colored block to be rejected")
	}

	// 2. Blue/green landscape / geometric logo without face structure
	landscape := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			if y < 64 {
				landscape.SetNRGBA(x, y, color.NRGBA{R: 70, G: 140, B: 230, A: 255})
			} else {
				landscape.SetNRGBA(x, y, color.NRGBA{R: 40, G: 160, B: 60, A: 255})
			}
		}
	}
	if ContainsHumanFace(landscape) {
		t.Fatalf("expected landscape image to be rejected")
	}
}

func TestBlurImageBytes_BlursHighFrequencyDetail(t *testing.T) {
	faceImg := generateSyntheticFaceImage(100, 100, color.NRGBA{R: 230, G: 185, B: 155, A: 255})
	origBuf := new(bytes.Buffer)
	if err := png.Encode(origBuf, faceImg); err != nil {
		t.Fatalf("failed to encode png: %v", err)
	}

	blurredBytes, err := BlurImageBytes(origBuf.Bytes(), "image/png", DefaultBlurRadius)
	if err != nil {
		t.Fatalf("BlurImageBytes failed: %v", err)
	}
	blurredImg, _, err := image.Decode(bytes.NewReader(blurredBytes))
	if err != nil {
		t.Fatalf("failed to decode blurred image: %v", err)
	}

	origGrad := averageGradient(faceImg)
	blurGrad := averageGradient(blurredImg)
	if blurGrad >= origGrad*0.65 {
		t.Fatalf("expected blurred image gradient (%.2f) to be significantly lower than original (%.2f)", blurGrad, origGrad)
	}
}

func averageGradient(img image.Image) float64 {
	b := img.Bounds()
	var sum float64
	var count int
	for y := b.Min.Y; y < b.Max.Y-1; y++ {
		for x := b.Min.X; x < b.Max.X-1; x++ {
			r1, g1, b1, _ := img.At(x, y).RGBA()
			r2, g2, b2, _ := img.At(x+1, y).RGBA()
			r3, g3, b3, _ := img.At(x, y+1).RGBA()
			lum1 := 0.299*float64(r1>>8) + 0.587*float64(g1>>8) + 0.114*float64(b1>>8)
			lum2 := 0.299*float64(r2>>8) + 0.587*float64(g2>>8) + 0.114*float64(b2>>8)
			lum3 := 0.299*float64(r3>>8) + 0.587*float64(g3>>8) + 0.114*float64(b3>>8)
			sum += math.Abs(lum2-lum1) + math.Abs(lum3-lum1)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
