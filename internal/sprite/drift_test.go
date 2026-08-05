package sprite

import (
	"image"
	"strings"
	"testing"
)

// makeCharFrame은 지정 색 구성의 더미 캐릭터 프레임을 만듭니다.
func makeCharFrame(body, hair rgb) *image.NRGBA {
	f := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	set := func(x, y int, c rgb) {
		i := f.PixOffset(x, y)
		f.Pix[i], f.Pix[i+1], f.Pix[i+2], f.Pix[i+3] = c.r, c.g, c.b, 255
	}
	for y := 8; y < 56; y++ {
		for x := 16; x < 48; x++ {
			if y < 20 {
				set(x, y, hair)
			} else {
				set(x, y, body)
			}
		}
	}
	return f
}

func TestInspectDriftConsistent(t *testing.T) {
	body, hair := rgb{60, 120, 200}, rgb{180, 140, 40}
	frames := []*image.NRGBA{
		makeCharFrame(body, hair),
		makeCharFrame(body, hair),
		makeCharFrame(body, hair),
		makeCharFrame(body, hair),
	}
	res := InspectFrames(frames, [3]uint8{255, 0, 255}, nil)
	for _, e := range res.Errors {
		if strings.Contains(e, "颜色组成") {
			t.Errorf("consistent frames flagged as drift: %s", e)
		}
	}
	for _, rep := range res.Reports {
		if rep.PaletteSim < 0.95 {
			t.Errorf("frame %d sim=%.2f, want ~1.0", rep.Index, rep.PaletteSim)
		}
	}
}

func TestInspectDriftDetected(t *testing.T) {
	body, hair := rgb{60, 120, 200}, rgb{180, 140, 40}
	frames := []*image.NRGBA{
		makeCharFrame(body, hair),
		makeCharFrame(body, hair),
		// 캐릭터 정체성 변형: 완전히 다른 색 구성
		makeCharFrame(rgb{20, 200, 60}, rgb{240, 30, 30}),
		makeCharFrame(body, hair),
	}
	res := InspectFrames(frames, [3]uint8{255, 0, 255}, nil)
	found := false
	for _, e := range res.Errors {
		if strings.Contains(e, "第 3 帧") && strings.Contains(e, "颜色组成") {
			found = true
		}
	}
	if !found {
		t.Errorf("drifted frame not detected as error. errors=%v warnings=%v", res.Errors, res.Warnings)
	}
	hintFound := false
	for _, h := range res.RetryHints {
		if strings.Contains(h, "identity") {
			hintFound = true
		}
	}
	if !hintFound {
		t.Error("identity retry hint missing")
	}
}

// TestInspectBaseDrift는 모든 프레임이 함께 드리프트해 leave-one-out으로는
// 안 잡히는 경우를 베이스 대비 검사가 잡아내는지 검증합니다.
func TestInspectBaseDrift(t *testing.T) {
	base := makeCharFrame(rgb{60, 120, 200}, rgb{180, 140, 40})
	// 프레임들은 서로 일관되지만 베이스와 전혀 다른 색 구성
	other, otherHair := rgb{20, 200, 60}, rgb{240, 30, 30}
	frames := []*image.NRGBA{
		makeCharFrame(other, otherHair),
		makeCharFrame(other, otherHair),
		makeCharFrame(other, otherHair),
	}
	res := InspectFrames(frames, [3]uint8{255, 0, 255}, base)
	found := false
	for _, e := range res.Errors {
		if strings.Contains(e, "基础角色") {
			found = true
		}
	}
	if !found {
		t.Errorf("일괄 드리프트 미검출. errors=%v warnings=%v", res.Errors, res.Warnings)
	}

	// 베이스와 같은 캐릭터면 오류 없어야 함
	same := []*image.NRGBA{
		makeCharFrame(rgb{60, 120, 200}, rgb{180, 140, 40}),
		makeCharFrame(rgb{60, 120, 200}, rgb{180, 140, 40}),
		makeCharFrame(rgb{60, 120, 200}, rgb{180, 140, 40}),
	}
	res = InspectFrames(same, [3]uint8{255, 0, 255}, base)
	for _, e := range res.Errors {
		if strings.Contains(e, "基础角色") {
			t.Errorf("동일 캐릭터가 베이스 드리프트로 오탐: %s", e)
		}
	}

	// 투명 영역이 없는 베이스(사진 등)는 검사를 건너뛰어야 함
	opaque := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for i := 3; i < len(opaque.Pix); i += 4 {
		opaque.Pix[i] = 255 // 전체 불투명 (검정 배경)
	}
	res = InspectFrames(frames, [3]uint8{255, 0, 255}, opaque)
	for _, e := range res.Errors {
		if strings.Contains(e, "基础角色") {
			t.Errorf("불투명 베이스에서 검사가 실행됨: %s", e)
		}
	}
}
