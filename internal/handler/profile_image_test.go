package handler

import (
	"image"
	"image/color"
	"testing"
)

func TestContainsExactlyOneFaceRejectsBlank(t *testing.T) {
	ok, err := containsExactlyOneFace(image.NewRGBA(image.Rect(0, 0, 80, 80)))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("blank image detected as a face")
	}
}

func TestBlurProfileImagePreservesBoundsAndChangesImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 28, 28))
	for y := 0; y < 28; y++ {
		for x := 0; x < 28; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x * 8),
				G: uint8(y * 8),
				B: uint8((x + y) * 4),
				A: 255,
			})
		}
	}

	got := blurProfileImage(img)
	if got.Bounds() != img.Bounds() {
		t.Fatalf("bounds changed: %v != %v", got.Bounds(), img.Bounds())
	}
	if got.At(7, 7) == img.At(7, 7) && got.At(18, 18) == img.At(18, 18) {
		t.Fatal("blur did not alter sampled pixels")
	}
}
