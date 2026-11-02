package facedetect

import (
	_ "embed"
	"encoding/binary"
	"image"
	"image/color"
	"math"
	"sort"
	"sync"
)

//go:embed facefinder
var defaultCascade []byte

var (
	classifierOnce sync.Once
	defaultPigo    *Pigo
)

type CascadeParams struct {
	Pixels      []uint8
	Rows        int
	Cols        int
	Dim         int
	MinSize     int
	MaxSize     int
	ShiftFactor float64
	ScaleFactor float64
}

type Pigo struct {
	treeCodes     []int8
	treePred      []float32
	treeThreshold []float32
	treeDepth     uint32
	treeNum       uint32
}

type Detection struct {
	Row   int
	Col   int
	Scale int
	Q     float32
}

func pow2(exp int) int {
	return 1 << exp
}

func UnpackCascade(packet []byte) (*Pigo, error) {
	pos := 8
	treeDepth := binary.LittleEndian.Uint32(packet[pos:])
	pos += 4
	treeNum := binary.LittleEndian.Uint32(packet[pos:])
	pos += 4

	treeThreshold := make([]float32, 0, treeNum)
	treeCodes := make([]int8, 0, 119808)
	treePred := make([]float32, 0, 29952)

	nodesPerTree := pow2(int(treeDepth))
	codeLen := 4*nodesPerTree - 4

	for t := 0; t < int(treeNum); t++ {
		treeCodes = append(treeCodes, 0, 0, 0, 0)
		for k := 0; k < codeLen; k++ {
			treeCodes = append(treeCodes, int8(packet[pos+k]))
		}
		pos += codeLen

		for i := 0; i < nodesPerTree; i++ {
			u32pred := binary.LittleEndian.Uint32(packet[pos:])
			treePred = append(treePred, math.Float32frombits(u32pred))
			pos += 4
		}
		u32thr := binary.LittleEndian.Uint32(packet[pos:])
		treeThreshold = append(treeThreshold, math.Float32frombits(u32thr))
		pos += 4
	}

	return &Pigo{
		treeCodes:     treeCodes,
		treePred:      treePred,
		treeThreshold: treeThreshold,
		treeDepth:     treeDepth,
		treeNum:       treeNum,
	}, nil
}

func (pg *Pigo) classifyRegion(r, c, s, treeDepth int, pixels []uint8, dim int) float32 {
	var root int
	var out float32

	r = r * 256
	c = c * 256

	if pg.treeNum > 0 {
		for i := 0; i < int(pg.treeNum); i++ {
			idx := 1
			for j := 0; j < int(pg.treeDepth); j++ {
				x1 := ((r+int(pg.treeCodes[root+4*idx+0])*s)>>8)*dim + ((c + int(pg.treeCodes[root+4*idx+1])*s) >> 8)
				x2 := ((r+int(pg.treeCodes[root+4*idx+2])*s)>>8)*dim + ((c + int(pg.treeCodes[root+4*idx+3])*s) >> 8)
				if pixels[x1] <= pixels[x2] {
					idx = 2*idx + 1
				} else {
					idx = 2 * idx
				}
			}
			out += pg.treePred[treeDepth*i+idx-treeDepth]
			if out <= pg.treeThreshold[i] {
				return -1.0
			}
			root += 4 * treeDepth
		}
		return out - pg.treeThreshold[pg.treeNum-1]
	}
	return 0.0
}

func (pg *Pigo) RunCascade(cp CascadeParams) []Detection {
	var detections []Detection
	pixels := cp.Pixels
	treeDepth := pow2(int(pg.treeDepth))
	scale := cp.MinSize

	for scale <= cp.MaxSize {
		step := int(math.Max(cp.ShiftFactor*float64(scale), 1))
		offset := scale/2 + 1

		for row := offset; row <= cp.Rows-offset; row += step {
			for col := offset; col <= cp.Cols-offset; col += step {
				q := pg.classifyRegion(row, col, scale, treeDepth, pixels, cp.Dim)
				if q > 0.0 {
					detections = append(detections, Detection{
						Row:   row,
						Col:   col,
						Scale: scale,
						Q:     q,
					})
				}
			}
		}
		scale = int(float64(scale) + math.Max(2, (float64(scale)*cp.ScaleFactor)-float64(scale)))
	}
	return detections
}

func (pg *Pigo) ClusterDetections(detections []Detection, iouThreshold float64) []Detection {
	sort.Slice(detections, func(i, j int) bool {
		return detections[i].Q < detections[j].Q
	})

	calcIoU := func(det1, det2 Detection) float64 {
		r1, c1, s1 := float64(det1.Row), float64(det1.Col), float64(det1.Scale)
		r2, c2, s2 := float64(det2.Row), float64(det2.Col), float64(det2.Scale)
		overRow := math.Max(0, math.Min(r1+s1/2, r2+s2/2)-math.Max(r1-s1/2, r2-s2/2))
		overCol := math.Max(0, math.Min(c1+s1/2, c2+s2/2)-math.Max(c1-s1/2, c2-s2/2))
		return overRow * overCol / (s1*s1 + s2*s2 - overRow*overCol)
	}

	assignments := make([]bool, len(detections))
	var clusters []Detection

	for i := 0; i < len(detections); i++ {
		if !assignments[i] {
			var r, c, s, n int
			var q float32
			for j := 0; j < len(detections); j++ {
				if calcIoU(detections[i], detections[j]) > iouThreshold {
					assignments[j] = true
					r += detections[j].Row
					c += detections[j].Col
					s += detections[j].Scale
					q += detections[j].Q
					n++
				}
			}
			if n > 0 {
				clusters = append(clusters, Detection{
					Row:   r / n,
					Col:   c / n,
					Scale: s / n,
					Q:     q,
				})
			}
		}
	}
	return clusters
}

func rgbToGrayscale(src image.Image) ([]uint8, int, int) {
	bounds := src.Bounds()
	cols, rows := bounds.Dx(), bounds.Dy()
	gray := make([]uint8, rows*cols)
	idx := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			lum := (299*(r>>8) + 587*(g>>8) + 114*(b>>8)) / 1000
			gray[idx] = uint8(lum)
			idx++
		}
	}
	return gray, rows, cols
}

func getClassifier() *Pigo {
	classifierOnce.Do(func() {
		defaultPigo, _ = UnpackCascade(defaultCascade)
	})
	return defaultPigo
}

// HasFace detects whether the provided image contains at least one human face.
func HasFace(img image.Image) bool {
	if img == nil {
		return false
	}
	pixels, rows, cols := rgbToGrayscale(img)
	if rows < 20 || cols < 20 {
		return false
	}
	maxDim := rows
	if cols > maxDim {
		maxDim = cols
	}
	pg := getClassifier()
	if pg == nil {
		return false
	}
	cParams := CascadeParams{
		Pixels:      pixels,
		Rows:        rows,
		Cols:        cols,
		Dim:         cols,
		MinSize:     20,
		MaxSize:     maxDim,
		ShiftFactor: 0.1,
		ScaleFactor: 1.1,
	}
	dets := pg.RunCascade(cParams)
	dets = pg.ClusterDetections(dets, 0.2)
	for _, d := range dets {
		if d.Q >= 5.0 {
			return true
		}
	}
	return false
}

// BlurImage applies a separable box/Gaussian approximation blur with the given radius in pure Go.
func BlurImage(src image.Image, radius int) image.Image {
	if src == nil || radius <= 0 {
		return src
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w == 0 || h == 0 {
		return src
	}

	tmp := image.NewNRGBA(image.Rect(0, 0, w, h))
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))

	// Convert source to NRGBA first
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(src.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			dst.SetNRGBA(x, y, c)
		}
	}

	// 2-pass separable box blur (horizontal + vertical) repeated twice for smooth Gaussian appearance
	for pass := 0; pass < 2; pass++ {
		// Horizontal pass: dst -> tmp
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				var rSum, gSum, bSum, aSum, count int
				xMin := x - radius
				if xMin < 0 {
					xMin = 0
				}
				xMax := x + radius
				if xMax >= w {
					xMax = w - 1
				}
				for kx := xMin; kx <= xMax; kx++ {
					p := dst.NRGBAAt(kx, y)
					rSum += int(p.R)
					gSum += int(p.G)
					bSum += int(p.B)
					aSum += int(p.A)
					count++
				}
				tmp.SetNRGBA(x, y, color.NRGBA{
					R: uint8(rSum / count),
					G: uint8(gSum / count),
					B: uint8(bSum / count),
					A: uint8(aSum / count),
				})
			}
		}

		// Vertical pass: tmp -> dst
		for y := 0; y < h; y++ {
			yMin := y - radius
			if yMin < 0 {
				yMin = 0
			}
			yMax := y + radius
			if yMax >= h {
				yMax = h - 1
			}
			for x := 0; x < w; x++ {
				var rSum, gSum, bSum, aSum, count int
				for ky := yMin; ky <= yMax; ky++ {
					p := tmp.NRGBAAt(x, ky)
					rSum += int(p.R)
					gSum += int(p.G)
					bSum += int(p.B)
					aSum += int(p.A)
					count++
				}
				dst.SetNRGBA(x, y, color.NRGBA{
					R: uint8(rSum / count),
					G: uint8(gSum / count),
					B: uint8(bSum / count),
					A: uint8(aSum / count),
				})
			}
		}
	}

	return dst
}
