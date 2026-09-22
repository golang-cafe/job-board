package handler

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"

	pigo "github.com/esimov/pigo/core"
	"github.com/nfnt/resize"
)

//go:embed facefinder
var faceCascade []byte

func containsExactlyOneFaceBytes(data []byte) (bool, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("decode profile image: %w", err)
	}
	return containsExactlyOneFace(img)
}

func containsExactlyOneFace(img image.Image) (bool, error) {
	src := pigo.ImgToNRGBA(img)
	classifier, err := pigo.NewPigo().Unpack(faceCascade)
	if err != nil {
		return false, fmt.Errorf("unpack face cascade: %w", err)
	}

	params := pigo.CascadeParams{
		MinSize:     20,
		MaxSize:     1000,
		ShiftFactor: 0.1,
		ScaleFactor: 1.1,
		ImageParams: pigo.ImageParams{
			Pixels: pigo.RgbToGrayscale(src),
			Rows:   src.Bounds().Dy(),
			Cols:   src.Bounds().Dx(),
			Dim:    src.Bounds().Dx(),
		},
	}

	detections := classifier.ClusterDetections(classifier.RunCascade(params, 0), 0.2)
	count := 0
	for _, d := range detections {
		if d.Q >= 5.0 {
			count++
		}
	}
	return count == 1, nil
}

func blurProfileImage(img image.Image) image.Image {
	b := img.Bounds()
	if b.Dx() < 2 || b.Dy() < 2 {
		return img
	}

	w, h := uint(b.Dx()/14), uint(b.Dy()/14)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	small := resize.Resize(w, h, img, resize.Bilinear)
	return resize.Resize(uint(b.Dx()), uint(b.Dy()), small, resize.NearestNeighbor)
}
