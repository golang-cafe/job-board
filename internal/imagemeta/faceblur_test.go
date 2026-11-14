package imagemeta

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func createSyntheticFaceImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	// Background
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 40, G: 50, B: 70, A: 255})
		}
	}
	// Central oval face with natural skin tone and darker eye sockets
	cx, cy := float64(w)/2.0, float64(h)/2.0
	rx, ry := float64(w)*0.28, float64(h)*0.34
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx := (float64(x) - cx) / rx
			dy := (float64(y) - cy) / ry
			if dx*dx+dy*dy <= 1.0 {
				// Skin tone
				c := color.NRGBA{R: 215, G: 160, B: 130, A: 255}
				// Eyes in upper-middle band
				if math.Abs(float64(y)-(cy-ry*0.2)) < ry*0.12 && (math.Abs(float64(x)-(cx-rx*0.35)) < rx*0.18 || math.Abs(float64(x)-(cx+rx*0.35)) < rx*0.18) {
					c = color.NRGBA{R: 45, G: 35, B: 30, A: 255}
				}
				img.SetNRGBA(x, y, c)
			}
		}
	}
	return img
}

func TestDetectFace(t *testing.T) {
	faceImg := createSyntheticFaceImage(120, 120)
	if !DetectFace(faceImg) {
		t.Fatalf("expected DetectFace to return true for synthetic portrait with face")
	}

	// Solid blank image should fail face detection
	blankImg := image.NewNRGBA(image.Rect(0, 0, 120, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 120; x++ {
			blankImg.SetNRGBA(x, y, color.NRGBA{R: 20, G: 100, B: 200, A: 255})
		}
	}
	if DetectFace(blankImg) {
		t.Fatalf("expected DetectFace to return false for uniform non-face image")
	}

	// Blue/green geometric pattern without skin tone or facial contrast should fail
	geoImg := image.NewNRGBA(image.Rect(0, 0, 120, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 120; x++ {
			if (x+y)%2 == 0 {
				geoImg.SetNRGBA(x, y, color.NRGBA{R: 10, G: 180, B: 220, A: 255})
			} else {
				geoImg.SetNRGBA(x, y, color.NRGBA{R: 20, G: 210, B: 240, A: 255})
			}
		}
	}
	if DetectFace(geoImg) {
		t.Fatalf("expected DetectFace to return false for non-face geometric pattern")
	}
}

func TestBlurImage(t *testing.T) {
	faceImg := createSyntheticFaceImage(100, 100)
	blurred := BlurImage(faceImg, 8)
	if blurred == nil {
		t.Fatalf("expected non-nil blurred image")
	}
	if blurred.Bounds().Dx() != 100 || blurred.Bounds().Dy() != 100 {
		t.Fatalf("expected 100x100 bounds, got %v", blurred.Bounds())
	}

	// Verify sharp eye-boundary contrast is smoothed out by blurring
	eyeX, eyeY := int(50-100*0.28*0.35), int(50-100*0.34*0.2)
	origR, _, _, _ := faceImg.At(eyeX, eyeY).RGBA()
	blurR, _, _, _ := blurred.At(eyeX, eyeY).RGBA()
	if blurR <= origR {
		t.Fatalf("expected dark eye pixel to be blended with surrounding brighter skin after blur (origR=%d, blurR=%d)", origR>>8, blurR>>8)
	}
}
