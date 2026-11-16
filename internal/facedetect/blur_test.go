package facedetect

import (
	"image"
	"image/color"
	"testing"
)

// checkerboard builds a high contrast image whose pixels alternate
// between black and white; a working blur must visibly soften it.
func checkerboard(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if (x+y)%2 == 0 {
				img.Set(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			} else {
				img.Set(x, y, color.NRGBA{A: 255})
			}
		}
	}
	return img
}

func TestBlurPreservesBounds(t *testing.T) {
	src := checkerboard(40)
	out := Blur(src, 5)
	if out.Bounds() != src.Bounds() {
		t.Errorf("expected bounds %v, got %v", src.Bounds(), out.Bounds())
	}
}

func TestBlurSoftensImage(t *testing.T) {
	src := checkerboard(40)
	out := Blur(src, 5)
	// in the centre of the checkerboard the blurred pixels should be a
	// mid grey rather than pure black or white
	r, g, b, _ := out.At(20, 20).RGBA()
	if r != g || g != b {
		t.Errorf("expected grey pixel after blurring a grey checkerboard, got r=%d g=%d b=%d", r, g, b)
	}
	grey := r >> 8
	if grey < 100 || grey > 155 {
		t.Errorf("expected centre pixel to be mid grey (100-155), got %d", grey)
	}
}

func TestBlurZeroRadiusReturnsCopy(t *testing.T) {
	src := checkerboard(10)
	out := Blur(src, 0)
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if out.At(x, y) != src.At(x, y) {
				t.Fatalf("expected pixel (%d,%d) to be unchanged with radius 0", x, y)
			}
		}
	}
}

func TestBlurDoesNotModifySource(t *testing.T) {
	src := checkerboard(20)
	before := src.NRGBAAt(10, 10)
	Blur(src, 4)
	if after := src.NRGBAAt(10, 10); after != before {
		t.Errorf("expected source image to be unmodified, pixel changed from %v to %v", before, after)
	}
}
