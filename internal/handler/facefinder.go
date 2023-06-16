package handler

import (
	"embed"
	"fmt"
	"image"

	pigo "github.com/esimov/pigo/core"
)

//go:embed facefinder
var fs embed.FS

func hasSingleFace(img image.Image) (bool, error) {
	cascadeFile, err := fs.ReadFile("facefinder")
	if err != nil {
		return false, fmt.Errorf("error reading the cascade file: %w", err)
	}

	src := pigo.ImgToNRGBA(img)
	pixels := pigo.RgbToGrayscale(src)
	cols, rows := src.Bounds().Max.X, src.Bounds().Max.Y

	cParams := pigo.CascadeParams{
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

	pigo := pigo.NewPigo()
	// Unpack the binary file. This will return the number of cascade trees,
	// the tree depth, the threshold and the prediction from tree's leaf nodes.
	classifier, err := pigo.Unpack(cascadeFile)
	if err != nil {
		return false, fmt.Errorf("error reading the cascade file: %w", err)
	}

	angle := 0.0 // cascade rotation angle. 0.0 is 0 radians and 1.0 is 2*pi radians

	// Run the classifier over the obtained leaf nodes and return the detection results.
	// The result contains quadruplets representing the row, column, scale and detection score.
	dets := classifier.RunCascade(cParams, angle)

	// Calculate the intersection over union (IoU) of two clusters.
	dets = classifier.ClusterDetections(dets, 0.2)

	var foundFaces int
	for _, det := range dets {
		if det.Q > 90 {
			foundFaces++
		}
	}

	if foundFaces == 1 {
		return true, nil
	} else if foundFaces > 1 {
		return false, fmt.Errorf("found %d faces", foundFaces)
	} else {
		return false, nil
	}
}
