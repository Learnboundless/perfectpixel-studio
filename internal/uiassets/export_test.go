package uiassets

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"slices"
	"sort"
	"testing"
)

func TestBuildProjectZIPContainsCompleteDeterministicProject(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	fillTestRect(source, source.Bounds(), color.NRGBA{R: 12, G: 14, B: 18, A: 255})
	fillTestRect(source, image.Rect(1, 2, 4, 4), color.NRGBA{R: 40, G: 180, B: 220, A: 255})
	project := ExportProject{
		ID:     "inventory-screen",
		Name:   "Inventory Screen",
		Source: source,
		Layout: LayoutDocument{
			Canvas: Size{Width: 8, Height: 6},
			Regions: []LayoutRegion{{
				ID:            "play-button",
				Name:          "Play Button",
				Type:          "button",
				Bounds:        PixelRect{X: 1, Y: 2, Width: 3, Height: 2},
				ZIndex:        2,
				Confidence:    0.95,
				Text:          "Play",
				ExportEnabled: true,
			}},
		},
		Tokens:           map[string]any{"accent": "#28b4dc", "spacing": 8},
		Copy:             "Play",
		RecreationPrompt: "# Recreation\n\nRecreate the inventory screen.",
	}

	first, err := BuildProjectZIP(project)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildProjectZIP(project)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("project archive is not deterministic")
	}

	files := readZIPFiles(t, first)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	want := []string{
		"assets/play-button.png",
		"copy.txt",
		"layout.json",
		"manifest.json",
		"recreation-prompt.md",
		"source.png",
		"tokens.json",
	}
	if !slices.Equal(names, want) {
		t.Fatalf("archive files = %v", names)
	}

	asset, err := png.Decode(bytes.NewReader(files["assets/play-button.png"]))
	if err != nil {
		t.Fatal(err)
	}
	if asset.Bounds().Dx() != 3 || asset.Bounds().Dy() != 2 {
		t.Fatalf("asset size = %v", asset.Bounds().Size())
	}
	var manifest projectManifest
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != ProjectSchema || manifest.Version != ProjectSchemaVersion || len(manifest.Assets) != 1 {
		t.Fatalf("manifest = %+v", manifest)
	}
	if string(files["copy.txt"]) != project.Copy || string(files["recreation-prompt.md"]) != project.RecreationPrompt {
		t.Fatal("text project artifacts changed during export")
	}
}

func TestBuildProjectZIPRejectsInvalidLayoutParent(t *testing.T) {
	_, err := BuildProjectZIP(ExportProject{
		Source: image.NewNRGBA(image.Rect(0, 0, 4, 4)),
		Layout: LayoutDocument{Regions: []LayoutRegion{{
			ID:       "child",
			ParentID: "missing",
			Bounds:   PixelRect{Width: 1, Height: 1},
		}}},
	})
	if err == nil {
		t.Fatal("unknown layout parent was accepted")
	}
}

func TestBuildProjectZIPRejectsLayoutCycle(t *testing.T) {
	_, err := BuildProjectZIP(ExportProject{
		Source: image.NewNRGBA(image.Rect(0, 0, 4, 4)),
		Layout: LayoutDocument{Regions: []LayoutRegion{
			{ID: "a", ParentID: "b", Bounds: PixelRect{Width: 1, Height: 1}},
			{ID: "b", ParentID: "a", Bounds: PixelRect{X: 1, Width: 1, Height: 1}},
		}},
	})
	if err == nil {
		t.Fatal("cyclic layout hierarchy was accepted")
	}
}

func TestBuildProjectZIPMakesCollidingAssetNamesUnique(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	data, err := BuildProjectZIP(ExportProject{
		Source: source,
		Assets: []Crop{
			{Region: Region{ID: "one", Name: "Icon", Bounds: PixelRect{Width: 1, Height: 1}}},
			{Region: Region{ID: "two", Name: "Icon", Bounds: PixelRect{X: 1, Width: 1, Height: 1}}},
			{Region: Region{ID: "three", Name: "Icon 2", Bounds: PixelRect{X: 2, Width: 1, Height: 1}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	files := readZIPFiles(t, data)
	for _, name := range []string{"assets/icon.png", "assets/icon-2.png", "assets/icon-2-2.png"} {
		if _, exists := files[name]; !exists {
			t.Fatalf("missing unique asset filename %q", name)
		}
	}
}

func readZIPFiles(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		handle, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(handle)
		closeErr := handle.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		files[file.Name] = content
	}
	return files
}
