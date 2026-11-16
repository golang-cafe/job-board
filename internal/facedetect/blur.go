package facedetect

import (
	"image"
)

// Blur returns a blurred copy of src using a two-pass (horizontal then
// vertical) box blur with the given radius in pixels. A radius of 0 or
// less returns an unmodified copy of src. It is used to serve
// privacy-blurred variants of developer profile pictures to visitors who
// are not logged in, replacing the previous CSS-only blur which left the
// original image publicly downloadable.
func Blur(src image.Image, radius int) image.Image {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			dst.Set(x, y, src.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	if radius <= 0 || width == 0 || height == 0 {
		return dst
	}
	return boxBlurVertical(boxBlurHorizontal(dst, radius), radius)
}

// boxBlurHorizontal returns a copy of src blurred along the x axis only.
// The sliding window reads exclusively from src, so writes to the output
// image cannot corrupt neighbouring averages.
func boxBlurHorizontal(src *image.NRGBA, radius int) *image.NRGBA {
	width := src.Bounds().Dx()
	height := src.Bounds().Dy()
	dst := image.NewNRGBA(src.Bounds())
	count := uint32(2*radius + 1)
	for y := 0; y < height; y++ {
		var sumR, sumG, sumB, sumA uint32
		for i := -radius; i <= radius; i++ {
			c := src.NRGBAAt(clampInt(i, 0, width-1), y)
			sumR += uint32(c.R)
			sumG += uint32(c.G)
			sumB += uint32(c.B)
			sumA += uint32(c.A)
		}
		for x := 0; x < width; x++ {
			dst.Pix[dst.PixOffset(x, y)+0] = uint8(sumR / count)
			dst.Pix[dst.PixOffset(x, y)+1] = uint8(sumG / count)
			dst.Pix[dst.PixOffset(x, y)+2] = uint8(sumB / count)
			dst.Pix[dst.PixOffset(x, y)+3] = uint8(sumA / count)
			outC := src.NRGBAAt(clampInt(x-radius, 0, width-1), y)
			inC := src.NRGBAAt(clampInt(x+radius+1, 0, width-1), y)
			sumR += uint32(inC.R) - uint32(outC.R)
			sumG += uint32(inC.G) - uint32(outC.G)
			sumB += uint32(inC.B) - uint32(outC.B)
			sumA += uint32(inC.A) - uint32(outC.A)
		}
	}
	return dst
}

// boxBlurVertical returns a copy of src blurred along the y axis only.
func boxBlurVertical(src *image.NRGBA, radius int) *image.NRGBA {
	width := src.Bounds().Dx()
	height := src.Bounds().Dy()
	dst := image.NewNRGBA(src.Bounds())
	count := uint32(2*radius + 1)
	for x := 0; x < width; x++ {
		var sumR, sumG, sumB, sumA uint32
		for i := -radius; i <= radius; i++ {
			c := src.NRGBAAt(x, clampInt(i, 0, height-1))
			sumR += uint32(c.R)
			sumG += uint32(c.G)
			sumB += uint32(c.B)
			sumA += uint32(c.A)
		}
		for y := 0; y < height; y++ {
			dst.Pix[dst.PixOffset(x, y)+0] = uint8(sumR / count)
			dst.Pix[dst.PixOffset(x, y)+1] = uint8(sumG / count)
			dst.Pix[dst.PixOffset(x, y)+2] = uint8(sumB / count)
			dst.Pix[dst.PixOffset(x, y)+3] = uint8(sumA / count)
			outC := src.NRGBAAt(x, clampInt(y-radius, 0, height-1))
			inC := src.NRGBAAt(x, clampInt(y+radius+1, 0, height-1))
			sumR += uint32(inC.R) - uint32(outC.R)
			sumG += uint32(inC.G) - uint32(outC.G)
			sumB += uint32(inC.B) - uint32(outC.B)
			sumA += uint32(inC.A) - uint32(outC.A)
		}
	}
	return dst
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
