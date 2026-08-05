package sprite

import (
	"image"
	"testing"
)

func TestExtractFramesAutoExpandUsesLargestPoseForCell(t *testing.T) {
	strip := image.NewNRGBA(image.Rect(0, 0, 220, 120))
	fillBox(strip, 20, 20, 189, 99, 200, 100, 50)

	expanded := ExtractFramesWithFitMode(strip, 1, 64, 64, 8, FrameFitAutoExpand)
	if len(expanded.Frames) != 1 {
		t.Fatalf("自动扩格帧数=%d", len(expanded.Frames))
	}
	if got := expanded.Frames[0].Rect.Dx(); got < 186 {
		t.Fatalf("自动扩格尺寸=%d，未容纳 170px 姿势和留白", got)
	}

	cropped := ExtractFramesWithFitMode(strip, 1, 64, 64, 8, FrameFitCrop)
	if len(cropped.Frames) != 1 || cropped.Frames[0].Rect.Dx() != 64 {
		t.Fatalf("固定裁剪应保持 64px 单元: %+v", cropped.Frames)
	}
}
