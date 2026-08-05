package uiassets

import (
	"image"
	"image/color"
	"testing"
)

func TestCreateMaskPaddingFeatherAndInvert(t *testing.T) {
	mask, err := CreateMask(Size{Width: 9, Height: 9}, []PixelRect{{X: 4, Y: 4, Width: 1, Height: 1}}, MaskOptions{Padding: 1})
	if err != nil {
		t.Fatal(err)
	}
	if mask.AlphaAt(3, 3).A != 255 || mask.AlphaAt(5, 5).A != 255 || mask.AlphaAt(2, 2).A != 0 {
		t.Fatal("padding did not produce the expected hard mask")
	}

	feathered, err := CreateMask(Size{Width: 9, Height: 9}, []PixelRect{{X: 4, Y: 4, Width: 1, Height: 1}}, MaskOptions{Feather: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := feathered.AlphaAt(4, 4).A; got == 0 || got == 255 {
		t.Fatalf("feathered center = %d", got)
	}
	if got := feathered.AlphaAt(3, 4).A; got == 0 {
		t.Fatal("feather did not extend to neighboring pixel")
	}

	inverted, err := CreateMask(Size{Width: 3, Height: 3}, []PixelRect{{X: 1, Y: 1, Width: 1, Height: 1}}, MaskOptions{Invert: true})
	if err != nil {
		t.Fatal(err)
	}
	if inverted.AlphaAt(1, 1).A != 0 || inverted.AlphaAt(0, 0).A != 255 {
		t.Fatal("inverted mask values are wrong")
	}
}

func TestCompositeSupportsReplacementAndTransparency(t *testing.T) {
	base := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	fillTestRect(base, base.Bounds(), color.NRGBA{R: 255, A: 255})
	edited := image.NewNRGBA(base.Bounds())
	edited.SetNRGBA(0, 0, color.NRGBA{B: 255, A: 255})
	edited.SetNRGBA(1, 0, color.NRGBA{})
	mask := image.NewAlpha(base.Bounds())
	mask.SetAlpha(0, 0, color.Alpha{A: 128})
	mask.SetAlpha(1, 0, color.Alpha{A: 255})

	out, err := Composite(base, edited, mask)
	if err != nil {
		t.Fatal(err)
	}
	first := out.NRGBAAt(0, 0)
	if first.R < 126 || first.R > 128 || first.B < 127 || first.B > 129 || first.A != 255 {
		t.Fatalf("partial replacement = %+v", first)
	}
	if got := out.NRGBAAt(1, 0); got.A != 0 {
		t.Fatalf("transparent replacement did not clear base: %+v", got)
	}
}

func TestCompositeRegionOnlyChangesSelectedBounds(t *testing.T) {
	base := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	fillTestRect(base, base.Bounds(), color.NRGBA{R: 20, G: 30, B: 40, A: 255})
	edit := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	fillTestRect(edit, edit.Bounds(), color.NRGBA{R: 200, A: 255})
	out, err := CompositeRegion(base, edit, PixelRect{X: 1, Y: 1, Width: 2, Height: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.NRGBAAt(1, 1).R != 200 || out.NRGBAAt(2, 1).R != 200 {
		t.Fatal("edit was not placed inside selected bounds")
	}
	if got := out.NRGBAAt(0, 0); got != (color.NRGBA{R: 20, G: 30, B: 40, A: 255}) {
		t.Fatalf("pixel outside selection changed: %+v", got)
	}
}
