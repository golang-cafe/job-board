package handler

import (
	"image"
	"image/color"
	"testing"
)

func TestHasSingleFaceRejectsBlankImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 80, 80))

	ok, err := hasSingleFace(img)
	if err != nil {
		t.Fatalf("hasSingleFace returned error for blank image: %v", err)
	}
	if ok {
		t.Fatal("hasSingleFace accepted a blank image")
	}
}

func TestObscureImageKeepsBoundsAndChangesPixels(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 10), G: uint8(y * 10), B: uint8((x + y) * 5), A: 255})
		}
	}

	obscured := obscureImage(img)
	if obscured.Bounds() != img.Bounds() {
		t.Fatalf("obscured image bounds = %v, want %v", obscured.Bounds(), img.Bounds())
	}
	if obscured.At(5, 5) == img.At(5, 5) && obscured.At(12, 12) == img.At(12, 12) {
		t.Fatal("obscureImage did not change sampled pixels")
	}
}
