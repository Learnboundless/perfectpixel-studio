package sprite

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
)

const (
	FrameSamplerUniform       = "uniform"
	FrameSamplerCandidatePool = "candidate_pool"

	AlignmentPreserveSourceCanvas = "preserve_source_canvas"
	AlignmentGameAnchorNormalize  = "game_anchor_normalize"
)

// ExtractVideoFrames samples a short video into image frames with ffmpeg.
func ExtractVideoFrames(ctx context.Context, video []byte, expected, seconds int) (ExtractResult, error) {
	return ExtractVideoFramesWithSampler(ctx, video, expected, seconds, FrameSamplerCandidatePool)
}

// ExtractVideoFramesWithSampler samples a short video into candidate frames,
// then returns a stable ordered subset for final sprite processing.
func ExtractVideoFramesWithSampler(ctx context.Context, video []byte, expected, seconds int, samplerMode string) (ExtractResult, error) {
	return ExtractVideoFramesWithSamplerForLoop(ctx, video, expected, seconds, samplerMode, false)
}

// ExtractVideoFramesWithSamplerForLoop additionally scores the wraparound seam
// when selecting frames for a looping animation.
func ExtractVideoFramesWithSamplerForLoop(ctx context.Context, video []byte, expected, seconds int, samplerMode string, loop bool) (ExtractResult, error) {
	res := ExtractResult{Expected: expected}
	if expected < 1 {
		return res, fmt.Errorf("帧数必须至少为 1")
	}
	ffmpeg, err := ffmpegPath()
	if err != nil {
		return res, err
	}
	if seconds <= 0 {
		seconds = 4
	}
	dir, err := os.MkdirTemp("", "perfectpixel-video-*")
	if err != nil {
		return res, err
	}
	defer os.RemoveAll(dir)

	input := filepath.Join(dir, "input.mp4")
	if err := os.WriteFile(input, video, 0o600); err != nil {
		return res, err
	}
	candidateCount := videoCandidateCount(expected, samplerMode)
	pattern := filepath.Join(dir, "frame-%03d.png")
	duration := probeVideoDuration(ctx, input, ffmpeg, float64(seconds))
	fps := float64(candidateCount) / duration
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", input,
		"-vf", fmt.Sprintf("fps=%.6f", fps),
		"-frames:v", fmt.Sprintf("%d", candidateCount),
		pattern,
	}
	if out, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput(); err != nil {
		return res, fmt.Errorf("视频帧抽取失败: %w%s", err, ffmpegOutputSuffix(out))
	}

	files, err := filepath.Glob(filepath.Join(dir, "frame-*.png"))
	if err != nil {
		return res, err
	}
	sort.Strings(files)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return res, err
		}
		decoded, err := decodePNGFile(data)
		if err != nil {
			return res, err
		}
		i := len(res.Candidates)
		res.Candidates = append(res.Candidates, decoded)
		res.CandidateMeta = append(res.CandidateMeta, FrameMeta{
			Index:      i,
			TimeMs:     int(math.Round(float64(i) * 1000 / fps)),
			SourcePath: path,
		})
	}
	selection := selectVideoCandidateSequenceDetailed(res.Candidates, expected, samplerMode, loop)
	selected, qualities := selection.Indices, selection.Qualities
	displayMotion := normalizeMotionScores(selection.Motion)
	for i := range res.CandidateMeta {
		if i < len(qualities) {
			res.CandidateMeta[i].Quality = qualities[i]
		}
		if i < len(selection.Motion) {
			res.CandidateMeta[i].Motion = displayMotion[i]
			res.CandidateMeta[i].Similarity = 1 - displayMotion[i]
		}
		if i < len(selection.Continuity) {
			res.CandidateMeta[i].Continuity = selection.Continuity[i]
		}
		if i < len(selection.NearDuplicate) {
			res.CandidateMeta[i].NearDuplicate = selection.NearDuplicate[i]
		}
	}
	res.Sequence = selection.Metrics
	if selection.Start >= 0 && selection.Start < len(res.CandidateMeta) {
		res.Sequence.ActiveStartMs = res.CandidateMeta[selection.Start].TimeMs
	}
	if selection.End >= 0 && selection.End < len(res.CandidateMeta) {
		res.Sequence.ActiveEndMs = res.CandidateMeta[selection.End].TimeMs
	}
	selectedSet := make(map[int]bool, len(selected))
	for order, idx := range selected {
		if idx < 0 || idx >= len(res.Candidates) {
			continue
		}
		meta := res.CandidateMeta[idx]
		meta.Selected = true
		meta.Reason = selectionReason(order, len(selected), samplerMode, meta.Quality)
		if loop && len(selected) > 1 && (order == 0 || order == len(selected)-1) {
			meta.Reason += "，循环接缝优化"
		}
		res.Frames = append(res.Frames, res.Candidates[idx])
		res.FrameMeta = append(res.FrameMeta, meta)
		selectedSet[idx] = true
	}
	for i := range res.CandidateMeta {
		if selectedSet[i] {
			res.CandidateMeta[i].Selected = true
			res.CandidateMeta[i].Reason = "已选为最终抽帧"
		} else if res.CandidateMeta[i].Reason == "" {
			res.CandidateMeta[i].Reason = "候选帧"
		}
	}
	res.Found = len(res.Frames)
	if res.Found != expected {
		res.Warnings = append(res.Warnings, fmt.Sprintf("从视频中抽取了 %d 帧，而不是期望的 %d 帧。", res.Found, expected))
	}
	if res.Found == 0 {
		return res, fmt.Errorf("无法从视频中抽取帧")
	}
	return res, nil
}

func normalizeMotionScores(motion []float64) []float64 {
	out := make([]float64, len(motion))
	if len(motion) == 0 {
		return out
	}
	positive := make([]float64, 0, len(motion))
	for _, value := range motion {
		if value > 0 {
			positive = append(positive, value)
		}
	}
	if len(positive) == 0 {
		return out
	}
	sort.Float64s(positive)
	// Use the 90th percentile so one cut or corrupt frame cannot flatten every
	// useful motion score in the candidate inspector.
	scaleIndex := int(math.Floor(float64(len(positive)-1) * 0.90))
	scale := math.Max(0.0001, positive[scaleIndex])
	for i, value := range motion {
		out[i] = clamp01(value / scale)
	}
	return out
}

func decodePNGFile(data []byte) (*image.NRGBA, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("视频帧解码失败: %w", err)
	}
	return ToNRGBA(img), nil
}

func videoCandidateCount(expected int, samplerMode string) int {
	if expected < 1 {
		return 0
	}
	if NormalizeFrameSamplerMode(samplerMode) == FrameSamplerUniform {
		return expected
	}
	multiplier := 5
	if expected > 24 {
		multiplier = 3
	}
	if expected > 48 {
		multiplier = 2
	}
	return max(expected, min(expected*multiplier, 180))
}

func NormalizeFrameSamplerMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case FrameSamplerUniform:
		return FrameSamplerUniform
	default:
		return FrameSamplerCandidatePool
	}
}

func NormalizeAlignmentMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case AlignmentGameAnchorNormalize:
		return AlignmentGameAnchorNormalize
	default:
		return AlignmentPreserveSourceCanvas
	}
}

func selectVideoCandidateIndices(frames []*image.NRGBA, expected int, samplerMode string) []int {
	selected, _ := selectVideoCandidateSequence(frames, expected, samplerMode)
	return selected
}

func selectVideoCandidateSequence(frames []*image.NRGBA, expected int, samplerMode string) ([]int, []float64) {
	return selectVideoCandidateSequenceForLoop(frames, expected, samplerMode, false)
}

func selectVideoCandidateSequenceForLoop(frames []*image.NRGBA, expected int, samplerMode string, loop bool) ([]int, []float64) {
	selection := selectVideoCandidateSequenceDetailed(frames, expected, samplerMode, loop)
	return selection.Indices, selection.Qualities
}

type videoCandidateSelection struct {
	Indices       []int
	Qualities     []float64
	Motion        []float64
	Continuity    []float64
	NearDuplicate []bool
	Metrics       FrameSequenceMetrics
	Start         int
	End           int
}

func selectVideoCandidateSequenceDetailed(frames []*image.NRGBA, expected int, samplerMode string, loop bool) videoCandidateSelection {
	n := len(frames)
	if expected <= 0 || n == 0 {
		return videoCandidateSelection{Start: -1, End: -1}
	}
	analysis := make([]*image.NRGBA, n)
	qualities := make([]float64, n)
	for i, frame := range frames {
		analysis[i] = prepareVideoAnalysisFrame(frame)
		qualities[i] = frameQualityPrepared(analysis[i])
	}
	motion := make([]float64, n)
	for i := 1; i < n; i++ {
		motion[i] = frameDifference(analysis[i-1], analysis[i])
	}
	continuity, nearDuplicate := candidateMotionDiagnostics(motion)
	if n <= expected {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return videoCandidateSelection{
			Indices:       out,
			Qualities:     qualities,
			Motion:        motion,
			Continuity:    continuity,
			NearDuplicate: nearDuplicate,
			Metrics:       analyzeSelectedSequence(analysis, out, 0, n-1, loop),
			Start:         0,
			End:           n - 1,
		}
	}
	if NormalizeFrameSamplerMode(samplerMode) == FrameSamplerUniform {
		out := evenlySpacedIndices(n, expected)
		return videoCandidateSelection{
			Indices:       out,
			Qualities:     qualities,
			Motion:        motion,
			Continuity:    continuity,
			NearDuplicate: nearDuplicate,
			Metrics:       analyzeSelectedSequence(analysis, out, 0, n-1, loop),
			Start:         0,
			End:           n - 1,
		}
	}
	start, end := activeMotionInterval(smoothMotionEnergy(motion), expected)
	selected := selectStableOrderedFrames(analysis, qualities, motion, start, end, expected)
	if loop {
		selected = optimizeLoopEndpoints(analysis, qualities, selected, start, end)
	}
	return videoCandidateSelection{
		Indices:       selected,
		Qualities:     qualities,
		Motion:        motion,
		Continuity:    continuity,
		NearDuplicate: nearDuplicate,
		Metrics:       analyzeSelectedSequence(analysis, selected, start, end, loop),
		Start:         start,
		End:           end,
	}
}

func candidateMotionDiagnostics(motion []float64) ([]float64, []bool) {
	continuity := make([]float64, len(motion))
	nearDuplicate := make([]bool, len(motion))
	if len(motion) == 0 {
		return continuity, nearDuplicate
	}
	continuity[0] = 1
	positive := make([]float64, 0, len(motion)-1)
	for _, value := range motion[1:] {
		if value > 0 {
			positive = append(positive, value)
		}
	}
	median := medianFloat(positive)
	duplicateThreshold := math.Max(0.0025, math.Min(0.018, median*0.20))
	scale := math.Max(0.012, median*1.5)
	for i := 1; i < len(motion); i++ {
		nearDuplicate[i] = motion[i] <= duplicateThreshold
		if i == 1 {
			continuity[i] = 1
			continue
		}
		continuity[i] = clamp01(math.Exp(-math.Abs(motion[i]-motion[i-1]) / scale))
	}
	return continuity, nearDuplicate
}

// selectStableOrderedFrames uses a small dynamic program over time-local
// windows. It balances temporal coverage, visual pose progression, clarity,
// and local motion while heavily penalizing duplicate poses and large jumps.
func selectStableOrderedFrames(analysis []*image.NRGBA, qualities, motion []float64, start, end, expected int) []int {
	if expected <= 0 || len(analysis) == 0 || end < start {
		return nil
	}
	start = max(0, start)
	end = min(len(analysis)-1, end)
	if end-start+1 <= expected {
		out := make([]int, 0, end-start+1)
		for i := start; i <= end; i++ {
			out = append(out, i)
		}
		return out
	}
	if expected == 1 {
		best, bestScore := start, math.Inf(-1)
		for i := start; i <= end; i++ {
			localMotion := 0.0
			if i < len(motion) {
				localMotion = motion[i]
			}
			score := 0.75*qualities[i] + 0.25*localMotion
			if score > bestScore {
				best, bestScore = i, score
			}
		}
		return []int{best}
	}

	spacing := float64(end-start) / float64(expected-1)
	radius := max(2, int(math.Ceil(spacing*0.70)))
	windows := make([][]int, expected)
	for slot := 0; slot < expected; slot++ {
		target := float64(start) + float64(slot)*spacing
		lo := max(start+slot, int(math.Floor(target))-radius)
		hi := min(end-(expected-slot-1), int(math.Ceil(target))+radius)
		if lo > hi {
			center := max(start+slot, min(int(math.Round(target)), end-(expected-slot-1)))
			lo, hi = center, center
		}
		for index := lo; index <= hi; index++ {
			windows[slot] = append(windows[slot], index)
		}
	}

	baseline := evenlySpacedIndices(end-start+1, expected)
	stepSamples := make([]float64, 0, max(0, len(baseline)-1))
	for i := 1; i < len(baseline); i++ {
		a := start + baseline[i-1]
		b := start + baseline[i]
		stepSamples = append(stepSamples, frameDifference(analysis[a], analysis[b]))
	}
	targetPoseStep := math.Max(0.012, medianFloat(stepSamples))
	duplicateThreshold := math.Max(0.004, math.Min(0.025, targetPoseStep*0.24))
	jumpThreshold := math.Max(0.22, targetPoseStep*2.8)
	motionPeak := 0.0
	for i := max(1, start); i <= end && i < len(motion); i++ {
		motionPeak = math.Max(motionPeak, motion[i])
	}
	motionPeak = math.Max(motionPeak, 0.01)

	scores := make([][]float64, expected)
	previous := make([][]int, expected)
	for slot := range windows {
		scores[slot] = make([]float64, len(windows[slot]))
		previous[slot] = make([]int, len(windows[slot]))
		for i := range scores[slot] {
			scores[slot][i] = math.Inf(-1)
			previous[slot][i] = -1
		}
		target := float64(start) + float64(slot)*spacing
		for j, index := range windows[slot] {
			targetScore := 1 - math.Min(1, math.Abs(float64(index)-target)/float64(max(radius, 1)))
			localMotion := 0.0
			if index < len(motion) {
				localMotion = clamp01(motion[index] / motionPeak)
			}
			nodeScore := 0.24*qualities[index] + 0.12*localMotion + 0.18*targetScore
			if slot == 0 {
				scores[slot][j] = nodeScore
				continue
			}
			for p, previousIndex := range windows[slot-1] {
				if previousIndex >= index || math.IsInf(scores[slot-1][p], -1) {
					continue
				}
				delta := float64(index - previousIndex)
				temporalScore := 1 - math.Min(1, math.Abs(delta-spacing)/math.Max(1, spacing))
				poseStep := frameDifference(analysis[previousIndex], analysis[index])
				poseScore := math.Exp(-math.Abs(poseStep-targetPoseStep) / (targetPoseStep + 0.02))
				penalty := 0.0
				if poseStep < duplicateThreshold {
					penalty += 0.85 * (1 - poseStep/duplicateThreshold)
				}
				if poseStep > jumpThreshold {
					penalty += 0.45 * math.Min(1, (poseStep-jumpThreshold)/(jumpThreshold+0.02))
				}
				score := scores[slot-1][p] + nodeScore + 0.22*temporalScore + 0.24*poseScore - penalty
				if score > scores[slot][j] {
					scores[slot][j] = score
					previous[slot][j] = p
				}
			}
		}
	}

	best := -1
	for i, score := range scores[expected-1] {
		if best < 0 || score > scores[expected-1][best] {
			best = i
		}
	}
	if best < 0 || math.IsInf(scores[expected-1][best], -1) {
		return selectOrderedFrames(qualities, start, end, expected)
	}
	out := make([]int, expected)
	for slot := expected - 1; slot >= 0; slot-- {
		out[slot] = windows[slot][best]
		best = previous[slot][best]
		if slot > 0 && best < 0 {
			return selectOrderedFrames(qualities, start, end, expected)
		}
	}
	return out
}

func analyzeSelectedSequence(analysis []*image.NRGBA, selected []int, start, end int, loop bool) FrameSequenceMetrics {
	metrics := FrameSequenceMetrics{}
	if len(selected) == 0 {
		return metrics
	}
	if len(selected) == 1 {
		metrics.Continuity = 1
		metrics.MotionCoverage = 1
		if loop {
			metrics.LoopClosure = 1
		}
		return metrics
	}

	steps := make([]float64, 0, len(selected)-1)
	timeSteps := make([]float64, 0, len(selected)-1)
	for i := 1; i < len(selected); i++ {
		steps = append(steps, frameDifference(analysis[selected[i-1]], analysis[selected[i]]))
		timeSteps = append(timeSteps, float64(selected[i]-selected[i-1]))
	}
	medianStep := math.Max(0.008, medianFloat(steps))
	duplicateThreshold := math.Max(0.004, math.Min(0.024, medianStep*0.25))
	duplicates := 0
	for i := 1; i < len(selected); i++ {
		isDuplicate := false
		for j := 0; j < i; j++ {
			if loop && i == len(selected)-1 && j == 0 {
				continue
			}
			if frameDifference(analysis[selected[j]], analysis[selected[i]]) <= duplicateThreshold {
				isDuplicate = true
				break
			}
		}
		if isDuplicate {
			duplicates++
		}
	}
	metrics.DuplicateRate = float64(duplicates) / float64(max(1, len(selected)-1))

	visualContinuity := 1.0
	if len(steps) > 1 {
		var total float64
		scale := medianStep + 0.025
		for i := 1; i < len(steps); i++ {
			total += math.Exp(-math.Abs(steps[i]-steps[i-1]) / scale)
		}
		visualContinuity = total / float64(len(steps)-1)
	}
	temporalContinuity := 1.0
	if len(timeSteps) > 1 {
		var total float64
		medianTime := math.Max(1, medianFloat(timeSteps))
		for _, step := range timeSteps {
			total += math.Exp(-math.Abs(step-medianTime) / medianTime)
		}
		temporalContinuity = total / float64(len(timeSteps))
	}
	metrics.Continuity = clamp01(0.72*visualContinuity + 0.28*temporalContinuity)

	activeSpan := max(1, end-start)
	selectedSpan := max(0, selected[len(selected)-1]-selected[0])
	spanCoverage := clamp01(float64(selectedSpan) / float64(activeSpan))
	distinctCoverage := 1 - metrics.DuplicateRate
	metrics.MotionCoverage = clamp01(0.48*spanCoverage + 0.52*distinctCoverage)
	if loop {
		seam := frameDifference(analysis[selected[0]], analysis[selected[len(selected)-1]])
		metrics.LoopClosure = clamp01(1 - seam/(medianStep*1.5+0.02))
	}
	return metrics
}

func optimizeLoopEndpoints(analysis []*image.NRGBA, qualities []float64, selected []int, start, end int) []int {
	if len(selected) < 2 || len(analysis) == 0 {
		return selected
	}
	out := append([]int(nil), selected...)
	lastOrder := len(out) - 1
	span := max(1, end-start)
	spacing := max(1, span/max(1, lastOrder))
	radius := max(1, spacing)

	firstLo := max(start, out[0]-radius)
	firstHi := min(end, out[0]+radius)
	lastLo := max(start, out[lastOrder]-radius)
	lastHi := min(end, out[lastOrder]+radius)
	if len(out) > 2 {
		firstHi = min(firstHi, out[1]-1)
		lastLo = max(lastLo, out[lastOrder-1]+1)
	}
	if firstLo > firstHi || lastLo > lastHi {
		return out
	}

	bestFirst, bestLast := out[0], out[lastOrder]
	bestScore := math.Inf(1)
	for first := firstLo; first <= firstHi; first++ {
		for last := lastLo; last <= lastHi; last++ {
			if first >= last {
				continue
			}
			qualityPenalty := (1 - qualities[first]) + (1 - qualities[last])
			distancePenalty := float64(absInt(first-out[0])+absInt(last-out[lastOrder])) / float64(max(1, radius*2))
			score := frameDifference(analysis[first], analysis[last]) + 0.08*qualityPenalty + 0.04*distancePenalty
			if score < bestScore {
				bestFirst, bestLast, bestScore = first, last, score
			}
		}
	}
	out[0], out[lastOrder] = bestFirst, bestLast
	return out
}

func evenlySpacedIndices(n, expected int) []int {
	if expected <= 0 || n <= 0 {
		return nil
	}
	if expected == 1 {
		return []int{0}
	}
	out := make([]int, 0, expected)
	last := n - 1
	for i := 0; i < expected; i++ {
		idx := int(float64(i)*float64(last)/float64(expected-1) + 0.5)
		if idx < 0 {
			idx = 0
		}
		if idx > last {
			idx = last
		}
		out = append(out, idx)
	}
	return out
}

func frameQuality(img *image.NRGBA) float64 {
	return frameQualityPrepared(prepareVideoAnalysisFrame(img))
}

func prepareVideoAnalysisFrame(img *image.NRGBA) *image.NRGBA {
	if img == nil || img.Rect.Empty() {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	const maxAnalysisSide = 192
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if max(w, h) <= maxAnalysisSide {
		return RemoveBackground(img)
	}
	scale := float64(maxAnalysisSide) / float64(max(w, h))
	dw := max(1, int(math.Round(float64(w)*scale)))
	dh := max(1, int(math.Round(float64(h)*scale)))
	small := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	xdraw.ApproxBiLinear.Scale(small, small.Rect, img, img.Rect, xdraw.Src, nil)
	return RemoveBackground(small)
}

func frameQualityPrepared(img *image.NRGBA) float64 {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w == 0 || h == 0 {
		return 0
	}
	box := contentBBox(img)
	if box.W == 0 || box.H == 0 {
		return 0
	}
	content := 0
	var grad float64
	step := max(1, min(w, h)/128)
	for y := 0; y < h; y += step {
		for x := 0; x < w; x += step {
			off := img.PixOffset(x, y)
			if img.Pix[off+3] <= alphaThreshold {
				continue
			}
			content++
			if x+step < w && img.Pix[img.PixOffset(x+step, y)+3] > alphaThreshold {
				grad += math.Abs(luma(img, x, y) - luma(img, x+step, y))
			}
			if y+step < h && img.Pix[img.PixOffset(x, y+step)+3] > alphaThreshold {
				grad += math.Abs(luma(img, x, y) - luma(img, x, y+step))
			}
		}
	}
	sampled := max(1, (w/step)*(h/step))
	contentRatio := float64(content) / float64(sampled)
	contentScore := 1.0 - math.Min(math.Abs(contentRatio-0.18)/0.30, 1)
	boxRatio := float64(box.W*box.H) / float64(w*h)
	boxScore := 1.0 - math.Min(math.Abs(boxRatio-0.25)/0.45, 1)
	sharpScore := math.Min(grad/float64(max(1, content))*0.025, 1)
	cropScore := 1.0
	if box.X <= 1 || box.Y <= 1 || box.X+box.W >= w-1 || box.Y+box.H >= h-1 {
		cropScore = 0.15
	}
	return clamp01(0.40*contentScore + 0.25*boxScore + 0.20*sharpScore + 0.15*cropScore)
}

func smoothMotionEnergy(motion []float64) []float64 {
	if len(motion) == 0 {
		return nil
	}
	out := make([]float64, len(motion))
	for i := range motion {
		from, to := max(1, i-1), min(len(motion)-1, i+1)
		if to < from {
			continue
		}
		for j := from; j <= to; j++ {
			out[i] += motion[j]
		}
		out[i] /= float64(to - from + 1)
	}
	return out
}

func activeMotionInterval(motion []float64, expected int) (int, int) {
	n := len(motion)
	if n == 0 {
		return 0, -1
	}
	if n <= expected || n < 4 {
		return 0, n - 1
	}
	values := append([]float64(nil), motion[1:]...)
	sort.Float64s(values)
	baseline := values[len(values)/4]
	peak := values[len(values)-1]
	if peak-baseline < 0.008 {
		return 0, n - 1
	}
	threshold := baseline + (peak-baseline)*0.18
	first, last, count := -1, -1, 0
	for i := 1; i < n; i++ {
		if motion[i] < threshold {
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
		count++
	}
	if count < 2 {
		return 0, n - 1
	}
	// Keep one candidate on either side as anticipation and recovery. The
	// selected sequence is then sampled continuously within this interval.
	start := max(0, first-1)
	end := min(n-1, last+1)
	for end-start+1 < expected {
		if start > 0 {
			start--
		}
		if end-start+1 >= expected {
			break
		}
		if end < n-1 {
			end++
		}
		if start == 0 && end == n-1 {
			break
		}
	}
	return start, end
}

func selectOrderedFrames(qualities []float64, start, end, expected int) []int {
	if expected <= 0 || len(qualities) == 0 || end < start {
		return nil
	}
	start = max(0, start)
	end = min(len(qualities)-1, end)
	if end-start+1 <= expected {
		out := make([]int, 0, end-start+1)
		for i := start; i <= end; i++ {
			out = append(out, i)
		}
		return out
	}
	if expected == 1 {
		best := start
		for i := start + 1; i <= end; i++ {
			if qualities[i] > qualities[best] {
				best = i
			}
		}
		return []int{best}
	}

	spacing := float64(end-start) / float64(expected-1)
	radius := max(1, int(math.Round(spacing*0.28)))
	out := make([]int, 0, expected)
	previous := start - 1
	for order := 0; order < expected; order++ {
		target := float64(start) + float64(order)*spacing
		center := int(math.Round(target))
		remaining := expected - order - 1
		lo := max(previous+1, center-radius)
		hi := min(end-remaining, center+radius)
		if lo > hi {
			lo = max(previous+1, min(center, end-remaining))
			hi = lo
		}
		best, bestScore := lo, math.Inf(-1)
		for i := lo; i <= hi; i++ {
			distance := math.Abs(float64(i)-target) / float64(max(radius, 1))
			score := 0.30*qualities[i] - 0.70*math.Min(distance, 1)
			if score > bestScore {
				best, bestScore = i, score
			}
		}
		out = append(out, best)
		previous = best
	}
	return out
}

func luma(img *image.NRGBA, x, y int) float64 {
	off := img.PixOffset(x, y)
	return 0.299*float64(img.Pix[off]) + 0.587*float64(img.Pix[off+1]) + 0.114*float64(img.Pix[off+2])
}

func frameDifference(a, b *image.NRGBA) float64 {
	if a == nil || b == nil || a.Rect != b.Rect {
		return 0
	}
	step := max(1, min(a.Rect.Dx(), a.Rect.Dy())/96)
	var total float64
	var n int
	for y := 0; y < a.Rect.Dy(); y += step {
		for x := 0; x < a.Rect.Dx(); x += step {
			ao, bo := a.PixOffset(x, y), b.PixOffset(x, y)
			if a.Pix[ao+3] <= alphaThreshold && b.Pix[bo+3] <= alphaThreshold {
				continue
			}
			dr := math.Abs(float64(a.Pix[ao]) - float64(b.Pix[bo]))
			dg := math.Abs(float64(a.Pix[ao+1]) - float64(b.Pix[bo+1]))
			db := math.Abs(float64(a.Pix[ao+2]) - float64(b.Pix[bo+2]))
			da := math.Abs(float64(a.Pix[ao+3]) - float64(b.Pix[bo+3]))
			total += (dr + dg + db + da) / (255 * 4)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return clamp01(total / float64(n))
}

func selectionReason(order, total int, samplerMode string, quality float64) string {
	if NormalizeFrameSamplerMode(samplerMode) == FrameSamplerUniform {
		return fmt.Sprintf("均匀抽帧 %d/%d，质量 %.2f", order+1, total, quality)
	}
	return fmt.Sprintf("完整动作序列 %d/%d，质量 %.2f", order+1, total, quality)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func ffmpegPath() (string, error) {
	return resolveFFmpegPath(os.Getenv("FFMPEG_PATH"), exec.LookPath, ffmpegSearchRoots())
}

func probeVideoDuration(ctx context.Context, input, ffmpeg string, fallback float64) float64 {
	if fallback <= 0 {
		fallback = 4
	}
	name := "ffprobe"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	probe := filepath.Join(filepath.Dir(ffmpeg), name)
	if !executableExists(probe) {
		if resolved, err := exec.LookPath(name); err == nil {
			probe = resolved
		} else {
			return fallback
		}
	}
	out, err := exec.CommandContext(ctx, probe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		input,
	).Output()
	if err != nil {
		return fallback
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || duration < 0.1 || duration > 60*60 {
		return fallback
	}
	return duration
}

func resolveFFmpegPath(envPath string, lookPath func(string) (string, error), roots []string) (string, error) {
	if p := cleanExecutablePath(envPath); executableExists(p) {
		return p, nil
	}
	if p, err := lookPath(ffmpegExecutableName()); err == nil && executableExists(p) {
		return p, nil
	}
	for _, root := range roots {
		if p := findBundledFFmpeg(root); p != "" {
			return p, nil
		}
	}
	return "", fmt.Errorf("未找到 ffmpeg，视频抽帧管线需要项目内置 ffmpeg；请运行 scripts/install-ffmpeg.ps1，或设置 FFMPEG_PATH 指向 ffmpeg 可执行文件")
}

func ffmpegSearchRoots() []string {
	var roots []string
	addRoot := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		for _, existing := range roots {
			if samePath(existing, path) {
				return
			}
		}
		roots = append(roots, path)
	}
	addRootWithParents := func(path string) {
		for i := 0; i < 6; i++ {
			path = strings.TrimSpace(path)
			if path == "" || path == "." {
				return
			}
			addRoot(path)
			parent := filepath.Dir(path)
			if samePath(parent, path) {
				return
			}
			path = parent
		}
	}
	if wd, err := os.Getwd(); err == nil {
		addRootWithParents(wd)
	}
	if exe, err := os.Executable(); err == nil {
		addRootWithParents(filepath.Dir(exe))
	}
	return roots
}

func findBundledFFmpeg(root string) string {
	if root == "" {
		return ""
	}
	name := ffmpegExecutableName()
	candidates := []string{
		filepath.Join(root, ".tools", "ffmpeg", "bin", name),
		filepath.Join(root, ".tools", "ffmpeg", name),
		filepath.Join(root, "tools", "ffmpeg", "bin", name),
		filepath.Join(root, "tools", "ffmpeg", name),
		filepath.Join(root, "frontend", "node_modules", "ffmpeg-static", name),
	}
	for _, candidate := range candidates {
		if executableExists(candidate) {
			return candidate
		}
	}
	for _, base := range []string{filepath.Join(root, ".tools", "ffmpeg"), filepath.Join(root, "tools", "ffmpeg")} {
		found := findExecutableUnder(base, name)
		if found != "" {
			return found
		}
	}
	return ""
}

func findExecutableUnder(root, name string) string {
	if !dirExists(root) {
		return ""
	}
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() {
			if path != root {
				rel, relErr := filepath.Rel(root, path)
				if relErr == nil && strings.Count(rel, string(os.PathSeparator)) > 4 {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if strings.EqualFold(filepath.Base(path), name) && executableExists(path) {
			found = path
		}
		return nil
	})
	return found
}

func ffmpegExecutableName() string {
	if runtime.GOOS == "windows" {
		return "ffmpeg.exe"
	}
	return "ffmpeg"
}

func cleanExecutablePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "\"")
	return os.ExpandEnv(path)
}

func executableExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func ffmpegOutputSuffix(out []byte) string {
	if len(out) == 0 {
		return ""
	}
	return ": " + string(out)
}

// PrepareVideoFrames removes the frame background and fits full video frames
// into fixed-size cells while preserving the original video composition.
func PrepareVideoFrames(frames []*image.NRGBA, expected, cellW, cellH, margin int) ExtractResult {
	return PrepareVideoFramesWithAlignment(frames, expected, cellW, cellH, margin, AlignmentPreserveSourceCanvas)
}

// PrepareVideoFramesWithAlignment prepares single-source frames for sprite export.
func PrepareVideoFramesWithAlignment(frames []*image.NRGBA, expected, cellW, cellH, margin int, alignmentMode string) ExtractResult {
	return PrepareVideoFramesWithAlignmentAndFit(frames, expected, cellW, cellH, margin, alignmentMode, FrameFitContain)
}

func PrepareVideoFramesWithAlignmentAndFit(frames []*image.NRGBA, expected, cellW, cellH, margin int, alignmentMode, fitMode string) ExtractResult {
	cleaned := make([]*image.NRGBA, 0, len(frames))
	for _, f := range frames {
		cleaned = append(cleaned, RemoveBackground(f))
	}
	if NormalizeAlignmentMode(alignmentMode) == AlignmentGameAnchorNormalize {
		return normalizeFramesToAnchoredCellsWithFit(cleaned, expected, cellW, cellH, margin, fitMode)
	}
	return normalizeFramesToCellsWithFit(cleaned, expected, cellW, cellH, margin, fitMode)
}

// NormalizeFramesToCells fits already-transparent frames into sprite cells
// without re-anchoring each frame by its content box. Video models already
// choose a camera frame; cropping every frame to its alpha bounds and then
// re-centering destroys that staging and causes visible position jumps.
func NormalizeFramesToCells(frames []*image.NRGBA, expected, cellW, cellH, _ int) ExtractResult {
	return normalizeFramesToCellsWithFit(frames, expected, cellW, cellH, 0, FrameFitContain)
}

func normalizeFramesToCellsWithFit(frames []*image.NRGBA, expected, cellW, cellH, _ int, fitMode string) ExtractResult {
	res := ExtractResult{Expected: expected}
	fitMode = NormalizeFrameFitMode(fitMode)
	type content struct {
		img *image.NRGBA
	}
	var items []content
	for _, f := range frames {
		box := contentBBox(f)
		if box.W == 0 || box.H == 0 {
			continue
		}
		items = append(items, content{img: f})
	}
	res.Found = len(items)
	if len(items) == 0 {
		res.Warnings = append(res.Warnings, "没有在图片中找到角色，请重新生成。")
		return res
	}
	if fitMode == FrameFitAutoExpand {
		for _, item := range items {
			cellW = max(cellW, item.img.Rect.Dx())
			cellH = max(cellH, item.img.Rect.Dy())
		}
	}

	for _, item := range items {
		srcW, srcH := item.img.Rect.Dx(), item.img.Rect.Dy()
		if srcW <= 0 || srcH <= 0 {
			continue
		}
		scale := 1.0
		if fitMode == FrameFitContain {
			scale = minf(float64(cellW)/float64(srcW), float64(cellH)/float64(srcH))
			if scale > 1 {
				scale = 1
			}
		}
		sw := max(1, int(float64(srcW)*scale+0.5))
		sh := max(1, int(float64(srcH)*scale+0.5))
		scaled := item.img
		if sw != srcW || sh != srcH {
			scaled = image.NewNRGBA(image.Rect(0, 0, sw, sh))
			xdraw.CatmullRom.Scale(scaled, scaled.Rect, item.img, item.img.Rect, xdraw.Src, nil)
		}
		cell := image.NewNRGBA(image.Rect(0, 0, cellW, cellH))
		left := (cellW - sw) / 2
		top := (cellH - sh) / 2
		xdraw.Copy(cell, image.Point{X: left, Y: top}, scaled, scaled.Rect, xdraw.Over, nil)
		res.Frames = append(res.Frames, cell)
	}
	if expected > 0 && res.Found != expected {
		res.Warnings = append(res.Warnings, fmt.Sprintf("从视频中抽取了 %d 帧，而不是期望的 %d 帧。", res.Found, expected))
	}
	return res
}

// NormalizeFramesToAnchoredCells fits transparent frames into cells using a
// shared scale and bottom/body anchor. This is useful for generated strip
// frames, but not for already-staged source videos.
func NormalizeFramesToAnchoredCells(frames []*image.NRGBA, expected, cellW, cellH, margin int) ExtractResult {
	return normalizeFramesToAnchoredCellsWithFit(frames, expected, cellW, cellH, margin, FrameFitContain)
}

func normalizeFramesToAnchoredCellsWithFit(frames []*image.NRGBA, expected, cellW, cellH, margin int, fitMode string) ExtractResult {
	res := ExtractResult{Expected: expected}
	fitMode = NormalizeFrameFitMode(fitMode)
	type item struct {
		img    *image.NRGBA
		crop   *image.NRGBA
		minX   int
		cx     float64
		bottom int
	}
	var items []item
	for _, f := range frames {
		box := contentBBox(f)
		if box.W == 0 || box.H == 0 {
			continue
		}
		crop := image.NewNRGBA(image.Rect(0, 0, box.W, box.H))
		var sumWX, sumW float64
		for y := 0; y < box.H; y++ {
			for x := 0; x < box.W; x++ {
				sx, sy := box.X+x, box.Y+y
				off := f.PixOffset(sx, sy)
				a := f.Pix[off+3]
				if a <= alphaThreshold {
					continue
				}
				dst := crop.PixOffset(x, y)
				copy(crop.Pix[dst:dst+4], f.Pix[off:off+4])
				sumWX += float64(sx) * float64(a)
				sumW += float64(a)
			}
		}
		cx := float64(box.X*2+box.W) / 2
		if sumW > 0 {
			cx = sumWX / sumW
		}
		items = append(items, item{
			img:    f,
			crop:   crop,
			minX:   box.X,
			cx:     cx,
			bottom: box.Y + box.H - 1,
		})
	}
	res.Found = len(items)
	if len(items) == 0 {
		res.Warnings = append(res.Warnings, "没有在图片中找到角色，请重新生成。")
		return res
	}
	baseline := 0
	maxBodyW, maxBodyH := 1, 1
	for _, it := range items {
		if it.bottom > baseline {
			baseline = it.bottom
		}
		bw, bh := bodyExtent(it.crop)
		if bw > maxBodyW {
			maxBodyW = bw
		}
		if bh > maxBodyH {
			maxBodyH = bh
		}
	}
	if fitMode == FrameFitAutoExpand {
		maxContent := max(maxBodyW, maxBodyH)
		for _, it := range items {
			maxContent = max(maxContent, max(it.crop.Rect.Dx(), it.crop.Rect.Dy()))
		}
		expanded := maxContent + margin*2
		cellW = max(cellW, expanded)
		cellH = max(cellH, expanded)
	}
	availW, availH := cellW-margin*2, cellH-margin*2
	if availW < 8 || availH < 8 {
		availW, availH = cellW, cellH
	}
	scale := minf(float64(availW)/float64(maxBodyW), float64(availH)/float64(maxBodyH))
	if scale > 1 {
		scale = 1
	}
	if fitMode == FrameFitCrop || fitMode == FrameFitAutoExpand {
		scale = 1
	}
	for _, it := range items {
		boxScale := scale
		if fitMode == FrameFitContain {
			boxScale = minf(scale, minf(float64(availW)/float64(it.crop.Rect.Dx()), float64(availH)/float64(it.crop.Rect.Dy())))
		}
		if boxScale > 1 {
			boxScale = 1
		}
		sw := max(1, int(float64(it.crop.Rect.Dx())*boxScale+0.5))
		sh := max(1, int(float64(it.crop.Rect.Dy())*boxScale+0.5))
		scaled := it.crop
		if sw != it.crop.Rect.Dx() || sh != it.crop.Rect.Dy() {
			scaled = image.NewNRGBA(image.Rect(0, 0, sw, sh))
			xdraw.CatmullRom.Scale(scaled, scaled.Rect, it.crop, it.crop.Rect, xdraw.Over, nil)
		}
		cell := image.NewNRGBA(image.Rect(0, 0, cellW, cellH))
		left := int(float64(cellW)/2 - (it.cx-float64(it.minX))*boxScale + 0.5)
		if fitMode != FrameFitCrop && left < 0 {
			left = 0
		}
		if fitMode != FrameFitCrop && left+sw > cellW {
			left = cellW - sw
		}
		contentBaseline := int(float64(baseline-it.bottom)*boxScale + 0.5)
		top := cellH - margin - contentBaseline - sh
		if fitMode != FrameFitCrop && top < 0 {
			top = 0
		}
		xdraw.Copy(cell, image.Point{X: left, Y: top}, scaled, scaled.Rect, xdraw.Over, nil)
		res.Frames = append(res.Frames, cell)
	}
	if expected > 0 && res.Found != expected {
		res.Warnings = append(res.Warnings, fmt.Sprintf("从视频中抽取了 %d 帧，而不是期望的 %d 帧。", res.Found, expected))
	}
	return res
}

func alphaCenterX(img *image.NRGBA) float64 {
	var sum, weight float64
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			a := float64(img.Pix[img.PixOffset(x, y)+3])
			if a <= alphaThreshold {
				continue
			}
			sum += float64(x) * a
			weight += a
		}
	}
	if weight == 0 {
		return float64(img.Rect.Dx()) / 2
	}
	return sum / weight
}

// AlignLoopFrameAnchors removes net foot-position drift across a loop. The
// endpoint correction is distributed over the sequence so the last frame
// closes on the first without introducing a single-frame position jump.
func AlignLoopFrameAnchors(frames []*image.NRGBA) (int, int) {
	if len(frames) < 2 {
		return 0, 0
	}
	firstX, firstY, firstOK := loopFootAnchor(frames[0])
	lastX, lastY, lastOK := loopFootAnchor(frames[len(frames)-1])
	if !firstOK || !lastOK {
		return 0, 0
	}
	dx := int(math.Round(firstX - lastX))
	dy := firstY - lastY
	if dx == 0 && dy == 0 {
		return 0, 0
	}
	lastIndex := len(frames) - 1
	for i := 1; i <= lastIndex; i++ {
		shiftX := int(math.Round(float64(dx) * float64(i) / float64(lastIndex)))
		shiftY := int(math.Round(float64(dy) * float64(i) / float64(lastIndex)))
		frames[i] = shiftFrameInsideCanvas(frames[i], shiftX, shiftY)
	}
	return dx, dy
}

func loopFootAnchor(img *image.NRGBA) (float64, int, bool) {
	if img == nil || img.Rect.Empty() {
		return 0, 0, false
	}
	box := contentBBox(img)
	if box.W == 0 || box.H == 0 {
		return 0, 0, false
	}
	bandTop := box.Y + box.H*3/4
	var sumX, weight float64
	for y := bandTop; y < box.Y+box.H; y++ {
		for x := box.X; x < box.X+box.W; x++ {
			a := float64(img.Pix[img.PixOffset(x, y)+3])
			if a <= alphaThreshold {
				continue
			}
			sumX += float64(x) * a
			weight += a
		}
	}
	centerX := float64(box.X*2+box.W-1) / 2
	if weight > 0 {
		centerX = sumX / weight
	}
	return centerX, box.Y + box.H, true
}

func shiftFrameInsideCanvas(src *image.NRGBA, dx, dy int) *image.NRGBA {
	if src == nil || src.Rect.Empty() || (dx == 0 && dy == 0) {
		return src
	}
	box := contentBBox(src)
	if box.W == 0 || box.H == 0 {
		return src
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dx = max(-box.X, min(dx, w-(box.X+box.W)))
	dy = max(-box.Y, min(dy, h-(box.Y+box.H)))
	if dx == 0 && dy == 0 {
		return src
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.Copy(dst, image.Point{X: dx, Y: dy}, src, src.Rect, xdraw.Src, nil)
	return dst
}
