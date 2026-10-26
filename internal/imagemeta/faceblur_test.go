package imagemeta

import (
	"image"
	"image/color"
	"testing"
)

func TestHasHumanFaceRejectsSolidSpamImage(t *testing.T) {
	spam := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			spam.SetNRGBA(x, y, color.NRGBA{R: 30, G: 30, B: 200, A: 255})
		}
	}
	if HasHumanFace(spam) {
		t.Fatalf("expected solid spam image to be rejected")
	}
}

func TestHasHumanFaceAcceptsSyntheticPortrait(t *testing.T) {
	portrait := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			c := color.NRGBA{R: 40, G: 45, B: 55, A: 255}
			if x >= 12 && x < 52 && y >= 8 && y < 56 {
				c = color.NRGBA{R: 215, G: 170, B: 140, A: 255}
			}
			if y >= 20 && y < 29 && ((x >= 18 && x < 28) || (x >= 36 && x < 46)) {
				c = color.NRGBA{R: 45, G: 35, B: 30, A: 255}
			}
			if y >= 42 && y < 46 && x >= 24 && x < 40 {
				c = color.NRGBA{R: 150, G: 75, B: 75, A: 255}
			}
			portrait.SetNRGBA(x, y, c)
		}
	}
	if !HasHumanFace(portrait) {
		t.Fatalf("expected synthetic portrait face to be accepted")
	}
}

func TestBlurImageBlursSharpEdge(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			if x < 20 {
				src.SetNRGBA(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: 255})
			} else {
				src.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	blurred := BlurImage(src, 6)
	r, _, _, _ := blurred.At(19, 20).RGBA()
	val := r >> 8
	if val == 0 || val == 255 {
		t.Fatalf("expected blurred boundary pixel to be intermediate, got %d", val)
	}
}
