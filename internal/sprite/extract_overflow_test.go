package sprite

import (
	"image"
	"strings"
	"testing"
)

func TestExtractRepairsPoseOverflowAcrossExpectedSlot(t *testing.T) {
	strip := image.NewNRGBA(image.Rect(0, 0, 640, 128))
	// Slot 1: normal full-body pose.
	fillBox(strip, 50, 28, 109, 111, 200, 100, 50)
	// Slot 2: a full body with a long weapon crossing into the next nominal slot.
	fillBox(strip, 210, 28, 269, 111, 200, 100, 50)
	fillBox(strip, 270, 58, 390, 64, 200, 100, 50)
	// Slot 4: another normal pose. Slot 3 is the failure mode: the weapon overflow
	// must not become a fragment-only animation frame.
	fillBox(strip, 530, 28, 589, 111, 200, 100, 50)

	res := ExtractFrames(strip, 4, 128, 128, 8)
	if len(res.Frames) != 4 {
		t.Fatalf("修复后应输出 4 帧，got %d found=%d warnings=%v", len(res.Frames), res.Found, res.Warnings)
	}
	if res.Found != 4 {
		t.Fatalf("修复后 Found 应为 4，got %d warnings=%v", res.Found, res.Warnings)
	}
	for i, frame := range res.Frames {
		if h := maxOpaqueColHeight(frame); h < 50 {
			t.Fatalf("第 %d 帧像碎片而不是完整角色: maxOpaqueColHeight=%d", i+1, h)
		}
	}
	joined := strings.Join(res.Warnings, "\n")
	if !strings.Contains(joined, "越过") && !strings.Contains(joined, "补齐") {
		t.Fatalf("缺少越界修复提示: %v", res.Warnings)
	}
}
