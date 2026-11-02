package facedetect

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"
)

func TestHasFaceRealImage(t *testing.T) {
	f, err := os.Open("testdata/sample.jpg")
	if err != nil {
		t.Fatalf("failed to open sample image: %v", err)
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatalf("failed to decode sample image: %v", err)
	}
	if !HasFace(img) {
		t.Fatalf("expected HasFace to return true on real face image")
	}
}

func TestHasFaceBlankImage(t *testing.T) {
	blank := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			blank.Set(x, y, color.RGBA{R: 120, G: 130, B: 140, A: 255})
		}
	}
	if HasFace(blank) {
		t.Fatalf("expected HasFace to return false on blank image")
	}
}

func TestBlurImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			if x < 20 {
				src.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			} else {
				src.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
			}
		}
	}
	blurred := BlurImage(src, 4)
	r, _, b, _ := blurred.At(20, 20).RGBA()
	if r == 0 || b == 0 {
		t.Fatalf("expected blurred boundary pixel to mix red and blue, got r=%d b=%d", r, b)
	}
}
