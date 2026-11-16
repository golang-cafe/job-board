package facedetect

import (
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"os"
	"testing"
)

// The positive fixture in testdata/face.jpg is the sample image shipped
// with the Pigo project (MIT licensed, Copyright (c) 2018 Endre Simo),
// used here to verify the detector end to end.
func TestContainsFaceDetectsFace(t *testing.T) {
	f, err := os.Open("testdata/face.jpg")
	if err != nil {
		t.Fatalf("unable to open test fixture: %v", err)
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatalf("unable to decode test fixture: %v", err)
	}
	found, err := ContainsFace(img)
	if err != nil {
		t.Fatalf("ContainsFace returned an error: %v", err)
	}
	if !found {
		t.Error("expected a face to be detected in the sample portrait image")
	}
}

func TestContainsFaceRejectsNonFaceImages(t *testing.T) {
	// a plain solid colour image contains no face
	solid := image.NewNRGBA(image.Rect(0, 0, 120, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 120; x++ {
			solid.Set(x, y, color.NRGBA{R: 120, G: 120, B: 120, A: 255})
		}
	}
	found, err := ContainsFace(solid)
	if err != nil {
		t.Fatalf("ContainsFace returned an error: %v", err)
	}
	if found {
		t.Error("expected no face to be detected in a solid colour image")
	}

	// random noise contains no face either
	noise := image.NewNRGBA(image.Rect(0, 0, 120, 120))
	rng := rand.New(rand.NewSource(42))
	for y := 0; y < 120; y++ {
		for x := 0; x < 120; x++ {
			v := uint8(rng.Intn(256))
			noise.Set(x, y, color.NRGBA{R: v, G: v, B: v, A: 255})
		}
	}
	found, err = ContainsFace(noise)
	if err != nil {
		t.Fatalf("ContainsFace returned an error: %v", err)
	}
	if found {
		t.Error("expected no face to be detected in a random noise image")
	}
}

func TestContainsFaceEmptyImage(t *testing.T) {
	found, err := ContainsFace(image.NewNRGBA(image.Rectangle{}))
	if err != nil {
		t.Fatalf("ContainsFace returned an error: %v", err)
	}
	if found {
		t.Error("expected no face to be detected in an empty image")
	}
}
