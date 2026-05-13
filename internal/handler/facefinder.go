package handler

import (
	"embed"
	"fmt"
	"image"

	pigo "github.com/esimov/pigo/core"
	"github.com/nfnt/resize"
)

//go:embed facefinder
var facefinderFS embed.FS

func hasSingleFace(img image.Image) (bool, error) {
	cascadeFile, err := facefinderFS.ReadFile("facefinder")
	if err != nil {
		return false, fmt.Errorf("read face cascade: %w", err)
	}

	src := pigo.ImgToNRGBA(img)
	pixels := pigo.RgbToGrayscale(src)
	cols, rows := src.Bounds().Dx(), src.Bounds().Dy()

	params := pigo.CascadeParams{
		MinSize:     20,
		MaxSize:     1000,
		ShiftFactor: 0.1,
		ScaleFactor: 1.1,
		ImageParams: pigo.ImageParams{
			Pixels: pixels,
			Rows:   rows,
			Cols:   cols,
			Dim:    cols,
		},
	}

	classifier, err := pigo.NewPigo().Unpack(cascadeFile)
	if err != nil {
		return false, fmt.Errorf("unpack face cascade: %w", err)
	}

	detections := classifier.RunCascade(params, 0)
	detections = classifier.ClusterDetections(detections, 0.2)

	faces := 0
	for _, detection := range detections {
		if detection.Q > 90 {
			faces++
		}
	}
	if faces == 1 {
		return true, nil
	}
	if faces > 1 {
		return false, fmt.Errorf("found %d faces", faces)
	}
	return false, nil
}

func obscureImage(img image.Image) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width < 2 || height < 2 {
		return img
	}

	smallWidth := uint(width / 12)
	if smallWidth < 1 {
		smallWidth = 1
	}
	smallHeight := uint(height / 12)
	if smallHeight < 1 {
		smallHeight = 1
	}

	downscaled := resize.Resize(smallWidth, smallHeight, img, resize.Bilinear)
	return resize.Resize(uint(width), uint(height), downscaled, resize.NearestNeighbor)
}
