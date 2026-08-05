package uiassets

import (
	"fmt"
	"image"
	"image/color"
)

// CreateMask builds an 8-bit mask where selected regions are white. Padding is
// applied before feathering; Invert swaps selected and unselected values.
func CreateMask(canvas Size, regions []PixelRect, options MaskOptions) (*image.Alpha, error) {
	if !canvas.Valid() {
		return nil, fmt.Errorf("%w: mask canvas %dx%d", ErrInvalidImageSize, canvas.Width, canvas.Height)
	}
	if options.Padding < 0 || options.Feather < 0 {
		return nil, fmt.Errorf("%w: negative mask padding or feather", ErrInvalidRegion)
	}
	mask := image.NewAlpha(canvas.Rect())
	for _, region := range regions {
		if region.Empty() || region.ImageRect().Intersect(canvas.Rect()).Empty() {
			return nil, fmt.Errorf("%w: mask region %+v", ErrInvalidRegion, region)
		}
		clipped := region.Intersect(canvas).Expand(options.Padding, canvas)
		for y := clipped.Y; y < clipped.Y+clipped.Height; y++ {
			start := mask.PixOffset(clipped.X, y)
			for x := 0; x < clipped.Width; x++ {
				mask.Pix[start+x] = 255
			}
		}
	}
	if options.Feather > 0 {
		mask = blurAlpha(mask, options.Feather)
	}
	if options.Invert {
		for i := range mask.Pix {
			mask.Pix[i] = 255 - mask.Pix[i]
		}
	}
	return mask, nil
}

// Composite replaces base pixels with edited pixels according to mask
// coverage. Unlike source-over compositing, a transparent edited pixel can
// intentionally clear the selected base pixel.
func Composite(base, edited image.Image, mask *image.Alpha) (*image.NRGBA, error) {
	baseImage := ToNRGBA(base)
	editedImage := ToNRGBA(edited)
	if baseImage == nil || editedImage == nil || mask == nil {
		return nil, fmt.Errorf("composite: base, edited image, and mask are required")
	}
	if baseImage.Bounds().Size() != editedImage.Bounds().Size() || baseImage.Bounds().Size() != mask.Bounds().Size() {
		return nil, fmt.Errorf("composite: image and mask sizes must match")
	}
	maskImage := normalizeAlpha(mask)
	out := image.NewNRGBA(baseImage.Bounds())
	for y := 0; y < out.Bounds().Dy(); y++ {
		for x := 0; x < out.Bounds().Dx(); x++ {
			bi := baseImage.PixOffset(x, y)
			ei := editedImage.PixOffset(x, y)
			mi := maskImage.PixOffset(x, y)
			oi := out.PixOffset(x, y)
			blendReplacement(out.Pix[oi:oi+4], baseImage.Pix[bi:bi+4], editedImage.Pix[ei:ei+4], maskImage.Pix[mi])
		}
	}
	return out, nil
}

// CompositeRegion applies a crop-sized edit and crop-sized mask at bounds in
// the base image. A nil mask means fully replace the selected rectangle.
func CompositeRegion(base, edited image.Image, bounds PixelRect, mask *image.Alpha) (*image.NRGBA, error) {
	baseImage := ToNRGBA(base)
	editedImage := ToNRGBA(edited)
	if baseImage == nil || editedImage == nil {
		return nil, fmt.Errorf("composite region: base and edited image are required")
	}
	size := Size{Width: baseImage.Bounds().Dx(), Height: baseImage.Bounds().Dy()}
	if !bounds.Within(size) {
		return nil, fmt.Errorf("%w: composite bounds %+v", ErrInvalidRegion, bounds)
	}
	if editedImage.Bounds().Dx() != bounds.Width || editedImage.Bounds().Dy() != bounds.Height {
		return nil, fmt.Errorf("composite region: edited image must be %dx%d", bounds.Width, bounds.Height)
	}
	var maskImage *image.Alpha
	if mask == nil {
		maskImage = image.NewAlpha(image.Rect(0, 0, bounds.Width, bounds.Height))
		for i := range maskImage.Pix {
			maskImage.Pix[i] = 255
		}
	} else {
		if mask.Bounds().Dx() != bounds.Width || mask.Bounds().Dy() != bounds.Height {
			return nil, fmt.Errorf("composite region: mask must be %dx%d", bounds.Width, bounds.Height)
		}
		maskImage = normalizeAlpha(mask)
	}
	out := image.NewNRGBA(baseImage.Bounds())
	copy(out.Pix, baseImage.Pix)
	for y := 0; y < bounds.Height; y++ {
		for x := 0; x < bounds.Width; x++ {
			bi := out.PixOffset(bounds.X+x, bounds.Y+y)
			ei := editedImage.PixOffset(x, y)
			mi := maskImage.PixOffset(x, y)
			basePixel := [4]uint8{out.Pix[bi], out.Pix[bi+1], out.Pix[bi+2], out.Pix[bi+3]}
			blendReplacement(out.Pix[bi:bi+4], basePixel[:], editedImage.Pix[ei:ei+4], maskImage.Pix[mi])
		}
	}
	return out, nil
}

func normalizeAlpha(src *image.Alpha) *image.Alpha {
	if src.Bounds().Min == (image.Point{}) {
		return src
	}
	dst := image.NewAlpha(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			dst.SetAlpha(x, y, src.AlphaAt(src.Bounds().Min.X+x, src.Bounds().Min.Y+y))
		}
	}
	return dst
}

func blendReplacement(dst, base, edited []uint8, mask uint8) {
	if mask == 0 {
		copy(dst, base)
		return
	}
	if mask == 255 {
		copy(dst, edited)
		return
	}
	m := int(mask)
	inv := 255 - m
	baseAlpha, editedAlpha := int(base[3]), int(edited[3])
	weightedAlpha := baseAlpha*inv + editedAlpha*m
	if weightedAlpha == 0 {
		clear(dst)
		return
	}
	for channel := 0; channel < 3; channel++ {
		numerator := int(base[channel])*baseAlpha*inv + int(edited[channel])*editedAlpha*m
		dst[channel] = uint8((numerator + weightedAlpha/2) / weightedAlpha)
	}
	dst[3] = uint8((weightedAlpha + 127) / 255)
}

func blurAlpha(src *image.Alpha, radius int) *image.Alpha {
	if radius <= 0 {
		return src
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	tmp := image.NewAlpha(image.Rect(0, 0, w, h))
	dst := image.NewAlpha(image.Rect(0, 0, w, h))
	prefix := make([]int, max(w, h)+1)
	for y := 0; y < h; y++ {
		prefix[0] = 0
		for x := 0; x < w; x++ {
			prefix[x+1] = prefix[x] + int(src.AlphaAt(src.Bounds().Min.X+x, src.Bounds().Min.Y+y).A)
		}
		for x := 0; x < w; x++ {
			left, right := max(0, x-radius), min(w, x+radius+1)
			tmp.SetAlpha(x, y, color.Alpha{A: uint8((prefix[right] - prefix[left]) / (right - left))})
		}
	}
	for x := 0; x < w; x++ {
		prefix[0] = 0
		for y := 0; y < h; y++ {
			prefix[y+1] = prefix[y] + int(tmp.AlphaAt(x, y).A)
		}
		for y := 0; y < h; y++ {
			top, bottom := max(0, y-radius), min(h, y+radius+1)
			dst.SetAlpha(x, y, color.Alpha{A: uint8((prefix[bottom] - prefix[top]) / (bottom - top))})
		}
	}
	return dst
}
