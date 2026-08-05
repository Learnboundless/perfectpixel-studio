package sprite

import (
	"image"

	xdraw "golang.org/x/image/draw"
)

const (
	FrameFitContain    = "contain"
	FrameFitAutoExpand = "auto_expand"
	FrameFitCrop       = "crop"
)

func NormalizeFrameFitMode(mode string) string {
	switch mode {
	case FrameFitAutoExpand, FrameFitCrop:
		return mode
	default:
		return FrameFitContain
	}
}

// PrepareFramesForAtlas places frame canvases into a common export cell.
func PrepareFramesForAtlas(frames []*image.NRGBA, cellW, cellH int, mode string) []*image.NRGBA {
	mode = NormalizeFrameFitMode(mode)
	cellW = max(1, cellW)
	cellH = max(1, cellH)
	out := make([]*image.NRGBA, 0, len(frames))
	for _, src := range frames {
		if src == nil || src.Rect.Empty() {
			continue
		}
		sw, sh := src.Rect.Dx(), src.Rect.Dy()
		drawSrc := src
		if mode == FrameFitContain && (sw > cellW || sh > cellH) {
			scale := minf(float64(cellW)/float64(sw), float64(cellH)/float64(sh))
			sw = max(1, int(float64(sw)*scale+0.5))
			sh = max(1, int(float64(sh)*scale+0.5))
			drawSrc = image.NewNRGBA(image.Rect(0, 0, sw, sh))
			xdraw.NearestNeighbor.Scale(drawSrc, drawSrc.Rect, src, src.Rect, xdraw.Src, nil)
		}
		dst := image.NewNRGBA(image.Rect(0, 0, cellW, cellH))
		xdraw.Copy(dst, image.Point{X: (cellW - sw) / 2, Y: (cellH - sh) / 2}, drawSrc, drawSrc.Rect, xdraw.Over, nil)
		out = append(out, dst)
	}
	return out
}

func ExpandedCellSize(frames []*image.NRGBA, minimum int) int {
	size := max(1, minimum)
	for _, frame := range frames {
		if frame == nil {
			continue
		}
		size = max(size, max(frame.Rect.Dx(), frame.Rect.Dy()))
	}
	return size
}
