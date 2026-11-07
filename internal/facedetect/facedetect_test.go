package facedetect

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"testing"
)

// Helper to create a solid color image
func createSolidImage(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
	return img
}

// Helper to create a synthetic human face portrait
func createSyntheticFace(w, h int, skin color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Light blue background
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{180, 200, 220, 255}}, image.Point{}, draw.Src)

	// Draw head / face oval in center
	centerX, centerY := w/2, h/2
	radiusX, radiusY := w/3, h/3
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx := float64(x - centerX)
			dy := float64(y - centerY)
			if (dx*dx)/(float64(radiusX*radiusX))+(dy*dy)/(float64(radiusY*radiusY)) <= 1.0 {
				img.SetRGBA(x, y, skin)
			}
		}
	}

	// Forehead highlight (slightly brighter)
	for y := centerY - radiusY/2; y < centerY - radiusY/5; y++ {
		for x := centerX - radiusX/2; x < centerX + radiusX/2; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(min(255, int(skin.R)+15)),
				G: uint8(min(255, int(skin.G)+15)),
				B: uint8(min(255, int(skin.B)+15)),
				A: 255,
			})
		}
	}

	// Eyes (darker contrast)
	eyeY := centerY - radiusY/10
	eyeLeftX := centerX - radiusX/2
	eyeRightX := centerX + radiusX/2
	for dy := -3; dy <= 3; dy++ {
		for dx := -4; dx <= 4; dx++ {
			img.SetRGBA(eyeLeftX+dx, eyeY+dy, color.RGBA{40, 30, 25, 255})
			img.SetRGBA(eyeRightX+dx, eyeY+dy, color.RGBA{40, 30, 25, 255})
		}
	}

	// Mouth
	mouthY := centerY + radiusY/2
	for dx := -radiusX / 3; dx <= radiusX / 3; dx++ {
		img.SetRGBA(centerX+dx, mouthY, color.RGBA{160, 50, 50, 255})
	}

	return img
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestContainsHumanFace_RejectsNonFaces(t *testing.T) {
	tests := []struct {
		name string
		img  image.Image
	}{
		{"nil image", nil},
		{"too small", createSolidImage(16, 16, color.White)},
		{"solid black", createSolidImage(100, 100, color.Black)},
		{"solid white", createSolidImage(100, 100, color.White)},
		{"solid blue", createSolidImage(100, 100, color.RGBA{0, 0, 255, 255})},
		{"solid skin color (no features/variance)", createSolidImage(100, 100, color.RGBA{220, 175, 140, 255})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if ContainsHumanFace(tt.img) {
				t.Errorf("ContainsHumanFace() = true, expected false for %s", tt.name)
			}
		})
	}
}

func TestContainsHumanFace_AcceptsSyntheticPortraits(t *testing.T) {
	// Test fair, olive, and darker skin tones
	skinTones := []struct {
		name string
		skin color.RGBA
	}{
		{"Fair Skin", color.RGBA{235, 195, 165, 255}},
		{"Medium / Olive Skin", color.RGBA{210, 165, 125, 255}},
		{"Darker Brown Skin", color.RGBA{145, 95, 60, 255}},
	}

	for _, st := range skinTones {
		t.Run(st.name, func(t *testing.T) {
			portrait := createSyntheticFace(120, 120, st.skin)
			if !ContainsHumanFace(portrait) {
				t.Errorf("ContainsHumanFace() = false, expected true for %s portrait", st.name)
			}
		})
	}
}

func TestContainsHumanFaceFromBytes(t *testing.T) {
	// Empty bytes
	_, err := ContainsHumanFaceFromBytes(nil)
	if err == nil {
		t.Errorf("expected error for empty bytes, got nil")
	}

	// Valid JPEG face
	portrait := createSyntheticFace(100, 100, color.RGBA{225, 180, 145, 255})
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, portrait, nil); err != nil {
		t.Fatalf("failed to encode jpeg: %v", err)
	}

	hasFace, err := ContainsHumanFaceFromBytes(buf.Bytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasFace {
		t.Errorf("expected true for encoded jpeg portrait, got false")
	}
}

func TestBlurImage(t *testing.T) {
	src := createSyntheticFace(80, 80, color.RGBA{210, 160, 120, 255})
	blurred := BlurImage(src, 8)
	if blurred == nil {
		t.Fatalf("BlurImage returned nil")
	}
	if blurred.Bounds() != src.Bounds() {
		t.Errorf("expected bounds %v, got %v", src.Bounds(), blurred.Bounds())
	}
}

func TestBlurImageBytes(t *testing.T) {
	src := createSyntheticFace(80, 80, color.RGBA{210, 160, 120, 255})

	// Test PNG encoding
	pngBuf := new(bytes.Buffer)
	if err := png.Encode(pngBuf, src); err != nil {
		t.Fatalf("png encode error: %v", err)
	}

	blurredPNG, err := BlurImageBytes(pngBuf.Bytes(), "image/png", 10)
	if err != nil {
		t.Fatalf("BlurImageBytes(png) error: %v", err)
	}
	if len(blurredPNG) == 0 {
		t.Errorf("blurred png bytes are empty")
	}

	// Test JPEG encoding
	jpgBuf := new(bytes.Buffer)
	if err := jpeg.Encode(jpgBuf, src, nil); err != nil {
		t.Fatalf("jpeg encode error: %v", err)
	}

	blurredJPG, err := BlurImageBytes(jpgBuf.Bytes(), "image/jpeg", 10)
	if err != nil {
		t.Fatalf("BlurImageBytes(jpeg) error: %v", err)
	}
	if len(blurredJPG) == 0 {
		t.Errorf("blurred jpeg bytes are empty")
	}
}
