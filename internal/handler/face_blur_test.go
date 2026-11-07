package handler

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

type drawRegion struct {
	rect  image.Rectangle
	shade color.NRGBA
}

func fillImageRegion(img *image.NRGBA, reg drawRegion) {
	for y := reg.rect.Min.Y; y < reg.rect.Max.Y; y++ {
		for x := reg.rect.Min.X; x < reg.rect.Max.X; x++ {
			img.SetNRGBA(x, y, reg.shade)
		}
	}
}

func buildSyntheticFaceImage() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 80, 80))
	fillImageRegion(img, drawRegion{rect: image.Rect(0, 0, 80, 80), shade: color.NRGBA{R: 235, G: 240, B: 245, A: 255}})
	fillImageRegion(img, drawRegion{rect: image.Rect(18, 14, 63, 67), shade: color.NRGBA{R: 215, G: 162, B: 132, A: 255}})
	fillImageRegion(img, drawRegion{rect: image.Rect(26, 28, 34, 35), shade: color.NRGBA{R: 35, G: 30, B: 28, A: 255}})
	fillImageRegion(img, drawRegion{rect: image.Rect(47, 28, 55, 35), shade: color.NRGBA{R: 35, G: 30, B: 28, A: 255}})
	fillImageRegion(img, drawRegion{rect: image.Rect(32, 50, 49, 55), shade: color.NRGBA{R: 35, G: 30, B: 28, A: 255}})
	return img
}

func TestDetectFaceInImageBytes(t *testing.T) {
	faceImg := buildSyntheticFaceImage()
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, faceImg); err != nil {
		t.Fatalf("failed to encode synthetic face image: %v", err)
	}
	if !DetectFaceInImageBytes(buf.Bytes()) {
		t.Fatalf("expected synthetic portrait with facial features and skin tone to be detected")
	}

	blankImg := image.NewNRGBA(image.Rect(0, 0, 80, 80))
	fillImageRegion(blankImg, drawRegion{rect: image.Rect(0, 0, 80, 80), shade: color.NRGBA{R: 10, G: 180, B: 220, A: 255}})
	blankBuf := new(bytes.Buffer)
	_ = png.Encode(blankBuf, blankImg)
	if DetectFaceInImageBytes(blankBuf.Bytes()) {
		t.Fatalf("expected uniform non-skin spam image to be rejected")
	}
}

func TestBlurImageReducesSharpEdgeContrast(t *testing.T) {
	faceImg := buildSyntheticFaceImage()
	origEye := faceImg.NRGBAAt(28, 30)
	blurred := BlurImage(faceImg, 8)
	r, _, _, _ := blurred.At(28, 30).RGBA()
	blurredR := uint8(r >> 8)
	if blurredR <= origEye.R {
		t.Fatalf("expected blur filter to blend dark eye pixel with surrounding skin, got %d vs %d", blurredR, origEye.R)
	}
}
