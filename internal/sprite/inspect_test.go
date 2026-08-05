package sprite

import (
	"image"
	"strings"
	"testing"
)

// fillRect는 테스트용 사각형을 채웁니다.
func fillRect(img *image.NRGBA, x0, y0, x1, y1 int, r, g, b uint8) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = r, g, b, 255
		}
	}
}

func TestInspectFramesClean(t *testing.T) {
	key := [3]uint8{255, 0, 255}
	var frames []*image.NRGBA
	for i := 0; i < 4; i++ {
		f := image.NewNRGBA(image.Rect(0, 0, 128, 128))
		fillRect(f, 30, 30, 100, 110, 40, 80, 200) // 파란 캐릭터 블록
		frames = append(frames, f)
	}
	res := InspectFrames(frames, key, nil)
	if !res.Ok() {
		t.Fatalf("정상 프레임에서 오류 발생: %v", res.Errors)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("정상 프레임에서 경고 발생: %v", res.Warnings)
	}
}

func TestInspectFramesSmallConsistentSprite(t *testing.T) {
	key := [3]uint8{255, 0, 255}
	var frames []*image.NRGBA
	for i := 0; i < 4; i++ {
		f := image.NewNRGBA(image.Rect(0, 0, 128, 128))
		fillRect(f, 44, 44, 84, 84, 40, 80, 200) // 1600px, 작지만 모든 프레임이 일관됨
		frames = append(frames, f)
	}

	res := InspectFrames(frames, key, nil)
	if len(res.Warnings) != 0 {
		t.Fatalf("일관된 작은 스프라이트에서 경고 발생: %v", res.Warnings)
	}
}

func TestInspectFramesContentBelowMedian(t *testing.T) {
	key := [3]uint8{255, 0, 255}
	var frames []*image.NRGBA
	for i := 0; i < 3; i++ {
		f := image.NewNRGBA(image.Rect(0, 0, 128, 128))
		fillRect(f, 44, 44, 84, 84, 40, 80, 200) // 1600px
		frames = append(frames, f)
	}
	thin := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(thin, 52, 52, 77, 77, 40, 80, 200) // 625px, 비정상적으로 작진 않지만 누락 의심
	frames = append(frames, thin)

	res := InspectFrames(frames, key, nil)
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "明显少于其他帧") {
			found = true
		}
	}
	if !found {
		t.Fatalf("중앙값 대비 콘텐츠 부족 경고 누락: %v", res.Warnings)
	}
}

func TestInspectFramesKeyResidue(t *testing.T) {
	key := [3]uint8{255, 0, 255}
	f := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(f, 30, 30, 100, 110, 40, 80, 200)
	fillRect(f, 40, 40, 60, 60, 250, 30, 250) // 마젠타 잔여물 400px
	res := InspectFrames([]*image.NRGBA{f}, key, nil)
	if res.Ok() {
		t.Fatal("마젠타 잔여물을 감지하지 못함")
	}
	joined := strings.Join(res.RetryHints, " ")
	if !strings.Contains(joined, "magenta") {
		t.Fatalf("마젠타 보정 힌트 누락: %v", res.RetryHints)
	}
}

func TestInspectFramesEdgeAndEmpty(t *testing.T) {
	key := [3]uint8{255, 0, 255}
	// 가장자리에 닿은 프레임
	edge := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(edge, 0, 30, 70, 110, 40, 80, 200) // x=0부터 시작 → 잘림
	// 빈 프레임
	empty := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(empty, 60, 60, 65, 65, 40, 80, 200) // 25px뿐

	res := InspectFrames([]*image.NRGBA{edge, empty}, key, nil)
	if res.Ok() {
		t.Fatal("빈 프레임을 오류로 감지하지 못함")
	}
	foundEdge := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "碰到边缘") {
			foundEdge = true
		}
	}
	if !foundEdge {
		t.Fatalf("가장자리 잘림 경고 누락: %v", res.Warnings)
	}
}

func TestInspectFramesSizeOutlier(t *testing.T) {
	key := [3]uint8{255, 0, 255}
	var frames []*image.NRGBA
	for i := 0; i < 3; i++ {
		f := image.NewNRGBA(image.Rect(0, 0, 128, 128))
		fillRect(f, 20, 20, 110, 110, 40, 80, 200) // 8100px
		frames = append(frames, f)
	}
	tiny := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(tiny, 50, 50, 80, 80, 40, 80, 200) // 900px ≈ 0.11×
	frames = append(frames, tiny)

	res := InspectFrames(frames, key, nil)
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "明显比其他帧小") {
			found = true
		}
	}
	if !found {
		t.Fatalf("크기 이상치 경고 누락: %v", res.Warnings)
	}
}
