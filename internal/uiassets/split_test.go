package uiassets

import (
	"image"
	"image/color"
	"testing"
)

func TestDetectRegionsOnAlphaBackgroundIsStable(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 12, 10))
	fillTestRect(img, image.Rect(1, 1, 4, 4), color.NRGBA{R: 220, A: 255})
	fillTestRect(img, image.Rect(7, 4, 11, 9), color.NRGBA{B: 220, A: 255})

	regions, err := DetectRegions(img, DetectionOptions{
		BackgroundMode: BackgroundAlpha,
		MinArea:        1,
		Connectivity:   4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 2 {
		t.Fatalf("region count = %d", len(regions))
	}
	if regions[0].ID != "component-001" || regions[0].Bounds != (PixelRect{X: 1, Y: 1, Width: 3, Height: 3}) || regions[0].PixelCount != 9 {
		t.Fatalf("first region = %+v", regions[0])
	}
	if regions[1].ID != "component-002" || regions[1].Bounds != (PixelRect{X: 7, Y: 4, Width: 4, Height: 5}) || regions[1].PixelCount != 20 {
		t.Fatalf("second region = %+v", regions[1])
	}
}

func TestAutoSplitSamplesSolidBackgroundAndPads(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 14, 9))
	fillTestRect(img, img.Bounds(), color.NRGBA{R: 248, G: 248, B: 248, A: 255})
	fillTestRect(img, image.Rect(2, 2, 5, 6), color.NRGBA{R: 220, G: 40, B: 40, A: 255})
	fillTestRect(img, image.Rect(9, 1, 12, 4), color.NRGBA{R: 30, G: 80, B: 220, A: 255})

	crops, err := Split(img, SplitOptions{
		Mode: SplitAuto,
		Detection: DetectionOptions{
			BackgroundMode: BackgroundAuto,
			ColorTolerance: 12,
			MinArea:        2,
			Connectivity:   8,
		},
		Trim:    true,
		Padding: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(crops) != 2 {
		t.Fatalf("crop count = %d", len(crops))
	}
	if crops[0].Region.ContentBounds != (PixelRect{X: 9, Y: 1, Width: 3, Height: 3}) || crops[0].Region.Bounds != (PixelRect{X: 8, Y: 0, Width: 5, Height: 5}) {
		t.Fatalf("first crop = %+v", crops[0].Region)
	}
	if crops[1].Region.ContentBounds != (PixelRect{X: 2, Y: 2, Width: 3, Height: 4}) || crops[1].Region.Bounds != (PixelRect{X: 1, Y: 1, Width: 5, Height: 6}) {
		t.Fatalf("second crop = %+v", crops[1].Region)
	}
}

func TestGridSplitTrimsCellsAndOmitsEmptyCells(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 12, 6))
	fillTestRect(img, image.Rect(2, 2, 4, 5), color.NRGBA{G: 180, A: 255})

	crops, err := Split(img, SplitOptions{
		Mode: SplitGrid,
		Grid: GridOptions{Rows: 1, Columns: 2},
		Detection: DetectionOptions{
			BackgroundMode: BackgroundAlpha,
			MinArea:        1,
		},
		Trim:    true,
		Padding: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(crops) != 1 {
		t.Fatalf("crop count = %d", len(crops))
	}
	if crops[0].Region.ID != "grid-r01-c01" || crops[0].Region.Bounds != (PixelRect{X: 1, Y: 1, Width: 4, Height: 5}) {
		t.Fatalf("crop = %+v", crops[0].Region)
	}
	if crops[0].Image.Bounds().Dx() != 4 || crops[0].Image.Bounds().Dy() != 5 {
		t.Fatalf("crop image size = %v", crops[0].Image.Bounds().Size())
	}
}

func TestManualSplitSortsBeforeAssigningIDs(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	crops, err := Split(img, SplitOptions{
		Mode: SplitManual,
		Manual: []Region{
			{Bounds: PixelRect{X: 6, Y: 6, Width: 2, Height: 2}},
			{Bounds: PixelRect{X: 1, Y: 1, Width: 3, Height: 2}},
		},
		Detection: DetectionOptions{BackgroundMode: BackgroundAlpha},
	})
	if err != nil {
		t.Fatal(err)
	}
	if crops[0].Region.ID != "manual-001" || crops[0].Region.Bounds.X != 1 {
		t.Fatalf("first crop = %+v", crops[0].Region)
	}
	if crops[1].Region.ID != "manual-002" || crops[1].Region.Bounds.X != 6 {
		t.Fatalf("second crop = %+v", crops[1].Region)
	}
}

func fillTestRect(img *image.NRGBA, rect image.Rectangle, value color.NRGBA) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.SetNRGBA(x, y, value)
		}
	}
}
