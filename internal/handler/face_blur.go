package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"strconv"

	"github.com/golang-cafe/job-board/internal/database"
	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/golang-cafe/job-board/internal/user"
	"github.com/gorilla/mux"
	"github.com/nfnt/resize"
)

const (
	defaultPrivacyBlurRadius = 12
	minFaceImageDimension    = 24
	minSkinRatioThreshold    = 0.08
	maxSkinRatioThreshold    = 0.88
	minLumaVarianceThreshold = 12.0
)

type signupImageProbe struct {
	ProfileImageID string `json:"profile_image_id"`
}

type blurPassConfig struct {
	src    *image.NRGBA
	bounds image.Rectangle
	radius int
	stepX  int
	stepY  int
}

func isWithinRange(val, low, high uint8) bool {
	return val >= low && val <= high
}

func isSkinPixelYCbCr(c color.Color) bool {
	r16, g16, b16, a16 := c.RGBA()
	if a16 < 0x4000 {
		return false
	}
	_, cb, cr := color.RGBToYCbCr(uint8(r16>>8), uint8(g16>>8), uint8(b16>>8))
	return isWithinRange(cb, 77, 127) && isWithinRange(cr, 133, 173)
}

func pixelLuminance(c color.Color) float64 {
	r16, g16, b16, _ := c.RGBA()
	r := float64(r16 >> 8)
	g := float64(g16 >> 8)
	b := float64(b16 >> 8)
	return 0.299*r + 0.587*g + 0.114*b
}

// DetectFaceInImage analyzes an image in pure Go using central YCbCr skin-region density and facial feature luminance variance.
func DetectFaceInImage(img image.Image) bool {
	if img == nil {
		return false
	}
	bounds := img.Bounds()
	if bounds.Dx() < minFaceImageDimension || bounds.Dy() < minFaceImageDimension {
		return false
	}
	skinRatio, lumaStdDev := sampleCentralFacialRegion(img, bounds)
	if skinRatio < minSkinRatioThreshold || skinRatio > maxSkinRatioThreshold {
		return false
	}
	return lumaStdDev >= minLumaVarianceThreshold
}

func sampleCentralFacialRegion(img image.Image, bounds image.Rectangle) (float64, float64) {
	xStart := bounds.Min.X + bounds.Dx()/6
	xEnd := bounds.Max.X - bounds.Dx()/6
	yStart := bounds.Min.Y + bounds.Dy()/8
	yEnd := bounds.Max.Y - bounds.Dy()/8

	var total, skinCount int
	var lumaSum, lumaSqSum float64
	for y := yStart; y < yEnd; y++ {
		for x := xStart; x < xEnd; x++ {
			c := img.At(x, y)
			if isSkinPixelYCbCr(c) {
				skinCount++
			}
			luma := pixelLuminance(c)
			lumaSum += luma
			lumaSqSum += luma * luma
			total++
		}
	}
	if total == 0 {
		return 0, 0
	}
	mean := lumaSum / float64(total)
	variance := math.Max(0, (lumaSqSum/float64(total))-(mean*mean))
	return float64(skinCount) / float64(total), math.Sqrt(variance)
}

// DetectFaceInImageBytes decodes raw PNG/JPEG image bytes and checks whether a human face region is present.
func DetectFaceInImageBytes(imgBytes []byte) bool {
	if len(imgBytes) == 0 {
		return false
	}
	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		return false
	}
	return DetectFaceInImage(img)
}

func toNRGBA(img image.Image) *image.NRGBA {
	bounds := img.Bounds()
	dst := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dst.Set(x, y, img.At(x, y))
		}
	}
	return dst
}

func clampCoord(val, minVal, maxVal int) int {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

func sampleKernelPixel(cfg blurPassConfig, pt image.Point) color.NRGBA {
	window := float64(2*cfg.radius + 1)
	var rSum, gSum, bSum, aSum float64
	for k := -cfg.radius; k <= cfg.radius; k++ {
		cx := clampCoord(pt.X+k*cfg.stepX, cfg.bounds.Min.X, cfg.bounds.Max.X-1)
		cy := clampCoord(pt.Y+k*cfg.stepY, cfg.bounds.Min.Y, cfg.bounds.Max.Y-1)
		px := cfg.src.NRGBAAt(cx, cy)
		rSum += float64(px.R)
		gSum += float64(px.G)
		bSum += float64(px.B)
		aSum += float64(px.A)
	}
	return color.NRGBA{
		R: uint8(rSum / window),
		G: uint8(gSum / window),
		B: uint8(bSum / window),
		A: uint8(aSum / window),
	}
}

func blurDirectionalPass(cfg blurPassConfig) *image.NRGBA {
	dst := image.NewNRGBA(cfg.bounds)
	for y := cfg.bounds.Min.Y; y < cfg.bounds.Max.Y; y++ {
		for x := cfg.bounds.Min.X; x < cfg.bounds.Max.X; x++ {
			dst.SetNRGBA(x, y, sampleKernelPixel(cfg, image.Pt(x, y)))
		}
	}
	return dst
}

// BlurImage applies a pure Go separable box blur filter to an image.
func BlurImage(img image.Image, radius int) image.Image {
	if radius <= 0 {
		radius = defaultPrivacyBlurRadius
	}
	bounds := img.Bounds()
	horiz := blurDirectionalPass(blurPassConfig{src: toNRGBA(img), bounds: bounds, radius: radius, stepX: 1, stepY: 0})
	return blurDirectionalPass(blurPassConfig{src: horiz, bounds: bounds, radius: radius, stepX: 0, stepY: 1})
}

// EncodeImageByMediaType encodes an image into PNG or JPEG bytes matching the given MIME type.
func EncodeImageByMediaType(img image.Image, mediaType string) ([]byte, error) {
	buf := new(bytes.Buffer)
	if mediaType == "image/png" {
		if err := png.Encode(buf, img); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	if err := jpeg.Encode(buf, img, nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func isRequestLoggedIn(svr server.Server, r *http.Request) bool {
	_, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
	return err == nil
}

func parsePositiveDimension(raw string) int {
	val, err := strconv.Atoi(raw)
	if err != nil || val <= 0 {
		return 0
	}
	return val
}

func decodeAndResizeMedia(media database.Media, r *http.Request) (image.Image, error) {
	decImage, _, err := image.Decode(bytes.NewReader(media.Bytes))
	if err != nil {
		return nil, err
	}
	he := parsePositiveDimension(r.URL.Query().Get("h"))
	wi := parsePositiveDimension(r.URL.Query().Get("w"))
	if he > 0 && wi > 0 {
		decImage = resize.Resize(uint(wi), uint(he), decImage, resize.Lanczos3)
	}
	return decImage, nil
}

// RetrieveMediaWithPrivacyBlurHandler serves media files and applies backend image blurring when requested by unauthenticated users.
func RetrieveMediaWithPrivacyBlurHandler(svr server.Server) http.HandlerFunc {
	baseHandler := RetrieveMediaPageHandler(svr)
	return func(w http.ResponseWriter, r *http.Request) {
		if isRequestLoggedIn(svr, r) {
			baseHandler(w, r)
			return
		}
		mediaID := mux.Vars(r)["id"]
		media, err := database.GetMediaByID(svr.Conn, mediaID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve media by ID: '%s'", mediaID))
			svr.MEDIA(w, http.StatusNotFound, media.Bytes, media.MediaType)
			return
		}
		decImage, err := decodeAndResizeMedia(media, r)
		if err != nil {
			svr.MEDIA(w, http.StatusOK, media.Bytes, media.MediaType)
			return
		}
		blurred := BlurImage(decImage, defaultPrivacyBlurRadius)
		encoded, err := EncodeImageByMediaType(blurred, media.MediaType)
		if err != nil {
			svr.Log(err, "unable to encode blurred media image")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		svr.MEDIA(w, http.StatusOK, encoded, media.MediaType)
	}
}

// SaveDeveloperProfileWithFaceCheckHandler validates that the uploaded developer profile image contains a human face before saving.
func SaveDeveloperProfileWithFaceCheckHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	userRepo := user.NewRepository(svr.Conn)
	baseHandler := SaveDeveloperProfileHandler(svr, devRepo, userRepo)
	return func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			svr.JSON(w, http.StatusBadRequest, "request is invalid")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		var probe signupImageProbe
		if err := json.Unmarshal(bodyBytes, &probe); err != nil || probe.ProfileImageID == "" {
			svr.JSON(w, http.StatusBadRequest, "profile picture is required")
			return
		}
		media, err := database.GetMediaByID(svr.Conn, probe.ProfileImageID)
		if err != nil || !DetectFaceInImageBytes(media.Bytes) {
			svr.JSON(w, http.StatusBadRequest, "face not detected in profile picture, please upload a clear photo of your face")
			return
		}
		baseHandler(w, r)
	}
}
