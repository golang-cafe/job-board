package profilephoto

import (
	"image"
	_ "image/png"
	"os"
	"testing"
)

func TestRejectBlankAndTinyImages(t *testing.T) {
	for _, size := range []int{1, 39, 128, 640} {
		ok, err := HasFace(image.NewRGBA(image.Rect(0, 0, size, size)))
		if err != nil || ok {
			t.Fatalf("blank %d: %v %v", size, ok, err)
		}
	}
}

func TestPublicFaceFixture(t *testing.T) {
	path := os.Getenv("PHOTO_TEST_FACE")
	if path == "" {
		t.Skip("set PHOTO_TEST_FACE to a licensed public face fixture")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	im, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if err = Validate(im); err != nil {
		t.Fatal(err)
	}
}
