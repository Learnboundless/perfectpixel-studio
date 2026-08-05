package uiassets

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"path"
	"sort"
	"strings"
	"time"
)

type projectManifest struct {
	Schema    string          `json:"schema"`
	Version   int             `json:"version"`
	ProjectID string          `json:"projectId"`
	Name      string          `json:"name"`
	Source    manifestSource  `json:"source"`
	Assets    []manifestAsset `json:"assets"`
}

type manifestSource struct {
	File   string `json:"file"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type manifestAsset struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	File          string    `json:"file"`
	Width         int       `json:"width"`
	Height        int       `json:"height"`
	Bounds        PixelRect `json:"bounds"`
	ContentBounds PixelRect `json:"contentBounds,omitempty"`
	PixelCount    int       `json:"pixelCount,omitempty"`
}

// BuildProjectZIP returns the complete deterministic UI project archive.
func BuildProjectZIP(project ExportProject) ([]byte, error) {
	var output bytes.Buffer
	if err := ExportProjectZIP(&output, project); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// ExportProjectZIP writes a complete UI project archive. If Assets is empty,
// export-enabled layout regions are cropped directly from Source.
func ExportProjectZIP(output io.Writer, project ExportProject) error {
	if output == nil {
		return fmt.Errorf("export project: output is required")
	}
	source := ToNRGBA(project.Source)
	if source == nil || source.Bounds().Empty() {
		return fmt.Errorf("export project: %w", ErrInvalidImageSize)
	}
	canvas := Size{Width: source.Bounds().Dx(), Height: source.Bounds().Dy()}
	layout := project.Layout
	if !layout.Canvas.Valid() {
		layout.Canvas = canvas
	}
	if layout.Canvas != canvas {
		return fmt.Errorf("export project: layout canvas %dx%d does not match source %dx%d", layout.Canvas.Width, layout.Canvas.Height, canvas.Width, canvas.Height)
	}
	if err := validateLayout(layout, canvas); err != nil {
		return err
	}
	assets, err := materializeAssets(source, project.Assets, layout)
	if err != nil {
		return err
	}
	sortCrops(assets)

	usedFiles := make(map[string]struct{}, len(assets))
	manifestAssets := make([]manifestAsset, 0, len(assets))
	assetFiles := make([]struct {
		name string
		data []byte
	}, 0, len(assets))
	for i, asset := range assets {
		base := slug(asset.Region.Name)
		if base == "" {
			base = slug(asset.Region.ID)
		}
		if base == "" {
			base = fmt.Sprintf("asset-%03d", i+1)
		}
		candidate := base
		for suffix := 2; ; suffix++ {
			if _, exists := usedFiles[candidate]; !exists {
				break
			}
			candidate = fmt.Sprintf("%s-%d", base, suffix)
		}
		usedFiles[candidate] = struct{}{}
		filename := path.Join("assets", candidate+".png")
		encoded, err := encodePNG(asset.Image)
		if err != nil {
			return fmt.Errorf("export asset %q: %w", asset.Region.ID, err)
		}
		assetFiles = append(assetFiles, struct {
			name string
			data []byte
		}{name: filename, data: encoded})
		manifestAssets = append(manifestAssets, manifestAsset{
			ID:            asset.Region.ID,
			Name:          asset.Region.Name,
			File:          filename,
			Width:         asset.Image.Bounds().Dx(),
			Height:        asset.Image.Bounds().Dy(),
			Bounds:        asset.Region.Bounds,
			ContentBounds: asset.Region.ContentBounds,
			PixelCount:    asset.Region.PixelCount,
		})
	}

	manifest := projectManifest{
		Schema:    ProjectSchema,
		Version:   ProjectSchemaVersion,
		ProjectID: project.ID,
		Name:      project.Name,
		Source:    manifestSource{File: "source.png", Width: canvas.Width, Height: canvas.Height},
		Assets:    manifestAssets,
	}
	sourcePNG, err := encodePNG(source)
	if err != nil {
		return fmt.Errorf("export source: %w", err)
	}
	manifestJSON, err := marshalJSON(manifest)
	if err != nil {
		return fmt.Errorf("export manifest: %w", err)
	}
	layout.Regions = sortedLayoutRegions(layout.Regions)
	layoutJSON, err := marshalJSON(layout)
	if err != nil {
		return fmt.Errorf("export layout: %w", err)
	}
	tokens := project.Tokens
	if tokens == nil {
		tokens = map[string]any{}
	}
	tokensJSON, err := marshalJSON(tokens)
	if err != nil {
		return fmt.Errorf("export tokens: %w", err)
	}

	archive := zip.NewWriter(output)
	files := []struct {
		name string
		data []byte
	}{
		{name: "source.png", data: sourcePNG},
	}
	files = append(files, assetFiles...)
	files = append(files,
		struct {
			name string
			data []byte
		}{name: "manifest.json", data: manifestJSON},
		struct {
			name string
			data []byte
		}{name: "layout.json", data: layoutJSON},
		struct {
			name string
			data []byte
		}{name: "tokens.json", data: tokensJSON},
		struct {
			name string
			data []byte
		}{name: "copy.txt", data: []byte(project.Copy)},
		struct {
			name string
			data []byte
		}{name: "recreation-prompt.md", data: []byte(project.RecreationPrompt)},
	)
	for _, file := range files {
		if err := writeZIPFile(archive, file.name, file.data); err != nil {
			_ = archive.Close()
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return fmt.Errorf("close project archive: %w", err)
	}
	return nil
}

func materializeAssets(source *image.NRGBA, explicit []Crop, layout LayoutDocument) ([]Crop, error) {
	assets := append([]Crop(nil), explicit...)
	if len(assets) == 0 {
		for _, region := range layout.Regions {
			if !region.ExportEnabled {
				continue
			}
			assets = append(assets, Crop{Region: Region{ID: region.ID, Name: region.Name, Bounds: region.Bounds}})
		}
	}
	sortCrops(assets)
	seen := make(map[string]struct{}, len(assets))
	canvas := Size{Width: source.Bounds().Dx(), Height: source.Bounds().Dy()}
	for i := range assets {
		asset := &assets[i]
		if asset.Region.ID == "" {
			asset.Region.ID = fmt.Sprintf("asset-%03d", i+1)
		}
		if asset.Region.Name == "" {
			asset.Region.Name = asset.Region.ID
		}
		if _, exists := seen[asset.Region.ID]; exists {
			return nil, fmt.Errorf("export project: duplicate asset id %q", asset.Region.ID)
		}
		seen[asset.Region.ID] = struct{}{}
		if !asset.Region.Bounds.Within(canvas) {
			return nil, fmt.Errorf("export project: %w %q bounds %+v", ErrInvalidRegion, asset.Region.ID, asset.Region.Bounds)
		}
		if asset.Image == nil {
			crop, err := CropImage(source, asset.Region.Bounds)
			if err != nil {
				return nil, err
			}
			asset.Image = crop
		} else {
			asset.Image = ToNRGBA(asset.Image)
		}
		if asset.Image.Bounds().Dx() != asset.Region.Bounds.Width || asset.Image.Bounds().Dy() != asset.Region.Bounds.Height {
			return nil, fmt.Errorf("export project: asset %q image is %dx%d, bounds require %dx%d", asset.Region.ID, asset.Image.Bounds().Dx(), asset.Image.Bounds().Dy(), asset.Region.Bounds.Width, asset.Region.Bounds.Height)
		}
	}
	return assets, nil
}

func validateLayout(layout LayoutDocument, canvas Size) error {
	seen := make(map[string]struct{}, len(layout.Regions))
	for _, region := range layout.Regions {
		if region.ID == "" {
			return fmt.Errorf("export project: layout region id is required")
		}
		if _, exists := seen[region.ID]; exists {
			return fmt.Errorf("export project: duplicate layout region id %q", region.ID)
		}
		seen[region.ID] = struct{}{}
		if !region.Bounds.Within(canvas) {
			return fmt.Errorf("export project: %w layout region %q bounds %+v", ErrInvalidRegion, region.ID, region.Bounds)
		}
		if region.Confidence < 0 || region.Confidence > 1 {
			return fmt.Errorf("export project: layout region %q confidence must be within [0,1]", region.ID)
		}
	}
	for _, region := range layout.Regions {
		if region.ParentID == "" {
			continue
		}
		if region.ParentID == region.ID {
			return fmt.Errorf("export project: layout region %q cannot parent itself", region.ID)
		}
		if _, exists := seen[region.ParentID]; !exists {
			return fmt.Errorf("export project: layout region %q has unknown parent %q", region.ID, region.ParentID)
		}
	}
	parents := make(map[string]string, len(layout.Regions))
	for _, region := range layout.Regions {
		parents[region.ID] = region.ParentID
	}
	for id := range parents {
		visiting := make(map[string]struct{})
		for current := id; current != ""; current = parents[current] {
			if _, exists := visiting[current]; exists {
				return fmt.Errorf("export project: layout hierarchy contains a cycle at %q", current)
			}
			visiting[current] = struct{}{}
		}
	}
	return nil
}

func sortedLayoutRegions(input []LayoutRegion) []LayoutRegion {
	regions := append([]LayoutRegion(nil), input...)
	sort.SliceStable(regions, func(i, j int) bool {
		a, b := regions[i], regions[j]
		if a.ZIndex != b.ZIndex {
			return a.ZIndex < b.ZIndex
		}
		if a.Bounds.Y != b.Bounds.Y {
			return a.Bounds.Y < b.Bounds.Y
		}
		if a.Bounds.X != b.Bounds.X {
			return a.Bounds.X < b.Bounds.X
		}
		return a.ID < b.ID
	})
	return regions
}

func sortCrops(crops []Crop) {
	sort.SliceStable(crops, func(i, j int) bool {
		a, b := crops[i].Region, crops[j].Region
		if a.Bounds.Y != b.Bounds.Y {
			return a.Bounds.Y < b.Bounds.Y
		}
		if a.Bounds.X != b.Bounds.X {
			return a.Bounds.X < b.Bounds.X
		}
		return a.ID < b.ID
	})
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var result strings.Builder
	separator := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
			separator = false
			continue
		}
		if result.Len() > 0 && !separator {
			result.WriteByte('-')
			separator = true
		}
	}
	return strings.Trim(result.String(), "-")
}

func encodePNG(img image.Image) ([]byte, error) {
	var output bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&output, img); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func marshalJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func writeZIPFile(archive *zip.Writer, name string, data []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o644)
	header.SetModTime(time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC))
	entry, err := archive.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create archive entry %q: %w", name, err)
	}
	if _, err := entry.Write(data); err != nil {
		return fmt.Errorf("write archive entry %q: %w", name, err)
	}
	return nil
}
