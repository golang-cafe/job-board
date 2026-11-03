// Package profilephoto performs local face detection, not identity verification.
package profilephoto

import (
	_ "embed"
	"errors"
	"image"
	"sync"

	"github.com/disintegration/imaging"
	pigo "github.com/esimov/pigo/core"
)

//go:embed facefinder
var cascade []byte

var detector struct {
	sync.Mutex
	once       sync.Once
	classifier *pigo.Pigo
	err        error
}

// HasFace bounds detector work and serializes the shared classifier.
func HasFace(src image.Image) (bool, error) {
	detector.once.Do(func() { detector.classifier, detector.err = pigo.NewPigo().Unpack(cascade) })
	if detector.err != nil {
		return false, detector.err
	}
	if src.Bounds().Dx() < 40 || src.Bounds().Dy() < 40 {
		return false, nil
	}
	small := imaging.Fit(src, 640, 640, imaging.Linear)
	detector.Lock()
	defer detector.Unlock()
	detections := detector.classifier.RunCascade(pigo.CascadeParams{
		MinSize: 40, MaxSize: 640, ShiftFactor: 0.1, ScaleFactor: 1.1,
		ImageParams: pigo.ImageParams{Pixels: pigo.RgbToGrayscale(small), Rows: small.Bounds().Dy(), Cols: small.Bounds().Dx(), Dim: small.Bounds().Dx()},
	}, 0)
	for _, d := range detector.classifier.ClusterDetections(detections, 0.2) {
		if d.Q >= 5 {
			return true, nil
		}
	}
	return false, nil
}

func Validate(src image.Image) error {
	ok, err := HasFace(src)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("please upload a clear photo with a visible face")
	}
	return nil
}

// Blur strips original encoded bytes and applies a server-side Gaussian blur.
func Blur(src image.Image) image.Image {
	return imaging.Blur(imaging.Fit(src, 512, 512, imaging.Linear), 20)
}
