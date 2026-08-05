package uiassets

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"sort"
)

type foregroundDetector struct {
	img        *image.NRGBA
	mode       BackgroundMode
	alpha      uint8
	tolerance  uint8
	background color.NRGBA
}

// SampleBackground returns the dominant quantized color along the image edge.
// It is intended for sheets whose sprites or UI elements sit on a flat color.
func SampleBackground(src image.Image, alphaThreshold uint8) (RGBA, error) {
	img := ToNRGBA(src)
	if img == nil || img.Bounds().Empty() {
		return RGBA{}, fmt.Errorf("%w: empty source", ErrInvalidImageSize)
	}
	type bucket struct {
		count          int
		r, g, b, alpha int64
	}
	buckets := make(map[uint16]*bucket)
	add := func(x, y int) {
		i := img.PixOffset(x, y)
		r, g, b, a := img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]
		if a <= alphaThreshold {
			return
		}
		key := uint16(r>>4)<<8 | uint16(g>>4)<<4 | uint16(b>>4)
		entry := buckets[key]
		if entry == nil {
			entry = &bucket{}
			buckets[key] = entry
		}
		entry.count++
		entry.r += int64(r)
		entry.g += int64(g)
		entry.b += int64(b)
		entry.alpha += int64(a)
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	for x := 0; x < w; x++ {
		add(x, 0)
		if h > 1 {
			add(x, h-1)
		}
	}
	for y := 1; y < h-1; y++ {
		add(0, y)
		if w > 1 {
			add(w-1, y)
		}
	}
	if len(buckets) == 0 {
		return RGBA{}, fmt.Errorf("sample background: image edge is fully transparent")
	}
	keys := make([]int, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, int(key))
	}
	sort.Ints(keys)
	bestKey := keys[0]
	for _, key := range keys[1:] {
		if buckets[uint16(key)].count > buckets[uint16(bestKey)].count {
			bestKey = key
		}
	}
	best := buckets[uint16(bestKey)]
	n := int64(best.count)
	return RGBA{
		R: uint8(best.r / n),
		G: uint8(best.g / n),
		B: uint8(best.b / n),
		A: uint8(best.alpha / n),
	}, nil
}

// DetectRegions finds connected foreground components and returns them in a
// stable top-to-bottom, left-to-right order.
func DetectRegions(src image.Image, options DetectionOptions) ([]Region, error) {
	img := ToNRGBA(src)
	if img == nil || img.Bounds().Empty() {
		return nil, fmt.Errorf("%w: empty source", ErrInvalidImageSize)
	}
	detector, normalized, err := newForegroundDetector(img, options)
	if err != nil {
		return nil, err
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	visited := make([]bool, w*h)
	queue := make([]int, 0, 1024)
	regions := make([]Region, 0)
	neighbors := [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
	if normalized.Connectivity == 8 {
		neighbors = append(neighbors, [2]int{-1, -1}, [2]int{1, -1}, [2]int{-1, 1}, [2]int{1, 1})
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			start := y*w + x
			if visited[start] {
				continue
			}
			visited[start] = true
			if !detector.foreground(x, y) {
				continue
			}
			queue = append(queue[:0], start)
			minX, minY, maxX, maxY, area := x, y, x, y, 0
			for head := 0; head < len(queue); head++ {
				p := queue[head]
				px, py := p%w, p/w
				area++
				minX, minY = min(minX, px), min(minY, py)
				maxX, maxY = max(maxX, px), max(maxY, py)
				for _, delta := range neighbors {
					nx, ny := px+delta[0], py+delta[1]
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					next := ny*w + nx
					if visited[next] {
						continue
					}
					visited[next] = true
					if detector.foreground(nx, ny) {
						queue = append(queue, next)
					}
				}
			}
			if area < normalized.MinArea {
				continue
			}
			bounds := PixelRect{X: minX, Y: minY, Width: maxX - minX + 1, Height: maxY - minY + 1}
			regions = append(regions, Region{Bounds: bounds, ContentBounds: bounds, PixelCount: area})
		}
	}
	sortRegions(regions)
	for i := range regions {
		regions[i].ID = fmt.Sprintf("component-%03d", i+1)
		regions[i].Name = fmt.Sprintf("Component %03d", i+1)
	}
	return regions, nil
}

// TrimBounds returns the foreground content bounds within a source rectangle,
// expanded by padding and clipped to the source canvas. An empty result means
// the rectangle contains no foreground pixels.
func TrimBounds(src image.Image, within PixelRect, options DetectionOptions, padding int) (PixelRect, error) {
	img := ToNRGBA(src)
	if img == nil || img.Bounds().Empty() {
		return PixelRect{}, fmt.Errorf("%w: empty source", ErrInvalidImageSize)
	}
	if padding < 0 {
		return PixelRect{}, fmt.Errorf("%w: negative padding", ErrInvalidSplitOptions)
	}
	size := Size{Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}
	if !within.Within(size) {
		return PixelRect{}, fmt.Errorf("%w: %+v outside %dx%d", ErrInvalidRegion, within, size.Width, size.Height)
	}
	detector, _, err := newForegroundDetector(img, options)
	if err != nil {
		return PixelRect{}, err
	}
	trimmed, _ := trimWithDetector(detector, within)
	return trimmed.Expand(padding, size), nil
}

// CropImage copies a source-image rectangle into a zero-origin NRGBA image.
func CropImage(src image.Image, bounds PixelRect) (*image.NRGBA, error) {
	img := ToNRGBA(src)
	if img == nil || img.Bounds().Empty() {
		return nil, fmt.Errorf("%w: empty source", ErrInvalidImageSize)
	}
	size := Size{Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}
	if !bounds.Within(size) {
		return nil, fmt.Errorf("%w: %+v outside %dx%d", ErrInvalidRegion, bounds, size.Width, size.Height)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, bounds.Width, bounds.Height))
	draw.Draw(dst, dst.Bounds(), img, image.Pt(bounds.X, bounds.Y), draw.Src)
	return dst, nil
}

// Split performs grid, manual, or connected-component slicing. Empty cells are
// omitted only when Trim is enabled.
func Split(src image.Image, options SplitOptions) ([]Crop, error) {
	img := ToNRGBA(src)
	if img == nil || img.Bounds().Empty() {
		return nil, fmt.Errorf("%w: empty source", ErrInvalidImageSize)
	}
	if options.Padding < 0 {
		return nil, fmt.Errorf("%w: negative padding", ErrInvalidSplitOptions)
	}
	size := Size{Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}
	var regions []Region
	var err error
	switch options.Mode {
	case SplitGrid:
		regions, err = gridRegions(size, options.Grid)
	case SplitManual:
		regions = append([]Region(nil), options.Manual...)
		if len(regions) == 0 {
			err = fmt.Errorf("%w: manual regions are empty", ErrInvalidSplitOptions)
		}
	case SplitAuto:
		regions, err = DetectRegions(img, options.Detection)
	default:
		err = fmt.Errorf("%w: unknown mode %q", ErrInvalidSplitOptions, options.Mode)
	}
	if err != nil {
		return nil, err
	}

	detector, _, err := newForegroundDetector(img, options.Detection)
	if err != nil {
		return nil, err
	}
	prepared := make([]Region, 0, len(regions))
	for _, region := range regions {
		if !region.Bounds.Within(size) {
			return nil, fmt.Errorf("%w: region %q bounds %+v outside %dx%d", ErrInvalidRegion, region.ID, region.Bounds, size.Width, size.Height)
		}
		content, pixels := trimWithDetector(detector, region.Bounds)
		if content.Empty() && options.Trim {
			continue
		}
		region.ContentBounds = content
		region.PixelCount = pixels
		if options.Trim && !content.Empty() {
			region.Bounds = content
		}
		region.Bounds = region.Bounds.Expand(options.Padding, size)
		prepared = append(prepared, region)
	}
	sortRegions(prepared)
	if err := assignAndValidateRegionIDs(prepared, options.Mode); err != nil {
		return nil, err
	}
	crops := make([]Crop, 0, len(prepared))
	for _, region := range prepared {
		crop, err := CropImage(img, region.Bounds)
		if err != nil {
			return nil, err
		}
		crops = append(crops, Crop{Region: region, Image: crop})
	}
	return crops, nil
}

func newForegroundDetector(img *image.NRGBA, options DetectionOptions) (foregroundDetector, DetectionOptions, error) {
	if options.BackgroundMode == "" {
		options.BackgroundMode = BackgroundAuto
	}
	if options.ColorTolerance == 0 {
		options.ColorTolerance = 24
	}
	if options.MinArea <= 0 {
		options.MinArea = 1
	}
	if options.Connectivity == 0 {
		options.Connectivity = 8
	}
	if options.Connectivity != 4 && options.Connectivity != 8 {
		return foregroundDetector{}, options, fmt.Errorf("%w: connectivity must be 4 or 8", ErrInvalidSplitOptions)
	}
	mode := options.BackgroundMode
	if mode != BackgroundAuto && mode != BackgroundAlpha && mode != BackgroundSolid {
		return foregroundDetector{}, options, fmt.Errorf("%w: unknown background mode %q", ErrInvalidSplitOptions, mode)
	}
	if mode == BackgroundAuto {
		if hasTransparentBackground(img, options.AlphaThreshold) {
			mode = BackgroundAlpha
		} else {
			mode = BackgroundSolid
		}
	}
	detector := foregroundDetector{img: img, mode: mode, alpha: options.AlphaThreshold, tolerance: options.ColorTolerance}
	if mode == BackgroundSolid {
		if options.BackgroundColor != nil {
			detector.background = options.BackgroundColor.Color()
		} else {
			background, err := SampleBackground(img, options.AlphaThreshold)
			if err != nil {
				return foregroundDetector{}, options, err
			}
			detector.background = background.Color()
		}
	}
	return detector, options, nil
}

func (d foregroundDetector) foreground(x, y int) bool {
	i := d.img.PixOffset(x, y)
	r, g, b, a := d.img.Pix[i], d.img.Pix[i+1], d.img.Pix[i+2], d.img.Pix[i+3]
	if a <= d.alpha {
		return false
	}
	if d.mode == BackgroundAlpha {
		return true
	}
	dr := int(r) - int(d.background.R)
	dg := int(g) - int(d.background.G)
	db := int(b) - int(d.background.B)
	tol := int(d.tolerance)
	return dr*dr+dg*dg+db*db > 3*tol*tol
}

func hasTransparentBackground(img *image.NRGBA, threshold uint8) bool {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	border, transparentBorder := 0, 0
	count := func(x, y int) {
		border++
		if img.Pix[img.PixOffset(x, y)+3] <= threshold {
			transparentBorder++
		}
	}
	for x := 0; x < w; x++ {
		count(x, 0)
		if h > 1 {
			count(x, h-1)
		}
	}
	for y := 1; y < h-1; y++ {
		count(0, y)
		if w > 1 {
			count(w-1, y)
		}
	}
	if transparentBorder*20 >= border {
		return true
	}
	transparent := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] <= threshold {
			transparent++
		}
	}
	return transparent*100 >= w*h
}

func trimWithDetector(detector foregroundDetector, within PixelRect) (PixelRect, int) {
	minX, minY := within.X+within.Width, within.Y+within.Height
	maxX, maxY, pixels := within.X-1, within.Y-1, 0
	for y := within.Y; y < within.Y+within.Height; y++ {
		for x := within.X; x < within.X+within.Width; x++ {
			if !detector.foreground(x, y) {
				continue
			}
			pixels++
			minX, minY = min(minX, x), min(minY, y)
			maxX, maxY = max(maxX, x), max(maxY, y)
		}
	}
	if pixels == 0 {
		return PixelRect{}, 0
	}
	return PixelRect{X: minX, Y: minY, Width: maxX - minX + 1, Height: maxY - minY + 1}, pixels
}

func gridRegions(size Size, grid GridOptions) ([]Region, error) {
	if grid.Rows <= 0 || grid.Columns <= 0 || grid.MarginX < 0 || grid.MarginY < 0 || grid.SpacingX < 0 || grid.SpacingY < 0 {
		return nil, fmt.Errorf("%w: invalid grid dimensions", ErrInvalidSplitOptions)
	}
	usableW := size.Width - 2*grid.MarginX - (grid.Columns-1)*grid.SpacingX
	usableH := size.Height - 2*grid.MarginY - (grid.Rows-1)*grid.SpacingY
	if usableW < grid.Columns || usableH < grid.Rows {
		return nil, fmt.Errorf("%w: grid cells have no area", ErrInvalidSplitOptions)
	}
	regions := make([]Region, 0, grid.Rows*grid.Columns)
	for row := 0; row < grid.Rows; row++ {
		y0 := grid.MarginY + row*usableH/grid.Rows + row*grid.SpacingY
		y1 := grid.MarginY + (row+1)*usableH/grid.Rows + row*grid.SpacingY
		for column := 0; column < grid.Columns; column++ {
			x0 := grid.MarginX + column*usableW/grid.Columns + column*grid.SpacingX
			x1 := grid.MarginX + (column+1)*usableW/grid.Columns + column*grid.SpacingX
			regions = append(regions, Region{
				ID:     fmt.Sprintf("grid-r%02d-c%02d", row+1, column+1),
				Name:   fmt.Sprintf("Grid %d,%d", row+1, column+1),
				Bounds: PixelRect{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0},
			})
		}
	}
	return regions, nil
}

func assignAndValidateRegionIDs(regions []Region, mode SplitMode) error {
	seen := make(map[string]struct{}, len(regions))
	prefix := "region"
	if mode == SplitManual {
		prefix = "manual"
	}
	for i := range regions {
		if regions[i].ID == "" {
			regions[i].ID = fmt.Sprintf("%s-%03d", prefix, i+1)
		}
		if _, exists := seen[regions[i].ID]; exists {
			return fmt.Errorf("%w: duplicate region id %q", ErrInvalidRegion, regions[i].ID)
		}
		seen[regions[i].ID] = struct{}{}
		if regions[i].Name == "" {
			regions[i].Name = regions[i].ID
		}
	}
	return nil
}

func sortRegions(regions []Region) {
	sort.SliceStable(regions, func(i, j int) bool {
		a, b := regions[i], regions[j]
		if a.Bounds.Y != b.Bounds.Y {
			return a.Bounds.Y < b.Bounds.Y
		}
		if a.Bounds.X != b.Bounds.X {
			return a.Bounds.X < b.Bounds.X
		}
		if a.Bounds.Height != b.Bounds.Height {
			return a.Bounds.Height < b.Bounds.Height
		}
		if a.Bounds.Width != b.Bounds.Width {
			return a.Bounds.Width < b.Bounds.Width
		}
		return a.ID < b.ID
	})
}
