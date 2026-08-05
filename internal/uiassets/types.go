// Package uiassets provides deterministic, local image processing for the UI
// workshop. It deliberately has no dependency on the application's persisted
// toolchain types so callers can map public API records at the package boundary.
package uiassets

import (
	"errors"
	"image"
	"image/color"
)

const (
	DefaultMaxImageBytes  int64 = 32 << 20
	DefaultMaxImagePixels int64 = 64 * 1024 * 1024
	ProjectSchema               = "perfectpixel.ui-project"
	ProjectSchemaVersion        = 1
)

var (
	ErrImageTooLarge       = errors.New("encoded image exceeds byte limit")
	ErrImageTooManyPixels  = errors.New("image exceeds pixel limit")
	ErrInvalidImageSize    = errors.New("image has invalid dimensions")
	ErrUnsupportedImage    = errors.New("unsupported image format")
	ErrInvalidRegion       = errors.New("invalid image region")
	ErrInvalidSplitOptions = errors.New("invalid split options")
)

type DecodeLimits struct {
	MaxBytes  int64 `json:"maxBytes"`
	MaxPixels int64 `json:"maxPixels"`
}

func DefaultDecodeLimits() DecodeLimits {
	return DecodeLimits{MaxBytes: DefaultMaxImageBytes, MaxPixels: DefaultMaxImagePixels}
}

type ImageInfo struct {
	Format    string `json:"format"`
	MIMEType  string `json:"mimeType"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	SizeBytes int64  `json:"sizeBytes"`
}

type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (s Size) Valid() bool {
	return s.Width > 0 && s.Height > 0
}

func (s Size) Rect() image.Rectangle {
	return image.Rect(0, 0, s.Width, s.Height)
}

type PixelRect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func RectFromImage(r image.Rectangle) PixelRect {
	return PixelRect{X: r.Min.X, Y: r.Min.Y, Width: r.Dx(), Height: r.Dy()}
}

func (r PixelRect) Empty() bool {
	return r.Width <= 0 || r.Height <= 0
}

func (r PixelRect) ImageRect() image.Rectangle {
	return image.Rect(r.X, r.Y, r.X+r.Width, r.Y+r.Height)
}

func (r PixelRect) Within(size Size) bool {
	if r.Empty() || !size.Valid() || r.X < 0 || r.Y < 0 {
		return false
	}
	return r.X <= size.Width-r.Width && r.Y <= size.Height-r.Height
}

func (r PixelRect) Intersect(size Size) PixelRect {
	return RectFromImage(r.ImageRect().Intersect(size.Rect()))
}

func (r PixelRect) Expand(padding int, size Size) PixelRect {
	if r.Empty() || !size.Valid() {
		return PixelRect{}
	}
	if padding < 0 {
		padding = 0
	}
	return RectFromImage(image.Rect(
		max(0, r.X-padding),
		max(0, r.Y-padding),
		min(size.Width, r.X+r.Width+padding),
		min(size.Height, r.Y+r.Height+padding),
	))
}

type RGBA struct {
	R uint8 `json:"r"`
	G uint8 `json:"g"`
	B uint8 `json:"b"`
	A uint8 `json:"a"`
}

func RGBAFromColor(c color.Color) RGBA {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return RGBA{R: n.R, G: n.G, B: n.B, A: n.A}
}

func (c RGBA) Color() color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A}
}

type SplitMode string

const (
	SplitGrid   SplitMode = "grid"
	SplitManual SplitMode = "manual"
	SplitAuto   SplitMode = "auto"
)

type BackgroundMode string

const (
	BackgroundAuto  BackgroundMode = "auto"
	BackgroundAlpha BackgroundMode = "alpha"
	BackgroundSolid BackgroundMode = "solid"
)

type DetectionOptions struct {
	BackgroundMode  BackgroundMode `json:"backgroundMode"`
	BackgroundColor *RGBA          `json:"backgroundColor,omitempty"`
	AlphaThreshold  uint8          `json:"alphaThreshold"`
	ColorTolerance  uint8          `json:"colorTolerance"`
	MinArea         int            `json:"minArea"`
	Connectivity    int            `json:"connectivity"`
}

func DefaultDetectionOptions() DetectionOptions {
	return DetectionOptions{
		BackgroundMode: BackgroundAuto,
		AlphaThreshold: 8,
		ColorTolerance: 24,
		MinArea:        4,
		Connectivity:   8,
	}
}

type GridOptions struct {
	Rows     int `json:"rows"`
	Columns  int `json:"columns"`
	MarginX  int `json:"marginX,omitempty"`
	MarginY  int `json:"marginY,omitempty"`
	SpacingX int `json:"spacingX,omitempty"`
	SpacingY int `json:"spacingY,omitempty"`
}

type SplitOptions struct {
	Mode      SplitMode        `json:"mode"`
	Grid      GridOptions      `json:"grid,omitempty"`
	Manual    []Region         `json:"manual,omitempty"`
	Detection DetectionOptions `json:"detection"`
	Trim      bool             `json:"trim"`
	Padding   int              `json:"padding"`
}

type Region struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Bounds        PixelRect `json:"bounds"`
	ContentBounds PixelRect `json:"contentBounds,omitempty"`
	PixelCount    int       `json:"pixelCount,omitempty"`
}

type Crop struct {
	Region Region       `json:"region"`
	Image  *image.NRGBA `json:"-"`
}

type MaskOptions struct {
	Padding int  `json:"padding"`
	Feather int  `json:"feather"`
	Invert  bool `json:"invert"`
}

// LayoutRegion is the package-local representation of a provider-derived UI
// region. Bounds are always source-image pixel coordinates.
type LayoutRegion struct {
	ID            string         `json:"id"`
	ParentID      string         `json:"parentId,omitempty"`
	Name          string         `json:"name"`
	Type          string         `json:"type"`
	Bounds        PixelRect      `json:"bounds"`
	ZIndex        int            `json:"zIndex"`
	Confidence    float64        `json:"confidence"`
	Text          string         `json:"text,omitempty"`
	Style         map[string]any `json:"style,omitempty"`
	ExportEnabled bool           `json:"exportEnabled"`
}

type LayoutDocument struct {
	Canvas  Size           `json:"canvas"`
	Regions []LayoutRegion `json:"regions"`
}

type ExportProject struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Source           image.Image    `json:"-"`
	Assets           []Crop         `json:"-"`
	Layout           LayoutDocument `json:"layout"`
	Tokens           map[string]any `json:"tokens"`
	Copy             string         `json:"copy"`
	RecreationPrompt string         `json:"recreationPrompt"`
}
