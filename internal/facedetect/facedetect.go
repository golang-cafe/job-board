// Package facedetect provides pure Go face detection and image blurring
// used by the developer profile image flows.
//
// Face detection is provided by the Pigo library (github.com/esimov/pigo),
// a pure Go implementation of the Viola-Jones object detection framework.
// It needs no C bindings and no external binaries, as requested by the
// maintainer. The cascade data file in data/facefinder is shipped by the
// Pigo project (MIT licensed, Copyright (c) 2018 Endre Simo) and is
// embedded into the binary at build time.
//
// Detection is a spam/abuse filter only: it checks that an uploaded
// profile picture contains something that looks like a human face. It is
// not identity, liveness or biometric verification.
package facedetect

import (
	_ "embed"
	"image"
	"sync"

	pigo "github.com/esimov/pigo/core"
)

//go:embed data/facefinder
var cascadeFile []byte

// minDetectionQuality is the minimum cascade score (Q) for a detection to
// be treated as a face. The Pigo examples use 5.0 as the threshold for a
// reliable detection.
const minDetectionQuality = 5.0

var (
	classifierOnce sync.Once
	classifier     *pigo.Pigo
	classifierErr  error
)

func unpackClassifier() (*pigo.Pigo, error) {
	classifierOnce.Do(func() {
		classifier, classifierErr = pigo.NewPigo().Unpack(cascadeFile)
	})
	return classifier, classifierErr
}

// ContainsFace reports whether at least one face is detected in src.
func ContainsFace(src image.Image) (bool, error) {
	classifier, err := unpackClassifier()
	if err != nil {
		return false, err
	}
	bounds := src.Bounds()
	cols, rows := bounds.Dx(), bounds.Dy()
	if cols == 0 || rows == 0 {
		return false, nil
	}
	maxFaceSize := cols
	if rows < maxFaceSize {
		maxFaceSize = rows
	}
	params := pigo.CascadeParams{
		MinSize:     20,
		MaxSize:     maxFaceSize,
		ShiftFactor: 0.1,
		ScaleFactor: 1.1,
		ImageParams: pigo.ImageParams{
			Pixels: pigo.RgbToGrayscale(src),
			Rows:   rows,
			Cols:   cols,
			Dim:    cols,
		},
	}
	detections := classifier.RunCascade(params, 0)
	detections = classifier.ClusterDetections(detections, 0.2)
	for _, detection := range detections {
		if detection.Q > minDetectionQuality {
			return true, nil
		}
	}
	return false, nil
}
