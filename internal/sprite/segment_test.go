package sprite

import (
	"image"
	"testing"
)

// TestSegmentCleanGutters는 깨끗한 골이 있는 스트립에서 자연 포즈 수와 분할이
// 정확한지 검증합니다 (투영 골 분할 경로).
func TestSegmentCleanGutters(t *testing.T) {
	strip := image.NewNRGBA(image.Rect(0, 0, 600, 100))
	fillBox(strip, 20, 20, 79, 79, 200, 100, 50)
	fillBox(strip, 220, 20, 279, 79, 200, 100, 50)
	fillBox(strip, 420, 20, 479, 79, 200, 100, 50)

	segs, natural := segmentStrip(strip, 3)
	if natural != 3 {
		t.Fatalf("자연 포즈 수 오류: %d", natural)
	}
	if len(segs) != 3 {
		t.Fatalf("세그먼트 수 오류: %d", len(segs))
	}
}

// TestSegmentTouchingDP는 두 포즈가 얇은 부위로 닿아 한 런이 됐을 때, prominence
// 봉우리 검출 + DP 최소절단이 두 토르소를 정확히 2개로 분리하는지 검증합니다
// (연결요소 방식이 실패하는, 붙어버린 포즈 케이스).
func TestSegmentTouchingDP(t *testing.T) {
	strip := image.NewNRGBA(image.Rect(0, 0, 400, 100))
	// 토르소 A(키 큼) ── 얇은 팔(낮은 질량)로 연결 ── 토르소 B(키 큼): 하나의 런이지만
	// 사이 골이 깊어 두 봉우리로 갈린다.
	fillBox(strip, 40, 10, 110, 89, 200, 100, 50)  // 토르소 A
	fillBox(strip, 110, 48, 250, 55, 200, 100, 50) // 얇은 연결부
	fillBox(strip, 250, 10, 330, 89, 200, 100, 50) // 토르소 B

	res := ExtractFrames(strip, 2, 100, 100, 8)
	if len(res.Frames) != 2 {
		t.Fatalf("닿은 포즈 분리 실패: 프레임 %d개 (기대 2)", len(res.Frames))
	}
	// 두 프레임 모두 실질 콘텐츠가 있어야 함
	for i, f := range res.Frames {
		opaque := 0
		for p := 3; p < len(f.Pix); p += 4 {
			if f.Pix[p] > alphaThreshold {
				opaque++
			}
		}
		if opaque < 500 {
			t.Fatalf("프레임 %d 콘텐츠 부족: %d", i+1, opaque)
		}
	}
}

// TestChromaMatteYCbCr는 마젠타 배경 위 녹색 사각형이 색차 평면 매팅으로
// 올바르게 분리되는지(녹색 불투명, 마젠타 투명) 검증합니다.
func TestChromaMatteYCbCr(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for i := 0; i+3 < len(src.Pix); i += 4 {
		src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = 255, 0, 255, 255 // 마젠타
	}
	fillBox(src, 20, 20, 43, 43, 40, 200, 60) // 녹색 사각형

	out := RemoveBackground(src)
	// 중앙(녹색)은 불투명
	ci := out.PixOffset(32, 32)
	if out.Pix[ci+3] < 200 {
		t.Fatalf("녹색 피사체가 투명 처리됨: alpha=%d", out.Pix[ci+3])
	}
	// 모서리(마젠타)는 투명
	ei := out.PixOffset(2, 2)
	if out.Pix[ei+3] > alphaThreshold {
		t.Fatalf("마젠타 배경이 남음: alpha=%d", out.Pix[ei+3])
	}
}

func TestChromaMatteKeepsNeutralSpriteOnDarkMagenta(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	fillBox(src, 0, 0, 95, 95, 145, 40, 88)     // dark generated magenta
	fillBox(src, 34, 22, 62, 72, 225, 232, 238) // white armor
	fillBox(src, 26, 28, 34, 78, 24, 30, 38)    // dark outline / weapon
	fillBox(src, 40, 30, 56, 52, 25, 85, 170)   // blue torso
	fillBox(src, 58, 28, 76, 70, 170, 34, 42)   // red shield
	fillBox(src, 44, 52, 52, 64, 235, 175, 20)  // yellow vent

	out := RemoveBackground(src)
	for _, p := range []image.Point{
		{42, 24}, // white
		{30, 40}, // black
		{48, 38}, // blue
		{66, 44}, // red
		{48, 58}, // yellow
	} {
		if a := out.Pix[out.PixOffset(p.X, p.Y)+3]; a <= alphaThreshold {
			t.Fatalf("sprite color at %+v was eaten by dark-magenta key: alpha=%d", p, a)
		}
	}
	if a := out.Pix[out.PixOffset(3, 3)+3]; a > alphaThreshold {
		t.Fatalf("dark magenta background was not removed: alpha=%d", a)
	}
}

func TestChromaMatteDespillsConnectedForegroundOnGradientMagenta(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 160, 160))
	for y := 0; y < 160; y++ {
		for x := 0; x < 160; x++ {
			i := src.PixOffset(x, y)
			// Video models rarely keep the requested key perfectly flat. This
			// approximates the bright center and darker/compressed border seen in
			// real generated clips.
			v := uint8((x + y) % 22)
			src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = 235+v/2, 8+v, 215+v, 255
		}
	}

	// Neutral armor contaminated by magenta environment light. The dark,
	// blue and red regions are legitimate sprite colors and must survive.
	fillBox(src, 46, 26, 119, 136, 18, 24, 34)
	fillBox(src, 50, 30, 115, 132, 214, 104, 205)
	fillBox(src, 70, 56, 91, 92, 24, 82, 168)
	fillBox(src, 95, 45, 119, 118, 178, 34, 40)

	out := RemoveBackground(src)
	if a := out.Pix[out.PixOffset(4, 4)+3]; a > alphaThreshold {
		t.Fatalf("gradient magenta background was not removed: alpha=%d", a)
	}

	armor := out.PixOffset(60, 60)
	if a := out.Pix[armor+3]; a <= alphaThreshold {
		t.Fatalf("magenta-lit armor was removed: alpha=%d", a)
	}
	// Despill should make the neutral armor substantially less magenta while
	// preserving its brightness. Before the fix R+B-G*2 is about 211.
	magentaExcess := int(out.Pix[armor]) + int(out.Pix[armor+2]) - 2*int(out.Pix[armor+1])
	if magentaExcess > 80 {
		t.Fatalf("magenta spill remains on neutral armor: rgb=(%d,%d,%d), excess=%d",
			out.Pix[armor], out.Pix[armor+1], out.Pix[armor+2], magentaExcess)
	}
	if out.Pix[armor+1] < 175 {
		t.Fatalf("despill made bright neutral armor too dark: rgb=(%d,%d,%d)",
			out.Pix[armor], out.Pix[armor+1], out.Pix[armor+2])
	}

	for _, p := range []image.Point{{80, 70}, {106, 70}, {48, 80}} {
		if a := out.Pix[out.PixOffset(p.X, p.Y)+3]; a <= alphaThreshold {
			t.Fatalf("legitimate sprite color at %+v was removed: alpha=%d", p, a)
		}
	}
}

func TestChromaOnlyClearsEdgeConnectedBackground(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillBox(src, 0, 0, 127, 127, 255, 0, 255)
	// A closed dark outline surrounds a large region that happens to use the
	// same color as the key. Full-frame color keying deletes it; an exterior
	// flood must preserve it because it is not connected to the canvas edge.
	fillBox(src, 3, 3, 124, 124, 18, 24, 34)
	fillBox(src, 7, 7, 120, 120, 255, 0, 255)

	out := RemoveBackground(src)
	if a := out.Pix[out.PixOffset(1, 1)+3]; a > alphaThreshold {
		t.Fatalf("edge-connected background was not cleared: alpha=%d", a)
	}
	if a := out.Pix[out.PixOffset(64, 64)+3]; a <= alphaThreshold {
		t.Fatalf("enclosed subject color was deleted: alpha=%d", a)
	}
}

func TestWhiteBackgroundPreservesEnclosedWhiteArmor(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	fillBox(src, 0, 0, 95, 95, 248, 248, 248)
	fillBox(src, 20, 12, 75, 83, 18, 24, 34)
	fillBox(src, 24, 16, 71, 79, 248, 248, 248)
	fillBox(src, 38, 32, 57, 65, 35, 92, 180)

	out := RemoveBackground(src)
	if a := out.Pix[out.PixOffset(2, 2)+3]; a > alphaThreshold {
		t.Fatalf("white exterior background was not cleared: alpha=%d", a)
	}
	if a := out.Pix[out.PixOffset(28, 24)+3]; a <= alphaThreshold {
		t.Fatalf("enclosed white armor was deleted: alpha=%d", a)
	}
	if a := out.Pix[out.PixOffset(48, 48)+3]; a <= alphaThreshold {
		t.Fatalf("colored subject detail was deleted: alpha=%d", a)
	}
}
