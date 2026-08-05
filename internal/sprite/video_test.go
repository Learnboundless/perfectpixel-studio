package sprite

import (
	"bytes"
	"context"
	"errors"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNormalizeFramesToCellsPreservesVideoPlacement(t *testing.T) {
	f1 := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(f1, 8, 18, 38, 92, 40, 80, 200)
	f2 := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(f2, 54, 26, 84, 100, 40, 80, 200)
	f3 := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(f3, 88, 10, 118, 84, 40, 80, 200)

	res := NormalizeFramesToCells([]*image.NRGBA{f1, f2, f3}, 3, 128, 128, 12)
	if res.Found != 3 || len(res.Frames) != 3 {
		t.Fatalf("프레임 정규화 수 오류: found=%d frames=%d warnings=%v", res.Found, len(res.Frames), res.Warnings)
	}
	wants := []FrameRect{
		{X: 8, Y: 18, W: 30, H: 74},
		{X: 54, Y: 26, W: 30, H: 74},
		{X: 88, Y: 10, W: 30, H: 74},
	}
	for i, f := range res.Frames {
		if f.Rect.Dx() != 128 || f.Rect.Dy() != 128 {
			t.Fatalf("프레임 %d 셀 크기 오류: %v", i, f.Rect)
		}
		box := contentBBox(f)
		if box.W == 0 || box.H == 0 {
			t.Fatalf("프레임 %d 콘텐츠 누락", i)
		}
		if box != wants[i] {
			t.Fatalf("프레임 %d 위치가 바뀌었습니다: got=%+v want=%+v", i, box, wants[i])
		}
	}
}

func TestEncodeMP4ProcessedAnimation(t *testing.T) {
	frames := make([]*image.NRGBA, 3)
	for i := range frames {
		frames[i] = image.NewNRGBA(image.Rect(0, 0, 64, 64))
		fillRect(frames[i], 8+i*8, 12, 30+i*8, 54, 30, 100, 220)
	}
	data, err := EncodeMP4(context.Background(), frames, 12)
	if err != nil {
		t.Fatalf("processed MP4 export failed: %v", err)
	}
	if len(data) < 32 || !bytes.Contains(data[:32], []byte("ftyp")) {
		t.Fatalf("MP4 header missing: %x", data[:min(len(data), 32)])
	}
}

func TestNormalizeFramesToCellsCentersWholeVideoCanvasWhenScaling(t *testing.T) {
	f := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	fillRect(f, 10, 20, 30, 60, 40, 80, 200)

	res := NormalizeFramesToCells([]*image.NRGBA{f}, 1, 128, 128, 12)
	if res.Found != 1 || len(res.Frames) != 1 {
		t.Fatalf("프레임 정규화 수 오류: found=%d frames=%d warnings=%v", res.Found, len(res.Frames), res.Warnings)
	}
	box := contentBBox(res.Frames[0])
	want := FrameRect{X: 26, Y: 36, W: 20, H: 40}
	if box != want {
		t.Fatalf("작은 비디오 캔버스 위치 오류: got=%+v want=%+v", box, want)
	}
}

func TestNormalizeFramesToAnchoredCellsRecentersCharacter(t *testing.T) {
	f1 := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(f1, 0, 0, 128, 128, 255, 0, 255)
	fillRect(f1, 8, 18, 38, 92, 40, 80, 200)
	f2 := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(f2, 0, 0, 128, 128, 255, 0, 255)
	fillRect(f2, 88, 18, 118, 92, 40, 80, 200)

	res := PrepareVideoFramesWithAlignment([]*image.NRGBA{f1, f2}, 2, 128, 128, 12, AlignmentGameAnchorNormalize)
	if res.Found != 2 || len(res.Frames) != 2 {
		t.Fatalf("anchored normalization count mismatch: found=%d frames=%d warnings=%v", res.Found, len(res.Frames), res.Warnings)
	}
	for i, frame := range res.Frames {
		box := contentBBox(frame)
		center := box.X + box.W/2
		if center < 60 || center > 68 {
			t.Fatalf("frame %d was not recentered around the game anchor: box=%+v center=%d", i, box, center)
		}
	}
}

func TestResolveFFmpegPathFindsBundledTool(t *testing.T) {
	root := t.TempDir()
	ffmpeg := filepath.Join(root, ".tools", "ffmpeg", "ffmpeg-test-build", "bin", ffmpegExecutableName())
	if err := os.MkdirAll(filepath.Dir(ffmpeg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ffmpeg, []byte("fake ffmpeg"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveFFmpegPath("", func(string) (string, error) {
		return "", errors.New("not on path")
	}, []string{root})
	if err != nil {
		t.Fatalf("bundled ffmpeg should be resolved: %v", err)
	}
	if got != ffmpeg {
		t.Fatalf("bundled ffmpeg path mismatch: got %q want %q", got, ffmpeg)
	}
}

func TestResolveFFmpegPathPrefersEnvPath(t *testing.T) {
	root := t.TempDir()
	envPath := filepath.Join(root, "custom", ffmpegExecutableName())
	if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte("fake ffmpeg"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveFFmpegPath(`"`+envPath+`"`, func(string) (string, error) {
		return "", errors.New("not on path")
	}, nil)
	if err != nil {
		t.Fatalf("env ffmpeg should be resolved: %v", err)
	}
	if got != envPath {
		t.Fatalf("env ffmpeg path mismatch: got %q want %q", got, envPath)
	}
}

func TestExtractVideoFramesWithAvailableFFmpeg(t *testing.T) {
	ffmpeg, err := ffmpegPath()
	if err != nil {
		t.Skipf("ffmpeg is not available: %v", err)
	}
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "sample.mp4")
	cmd := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi",
		"-i", "testsrc=size=64x64:rate=6:duration=1",
		"-pix_fmt", "yuv420p",
		videoPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("test video generation failed: %v: %s", err, out)
	}
	video, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatal(err)
	}

	res, err := ExtractVideoFrames(context.Background(), video, 3, 1)
	if err != nil {
		t.Fatalf("video frame extraction failed: %v", err)
	}
	if res.Found != 3 || len(res.Frames) != 3 {
		t.Fatalf("frame count mismatch: found=%d frames=%d warnings=%v", res.Found, len(res.Frames), res.Warnings)
	}
	if len(res.Candidates) < len(res.Frames) || len(res.CandidateMeta) != len(res.Candidates) {
		t.Fatalf("candidate pool missing: selected=%d candidates=%d meta=%d", len(res.Frames), len(res.Candidates), len(res.CandidateMeta))
	}
	if len(res.FrameMeta) != len(res.Frames) {
		t.Fatalf("selected frame meta mismatch: frames=%d meta=%d", len(res.Frames), len(res.FrameMeta))
	}
	for _, meta := range res.FrameMeta {
		if !meta.Selected || meta.Reason == "" {
			t.Fatalf("selected frame meta should be traceable: %+v", meta)
		}
	}
	for i, frame := range res.Frames {
		if frame.Rect.Dx() != 64 || frame.Rect.Dy() != 64 {
			t.Fatalf("frame %d size mismatch: %v", i, frame.Rect)
		}
	}
}

func TestExtractVideoFramesUniformSamplerUsesOnlyRequestedFrames(t *testing.T) {
	ffmpeg, err := ffmpegPath()
	if err != nil {
		t.Skipf("ffmpeg is not available: %v", err)
	}
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "sample.mp4")
	cmd := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi",
		"-i", "testsrc=size=64x64:rate=12:duration=1",
		"-pix_fmt", "yuv420p",
		videoPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("test video generation failed: %v: %s", err, out)
	}
	video, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatal(err)
	}

	res, err := ExtractVideoFramesWithSampler(context.Background(), video, 4, 1, FrameSamplerUniform)
	if err != nil {
		t.Fatalf("uniform video extraction failed: %v", err)
	}
	if len(res.Candidates) != 4 || len(res.Frames) != 4 {
		t.Fatalf("uniform sampler should not build a larger pool: frames=%d candidates=%d", len(res.Frames), len(res.Candidates))
	}
}

func TestExtractVideoFramesUsesActualVideoDuration(t *testing.T) {
	ffmpeg, err := ffmpegPath()
	if err != nil {
		t.Skipf("ffmpeg is not available: %v", err)
	}
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "two-seconds.mp4")
	cmd := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi",
		"-i", "testsrc=size=64x64:rate=12:duration=2",
		"-pix_fmt", "yuv420p",
		videoPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("test video generation failed: %v: %s", err, out)
	}
	video, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatal(err)
	}

	// The requested duration is deliberately wrong. Extraction must follow the
	// media timeline so a provider returning a longer clip does not lose its
	// recovery frames.
	res, err := ExtractVideoFramesWithSampler(context.Background(), video, 4, 1, FrameSamplerUniform)
	if err != nil {
		t.Fatalf("video frame extraction failed: %v", err)
	}
	last := res.CandidateMeta[len(res.CandidateMeta)-1].TimeMs
	if last < 1400 {
		t.Fatalf("extraction used configured duration instead of media duration: last=%dms", last)
	}
}

func TestNormalizeMotionScoresUsesRobustRelativeScale(t *testing.T) {
	scores := normalizeMotionScores([]float64{0, 0.01, 0.02, 0.03, 0.04, 0.90})
	if scores[0] != 0 {
		t.Fatalf("static frame motion should remain zero: %v", scores)
	}
	if scores[3] <= scores[2] || scores[4] <= scores[3] {
		t.Fatalf("relative motion scores should preserve ordering: %v", scores)
	}
	if scores[4] < 0.04 {
		t.Fatalf("single outlier flattened useful motion scores: %v", scores)
	}
	if scores[5] != 1 {
		t.Fatalf("motion score must be clamped to one: %v", scores)
	}
}

func TestCandidatePoolSelectsCompleteActiveMotionInterval(t *testing.T) {
	const total = 40
	frames := make([]*image.NRGBA, 0, total)
	for i := 0; i < total; i++ {
		x := 18
		switch {
		case i >= 8 && i <= 18:
			x += (i - 8) * 3
		case i > 18 && i <= 28:
			x += (28 - i) * 3
		}
		frame := image.NewNRGBA(image.Rect(0, 0, 96, 64))
		fillRect(frame, 0, 0, 96, 64, 245, 8, 225)
		fillRect(frame, x, 20, x+14, 50, 16, 22, 32)
		fillRect(frame, x+3, 23, x+11, 47, 220, 228, 236)
		frames = append(frames, frame)
	}

	selected := selectVideoCandidateIndices(frames, 6, FrameSamplerCandidatePool)
	if len(selected) != 6 {
		t.Fatalf("selected frame count=%d want=6: %v", len(selected), selected)
	}
	if selected[0] < 5 || selected[0] > 9 {
		t.Fatalf("sequence should begin at anticipation near the first motion, got %v", selected)
	}
	if selected[len(selected)-1] < 27 || selected[len(selected)-1] > 31 {
		t.Fatalf("sequence should end at recovery near the last motion, got %v", selected)
	}
	peakCovered := false
	for i, idx := range selected {
		if i > 0 && idx <= selected[i-1] {
			t.Fatalf("sequence is not strictly ordered: %v", selected)
		}
		if idx >= 15 && idx <= 21 {
			peakCovered = true
		}
	}
	if !peakCovered {
		t.Fatalf("sequence missed the peak action: %v", selected)
	}
}

func TestAnalyzeSelectedSequenceReportsRepeatedPoses(t *testing.T) {
	positions := []int{10, 10, 18, 18, 26, 34}
	analysis := make([]*image.NRGBA, 0, len(positions))
	selected := make([]int, len(positions))
	for i, x := range positions {
		analysis = append(analysis, prepareVideoAnalysisFrame(motionTestFrame(x)))
		selected[i] = i
	}

	metrics := analyzeSelectedSequence(analysis, selected, 0, len(analysis)-1, false)
	if metrics.DuplicateRate < 0.35 {
		t.Fatalf("repeated poses were not reflected in duplicate rate: %+v", metrics)
	}
	if metrics.MotionCoverage >= 0.90 {
		t.Fatalf("repeated poses should reduce motion coverage: %+v", metrics)
	}
}

func TestAnalyzeSelectedSequenceRewardsStableMotion(t *testing.T) {
	positions := []int{10, 14, 18, 22, 26, 30}
	analysis := make([]*image.NRGBA, 0, len(positions))
	selected := make([]int, len(positions))
	for i, x := range positions {
		analysis = append(analysis, prepareVideoAnalysisFrame(motionTestFrame(x)))
		selected[i] = i
	}

	metrics := analyzeSelectedSequence(analysis, selected, 0, len(analysis)-1, false)
	if metrics.DuplicateRate != 0 {
		t.Fatalf("stable motion should not contain duplicate poses: %+v", metrics)
	}
	if metrics.Continuity < 0.85 {
		t.Fatalf("evenly progressing motion should have high continuity: %+v", metrics)
	}
	if metrics.MotionCoverage < 0.90 {
		t.Fatalf("stable sequence should cover the active interval: %+v", metrics)
	}
}

func TestSmartCandidateSelectionAvoidsDuplicatePoses(t *testing.T) {
	positions := []int{10, 10, 10, 14, 18, 18, 22, 26, 26, 30, 34, 38}
	frames := make([]*image.NRGBA, 0, len(positions))
	for _, x := range positions {
		frames = append(frames, motionTestFrame(x))
	}

	smart := selectVideoCandidateSequenceDetailed(frames, 6, FrameSamplerCandidatePool, false)
	uniform := selectVideoCandidateSequenceDetailed(frames, 6, FrameSamplerUniform, false)
	if len(smart.Indices) != 6 {
		t.Fatalf("smart sampler selected %d frames, want 6: %v", len(smart.Indices), smart.Indices)
	}
	if smart.Metrics.DuplicateRate >= uniform.Metrics.DuplicateRate {
		t.Fatalf("smart sampler did not improve duplicate rate: smart=%+v uniform=%+v indices=%v", smart.Metrics, uniform.Metrics, smart.Indices)
	}
	for i := 1; i < len(smart.Indices); i++ {
		if smart.Indices[i] <= smart.Indices[i-1] {
			t.Fatalf("smart sequence is not strictly ordered: %v", smart.Indices)
		}
	}
}

func TestLoopCandidatePoolChoosesClosingFrameNearFirstPose(t *testing.T) {
	frames := make([]*image.NRGBA, 0, 12)
	positions := []int{18, 22, 28, 34, 40, 46, 42, 36, 30, 24, 18, 66}
	for _, x := range positions {
		frame := image.NewNRGBA(image.Rect(0, 0, 96, 64))
		fillRect(frame, 0, 0, 96, 64, 245, 8, 225)
		fillRect(frame, x, 20, x+14, 50, 16, 22, 32)
		fillRect(frame, x+3, 23, x+11, 47, 220, 228, 236)
		frames = append(frames, frame)
	}

	selected, _ := selectVideoCandidateSequenceForLoop(frames, 4, FrameSamplerCandidatePool, true)
	if len(selected) != 4 {
		t.Fatalf("selected frame count=%d want=4: %v", len(selected), selected)
	}
	if selected[len(selected)-1] == len(frames)-1 {
		t.Fatalf("loop selector should avoid the discontinuous final candidate: %v", selected)
	}
	first := prepareVideoAnalysisFrame(frames[selected[0]])
	gap := frameDifference(first, prepareVideoAnalysisFrame(frames[selected[len(selected)-1]]))
	badGap := frameDifference(first, prepareVideoAnalysisFrame(frames[len(frames)-1]))
	if gap >= badGap {
		t.Fatalf("loop seam was not improved: selected=%v gap=%.4f bad=%.4f", selected, gap, badGap)
	}
}

func TestAlignLoopFrameAnchorsClosesEndpointDrift(t *testing.T) {
	frames := make([]*image.NRGBA, 4)
	positions := []struct{ x, y int }{{10, 16}, {18, 18}, {24, 20}, {30, 22}}
	for i, pos := range positions {
		frames[i] = image.NewNRGBA(image.Rect(0, 0, 64, 64))
		fillRect(frames[i], pos.x, pos.y, pos.x+14, pos.y+34, 40, 90, 210)
	}
	firstBefore := contentBBox(frames[0])

	dx, dy := AlignLoopFrameAnchors(frames)
	if dx >= 0 || dy >= 0 {
		t.Fatalf("expected correction toward the first frame, got dx=%d dy=%d", dx, dy)
	}
	if got := contentBBox(frames[0]); got != firstBefore {
		t.Fatalf("loop closure must not move the first frame: got=%+v want=%+v", got, firstBefore)
	}
	first := contentBBox(frames[0])
	last := contentBBox(frames[len(frames)-1])
	if first.X+first.W/2 != last.X+last.W/2 || first.Y+first.H != last.Y+last.H {
		t.Fatalf("loop endpoint anchors were not closed: first=%+v last=%+v", first, last)
	}
}

func motionTestFrame(extension int) *image.NRGBA {
	frame := image.NewNRGBA(image.Rect(0, 0, 96, 64))
	fillRect(frame, 0, 0, 96, 64, 245, 8, 225)
	fillRect(frame, 28, 18, 42, 48, 24, 30, 38)
	fillRect(frame, 31, 21, 39, 44, 220, 228, 236)
	fillRect(frame, 38, 24, 42+extension, 29, 24, 30, 38)
	fillRect(frame, 30, 48, 34, 57, 24, 30, 38)
	fillRect(frame, 36, 48, 40, 57, 24, 30, 38)
	return frame
}
