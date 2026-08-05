package sprite

import (
	"image"
	"testing"
)

// makeBlocky는 scale 크기 블록으로 구성된 진짜 픽셀아트 시뮬레이션 이미지를 만듭니다.
func makeBlocky(w, h, scale int, colors []rgb) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for by := 0; by < h; by += scale {
		for bx := 0; bx < w; bx += scale {
			c := colors[((by/scale)*7+(bx/scale)*3)%len(colors)]
			for dy := 0; dy < scale && by+dy < h; dy++ {
				for dx := 0; dx < scale && bx+dx < w; dx++ {
					i := img.PixOffset(bx+dx, by+dy)
					img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.r, c.g, c.b, 255
				}
			}
		}
	}
	return img
}

func TestDetectPixelScale(t *testing.T) {
	colors := []rgb{{200, 40, 40}, {40, 200, 40}, {40, 40, 200}, {220, 220, 60}}
	for _, scale := range []int{4, 8, 16} {
		img := makeBlocky(128, 128, scale, colors)
		got := DetectPixelScale(img)
		if got != scale {
			t.Errorf("scale %d: got %d", scale, got)
		}
	}
}

func TestDetectPixelScaleNative(t *testing.T) {
	// 1px 단위 노이즈 이미지 → 스케일 감지 불가 (1 반환)
	colors := []rgb{{10, 20, 30}, {200, 100, 50}, {90, 180, 210}, {250, 250, 250}, {120, 60, 200}}
	img := makeBlocky(64, 64, 1, colors)
	if got := DetectPixelScale(img); got != 1 {
		t.Errorf("native image: got %d, want 1", got)
	}
}

func TestDetectPixelScaleIgnoresTransparentCanvas(t *testing.T) {
	colors := []rgb{{230, 230, 240}, {40, 90, 170}, {210, 40, 40}, {20, 25, 35}}
	sprite := makeBlocky(64, 64, 8, colors)
	img := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			src := sprite.PixOffset(x, y)
			dst := img.PixOffset(224+x, 224+y)
			copy(img.Pix[dst:dst+4], sprite.Pix[src:src+4])
		}
	}
	if got := DetectPixelScale(img); got != 8 {
		t.Fatalf("transparent canvas should not dominate scale detection: got %d want 8", got)
	}

	before := opaquePixelCount(img)
	frames := []*image.NRGBA{img}
	PixelPostProcess(frames, 8)
	after := opaquePixelCount(frames[0])
	if after < before/2 {
		t.Fatalf("pixel post-process erased the sprite on a transparent canvas: before=%d after=%d", before, after)
	}
}

func TestPixelPostProcessClampsOversizedRunsForSparseSprite(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	// Long flat runs inside a small sprite can be 20px+ wide, but they are
	// color regions, not the sprite's true pixel grid.
	fillRect(img, 212, 168, 236, 348, 40, 90, 170)
	fillRect(img, 236, 168, 260, 348, 230, 230, 240)
	fillRect(img, 260, 168, 284, 348, 40, 90, 170)
	fillRect(img, 228, 340, 268, 356, 210, 40, 40)
	fillRect(img, 230, 150, 266, 180, 20, 25, 35)

	rawScale := DetectPixelScale(img)
	clamped := clampPixelScaleToContent(img, rawScale)
	if rawScale < 16 {
		t.Fatalf("test fixture should expose an oversized raw scale, got %d", rawScale)
	}
	if clamped >= rawScale || clamped > 8 {
		t.Fatalf("oversized sparse-sprite scale was not clamped: raw=%d clamped=%d", rawScale, clamped)
	}
	before := opaquePixelCount(img)
	frames := []*image.NRGBA{img}
	PixelPostProcess(frames, 8)
	after := opaquePixelCount(frames[0])
	if after < before/2 {
		t.Fatalf("clamped pixel post-process erased sparse sprite: before=%d after=%d rawScale=%d clamped=%d", before, after, rawScale, clamped)
	}
}

func TestPixelizeSnapsGrid(t *testing.T) {
	colors := []rgb{{200, 40, 40}, {40, 200, 40}}
	img := makeBlocky(64, 64, 8, colors)
	// AA 노이즈 주입: 블록 경계에 중간색 픽셀
	for y := 0; y < 64; y++ {
		i := img.PixOffset(7, y)
		img.Pix[i], img.Pix[i+1], img.Pix[i+2] = 120, 120, 40
	}
	out := Pixelize(img, 8)
	// 모든 8x8 블록이 단일 색이어야 함
	for by := 0; by < 64; by += 8 {
		for bx := 0; bx < 64; bx += 8 {
			first := out.PixOffset(bx, by)
			fr, fg, fb := out.Pix[first], out.Pix[first+1], out.Pix[first+2]
			for dy := 0; dy < 8; dy++ {
				for dx := 0; dx < 8; dx++ {
					i := out.PixOffset(bx+dx, by+dy)
					if out.Pix[i] != fr || out.Pix[i+1] != fg || out.Pix[i+2] != fb {
						t.Fatalf("block (%d,%d) not uniform", bx, by)
					}
				}
			}
		}
	}
}

func TestBuildSharedPaletteAndApply(t *testing.T) {
	// 두 프레임에 비슷하지만 다른 색 → 공유 팔레트 적용 후 동일 색으로 수렴
	f1 := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	f2 := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	fill := func(img *image.NRGBA, c rgb) {
		for i := 0; i+3 < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.r, c.g, c.b, 255
		}
	}
	fill(f1, rgb{200, 50, 50})
	fill(f2, rgb{204, 54, 54}) // 프레임 간 미세 drift
	frames := []*image.NRGBA{f1, f2}
	pal := BuildSharedPalette(frames, 4)
	if len(pal) == 0 {
		t.Fatal("empty palette")
	}
	ApplyPalette(f1, pal)
	ApplyPalette(f2, pal)
	if f1.Pix[0] != f2.Pix[0] || f1.Pix[1] != f2.Pix[1] || f1.Pix[2] != f2.Pix[2] {
		t.Errorf("frames did not converge: %v vs %v", f1.Pix[:4], f2.Pix[:4])
	}
}

func TestApplyPaletteBinarizesAlpha(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 1))
	set := func(x int, a uint8) {
		i := img.PixOffset(x, 0)
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 100, 100, 100, a
	}
	set(0, 30)
	set(1, 127)
	set(2, 128)
	set(3, 255)
	ApplyPalette(img, []rgb{{100, 100, 100}})
	for x, want := range []uint8{0, 0, 255, 255} {
		if got := img.Pix[img.PixOffset(x, 0)+3]; got != want {
			t.Errorf("x=%d alpha=%d want %d", x, got, want)
		}
	}
}

func TestPaletteSizeForStyle(t *testing.T) {
	if PaletteSizeForStyle("retro16") != 16 || PaletteSizeForStyle("pixel") != 32 {
		t.Error("pixel styles must enable post-processing")
	}
	if PaletteSizeForStyle("cartoon") != 0 || PaletteSizeForStyle("chibi") != 0 || PaletteSizeForStyle("") != 0 {
		t.Error("non-pixel styles must disable post-processing")
	}
}

func opaquePixelCount(img *image.NRGBA) int {
	n := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] > alphaThreshold {
			n++
		}
	}
	return n
}
