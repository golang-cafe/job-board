package imagemeta

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"path/filepath"

	"github.com/golang-cafe/job-board/internal/job"
	"github.com/fogleman/gg"
	"github.com/pkg/errors"
)

const (
	backgroundImageFilename = "static/assets/img/meta-bg.jpg"
	outputFilename          = "output.jpg"
)

func GenerateImageForJob(jobPost job.JobPost) (io.ReadWriter, error) {
	dc := gg.NewContext(1200, 628)
	backgroundImage, err := gg.LoadImage(backgroundImageFilename)
	w := bytes.NewBuffer([]byte{})
	if err != nil {
		return w, errors.Wrap(err, "load background image")
	}
	// draw background image
	dc.DrawImage(backgroundImage, 0, 0)

	// draw job post title and description
	title := fmt.Sprintf("%s with %s\n\n %s\n\n %s", jobPost.JobTitle, jobPost.Company, jobPost.Location, jobPost.SalaryRange)
	mainTextColor := color.RGBA{
		R: uint8(0),
		G: uint8(0),
		B: uint8(144),
		A: uint8(255),
	}
	fontPath := filepath.Join("static", "assets", "fonts", "verdana", "verdana.ttf")
	if err := dc.LoadFontFace(fontPath, 60); err != nil {
		return w, errors.Wrap(err, "load Courier_Prime for job link")
	}
	textRightMargin := 80.0
	textTopMargin := 90.0
	x := textRightMargin
	y := textTopMargin
	maxWidth := float64(dc.Width()) - textRightMargin - textRightMargin
	dc.SetColor(mainTextColor)
	dc.DrawStringWrapped(title, x, y, 0, 0, maxWidth, 1.5, gg.AlignLeft)

	if err := png.Encode(w, dc.Image()); err != nil {
		return w, err
	}

	return w, nil
}

// ValidateDeveloperFaceImage performs a pure-Go human portrait/skin-tone & contrast check
// without requiring C bindings or OpenCV, rejecting solid-color or spam banner images.
func ValidateDeveloperFaceImage(img image.Image) error {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	if w < 64 || h < 64 {
		return errors.New("avatar image is too small (minimum 64x64)")
	}
	x0 := bounds.Min.X + w/5
	x1 := bounds.Max.X - w/5
	y0 := bounds.Min.Y + h/5
	y1 := bounds.Max.Y - h/5

	totalSamples := 0
	skinToneCount := 0
	var lumSum, lumSqSum float64

	for y := y0; y < y1; y += 2 {
		for x := x0; x < x1; x += 2 {
			r16, g16, b16, _ := img.At(x, y).RGBA()
			r := float64(r16 >> 8)
			g := float64(g16 >> 8)
			b := float64(b16 >> 8)
			totalSamples++

			lum := 0.299*r + 0.587*g + 0.114*b
			lumSum += lum
			lumSqSum += lum * lum

			cb := 128.0 - 0.168736*r - 0.331264*g + 0.5*b
			cr := 128.0 + 0.5*r - 0.418688*g - 0.081312*b
			if cb >= 75 && cb <= 135 && cr >= 130 && cr <= 180 {
				skinToneCount++
			}
		}
	}

	if totalSamples == 0 {
		return errors.New("invalid image dimensions")
	}
	meanLum := lumSum / float64(totalSamples)
	variance := (lumSqSum / float64(totalSamples)) - (meanLum * meanLum)
	if variance < 120.0 {
		return errors.New("image lacks facial detail or is a flat/solid graphic")
	}
	skinRatio := float64(skinToneCount) / float64(totalSamples)
	if skinRatio < 0.08 {
		return errors.New("no human face detected in avatar image")
	}
	return nil
}

// BlurImageForAnonymous applies a fast separable box blur in pure Go for unauthenticated visitors.
func BlurImageForAnonymous(src image.Image, radius int) image.Image {
	if radius <= 0 {
		radius = 12
	}
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	tmp := image.NewRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			var rSum, gSum, bSum, aSum, count uint32
			for dx := -radius; dx <= radius; dx++ {
				nx := x + dx
				if nx < bounds.Min.X {
					nx = bounds.Min.X
				} else if nx >= bounds.Max.X {
					nx = bounds.Max.X - 1
				}
				r, g, b, a := src.At(nx, y).RGBA()
				rSum += r >> 8
				gSum += g >> 8
				bSum += b >> 8
				aSum += a >> 8
				count++
			}
			tmp.SetRGBA(x, y, color.RGBA{
				R: uint8(rSum / count),
				G: uint8(gSum / count),
				B: uint8(bSum / count),
				A: uint8(aSum / count),
			})
		}
	}

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			var rSum, gSum, bSum, aSum, count uint32
			for dy := -radius; dy <= radius; dy++ {
				ny := y + dy
				if ny < bounds.Min.Y {
					ny = bounds.Min.Y
				} else if ny >= bounds.Max.Y {
					ny = bounds.Max.Y - 1
				}
				c := tmp.RGBAAt(x, ny)
				rSum += uint32(c.R)
				gSum += uint32(c.G)
				bSum += uint32(c.B)
				aSum += uint32(c.A)
				count++
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(rSum / count),
				G: uint8(gSum / count),
				B: uint8(bSum / count),
				A: uint8(aSum / count),
			})
		}
	}
	return dst
}
