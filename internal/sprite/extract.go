package sprite

import (
	"fmt"
	"image"
	"sort"

	xdraw "golang.org/x/image/draw"
)

const alphaThreshold = 10 // 이 값 이하의 알파는 빈(투명) 픽셀로 취급

// frameContent는 스트립 좌표계에서 추출한 한 포즈의 콘텐츠입니다.
type frameContent struct {
	img    *image.NRGBA // bbox로 자른 콘텐츠
	minX   int
	cx     float64 // 알파 가중 질량 중심 X (스트립 좌표)
	bottom int     // 베이스라인(콘텐츠 최하단 행, 스트립 좌표)
}

// extractContent는 컬럼 구간 span 안의 불투명 픽셀을 bbox로 잘라냅니다.
// 소유권 추적(연결요소) 없이 구간 내 모든 콘텐츠를 모으므로, 팔다리가 분리되어도
// 한 포즈로 안전하게 합쳐집니다.
func extractContent(strip *image.NRGBA, span colSpan, h int) frameContent {
	minX, minY, maxX, maxY := span.end, h, span.start-1, -1
	var sumWX, sumW float64
	for x := span.start; x < span.end; x++ {
		for y := 0; y < h; y++ {
			a := strip.Pix[strip.PixOffset(x, y)+3]
			if a <= alphaThreshold {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
			sumWX += float64(x) * float64(a)
			sumW += float64(a)
		}
	}
	if maxX < minX || maxY < minY {
		return frameContent{}
	}
	gw, gh := maxX-minX+1, maxY-minY+1
	dst := image.NewNRGBA(image.Rect(0, 0, gw, gh))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			si := strip.PixOffset(x, y)
			if strip.Pix[si+3] <= alphaThreshold {
				continue
			}
			di := dst.PixOffset(x-minX, y-minY)
			copy(dst.Pix[di:di+4], strip.Pix[si:si+4])
		}
	}
	cx := float64(minX+maxX+1) / 2
	if sumW > 0 {
		cx = sumWX / sumW
	}
	return frameContent{img: dst, minX: minX, cx: cx, bottom: maxY}
}

// ExtractFrames는 투명 배경 스트립에서 포즈를 투영 분할로 검출해 셀 크기 프레임으로
// 만듭니다. 모든 프레임에 공통 스케일을 적용하고, 질량 중심으로 수평 정렬하며,
// 공통 베이스라인 기준으로 수직 오프셋(점프 호 등)을 보존합니다.
func ExtractFrames(strip *image.NRGBA, expected, cellW, cellH, margin int) ExtractResult {
	return ExtractFramesWithFitMode(strip, expected, cellW, cellH, margin, FrameFitContain)
}

func ExtractFramesWithFitMode(strip *image.NRGBA, expected, cellW, cellH, margin int, fitMode string) ExtractResult {
	res := ExtractResult{Expected: expected}
	fitMode = NormalizeFrameFitMode(fitMode)
	segs, natural := segmentStrip(strip, expected)
	if len(segs) == 0 {
		res.Warnings = append(res.Warnings, "没有在图片中找到角色，请重新生成。")
		return res
	}
	h := strip.Rect.Dy()

	var fcs []frameContent
	for _, s := range segs {
		fc := extractContent(strip, s, h)
		if fc.img != nil {
			fcs = append(fcs, fc)
		}
	}
	if len(fcs) == 0 {
		res.Warnings = append(res.Warnings, "没有找到有效姿势，请重新生成。")
		return res
	}
	if repaired, warnings := repairStripOverflowContents(fcs, expected, strip.Rect.Dx()); len(warnings) > 0 {
		fcs = repaired
		res.Warnings = append(res.Warnings, warnings...)
	}

	// 공통 베이스라인 + 공유 스케일
	baseline := 0
	for _, g := range fcs {
		if g.bottom > baseline {
			baseline = g.bottom
		}
	}
	availW := cellW - margin*2
	availH := cellH - margin*2
	if availW < 8 || availH < 8 {
		availW, availH = cellW, cellH
	}
	maxBodyW, maxBodyH := 1, 1
	for _, g := range fcs {
		bw, bh := bodyExtent(g.img)
		if bw > maxBodyW {
			maxBodyW = bw
		}
		if bh > maxBodyH {
			maxBodyH = bh
		}
	}
	if fitMode == FrameFitAutoExpand {
		maxContent := max(maxBodyW, maxBodyH)
		for _, g := range fcs {
			maxContent = max(maxContent, max(g.img.Rect.Dx(), g.img.Rect.Dy()))
		}
		expanded := maxContent + margin*2
		cellW = max(cellW, expanded)
		cellH = max(cellH, expanded)
		availW = cellW - margin*2
		availH = cellH - margin*2
	}
	scale := minf(float64(availW)/float64(maxBodyW), float64(availH)/float64(maxBodyH))
	if scale > 1 {
		scale = 1
	}
	if fitMode == FrameFitCrop || fitMode == FrameFitAutoExpand {
		scale = 1
	}

	for _, g := range fcs {
		// scale은 body extent 기준으로 계산했으므로, 그 외 바운딩 박스 빈 공간은
		// 여유 공간에 맞춰 조정한다.
		boxScale := scale
		if fitMode == FrameFitContain {
			boxScale = minf(scale, minf(float64(availW)/float64(g.img.Rect.Dx()), float64(availH)/float64(g.img.Rect.Dy())))
		}
		if boxScale > 1 {
			boxScale = 1
		}
		// 세로 정렬에서는 실제 발/하체 부분(bottom)이 아니라 콘텐츠 하단 기준 사용
		sw := int(float64(g.img.Rect.Dx())*boxScale + 0.5)
		sh := int(float64(g.img.Rect.Dy())*boxScale + 0.5)
		if sw < 1 {
			sw = 1
		}
		if sh < 1 {
			sh = 1
		}
		scaled := g.img
		if sw != g.img.Rect.Dx() || sh != g.img.Rect.Dy() {
			scaled = image.NewNRGBA(image.Rect(0, 0, sw, sh))
			xdraw.CatmullRom.Scale(scaled, scaled.Rect, g.img, g.img.Rect, xdraw.Over, nil)
		}
		// 공통 baseline 보정을 위해 strip 내 콘텐츠 하단 대비 offset을 scale
		contentBaseline := int(float64(baseline-g.bottom)*boxScale + 0.5)

		cell := image.NewNRGBA(image.Rect(0, 0, cellW, cellH))
		// 질량 중심이 셀 중앙에 오도록 수평 배치 (팔다리가 한쪽으로 뻗어도
		// 면적이 큰 몸통이 지배해 프레임 간 흔들림이 적음).
		left := int(float64(cellW)/2 - (g.cx-float64(g.minX))*boxScale + 0.5)
		if fitMode != FrameFitCrop && left < 0 {
			left = 0
		}
		if fitMode != FrameFitCrop && left+sw > cellW {
			left = cellW - sw
		}
		top := cellH - margin - contentBaseline - sh
		if fitMode != FrameFitCrop && top < 0 {
			top = 0
		}
		xdraw.Copy(cell, image.Point{X: left, Y: top}, scaled, scaled.Rect, xdraw.Over, nil)
		res.Frames = append(res.Frames, cell)
	}

	res.Found = len(res.Frames)
	if res.Found != expected {
		res.Warnings = append(res.Warnings,
			fmt.Sprintf("检测到 %d 个姿势，而不是期望的 %d 个。可能有姿势重叠或缺失，建议重新生成。", res.Found, expected))
	} else if natural != expected {
		res.Warnings = append(res.Warnings,
			fmt.Sprintf("检测到的自然姿势数为 %d，已按期望的 %d 帧完成修复输出。", natural, expected))
	}
	return res
}

type frameContentMetric struct {
	mass         float64
	maxColH      int
	opaquePixels int
}

func repairStripOverflowContents(fcs []frameContent, expected, stripW int) ([]frameContent, []string) {
	if expected < 2 || stripW <= 0 || len(fcs) == 0 {
		return fcs, nil
	}
	out, merged := mergeOverflowFragments(fcs, expected, stripW)
	padded := false
	if len(out) < expected && expected-len(out) <= max(1, expected/4) {
		out, padded = padFrameContentsToExpectedSlots(out, expected, stripW)
	}
	if !merged && !padded {
		return fcs, nil
	}

	var warnings []string
	switch {
	case merged && padded:
		warnings = append(warnings, "检测到某个姿势、武器或外轮廓越过了预期帧边界，已把碎片并回相邻帧并用相邻有效姿势补齐缺失帧。建议重新生成时扩大画布或增加列间距。")
	case merged:
		warnings = append(warnings, "检测到某个姿势、武器或外轮廓越过了预期帧边界，已把碎片并回相邻帧。建议重新生成时扩大画布或增加列间距。")
	case padded:
		warnings = append(warnings, "检测到图片条带存在空列或缺失帧，已用相邻有效姿势补齐，避免碎片帧进入导出结果。建议重新生成时扩大画布或增加列间距。")
	}
	return out, warnings
}

func mergeOverflowFragments(fcs []frameContent, expected, stripW int) ([]frameContent, bool) {
	if len(fcs) < 2 {
		return fcs, false
	}
	metrics := make([]frameContentMetric, len(fcs))
	masses := make([]float64, 0, len(fcs))
	heights := make([]int, 0, len(fcs))
	for i, fc := range fcs {
		metrics[i] = measureFrameContent(fc)
		if metrics[i].mass > 0 {
			masses = append(masses, metrics[i].mass)
		}
		if metrics[i].maxColH > 0 {
			heights = append(heights, metrics[i].maxColH)
		}
	}
	medianMass := medianFloat(masses)
	medianHeight := medianInt(heights)
	if medianMass <= 0 || medianHeight <= 0 {
		return fcs, false
	}

	isFragment := make([]bool, len(fcs))
	for i, m := range metrics {
		isFragment[i] = isOverflowFragment(m, medianMass, medianHeight)
	}
	out := make([]frameContent, 0, len(fcs))
	merged := false
	slotW := max(1, stripW/expected)
	maxGap := max(8, slotW/2)
	for i := 0; i < len(fcs); i++ {
		if !isFragment[i] {
			out = append(out, fcs[i])
			continue
		}

		leftIdx := -1
		if len(out) > 0 {
			leftIdx = len(out) - 1
		}
		rightIdx := -1
		if i+1 < len(fcs) && !isFragment[i+1] {
			rightIdx = i + 1
		}
		mergeLeft := false
		mergeRight := false
		leftGap, rightGap := maxGap+1, maxGap+1
		if leftIdx >= 0 {
			leftGap = contentEdgeGap(out[leftIdx], fcs[i])
			mergeLeft = leftGap <= maxGap
		}
		if rightIdx >= 0 {
			rightGap = contentEdgeGap(fcs[i], fcs[rightIdx])
			mergeRight = rightGap <= maxGap
		}
		switch {
		case mergeLeft && (!mergeRight || leftGap <= rightGap):
			out[leftIdx] = mergeFrameContents(out[leftIdx], fcs[i])
			merged = true
		case mergeRight:
			fcs[rightIdx] = mergeFrameContents(fcs[i], fcs[rightIdx])
			merged = true
		default:
			out = append(out, fcs[i])
		}
	}
	if !merged {
		return fcs, false
	}
	return out, true
}

func measureFrameContent(fc frameContent) frameContentMetric {
	if fc.img == nil {
		return frameContentMetric{}
	}
	w, h := fc.img.Rect.Dx(), fc.img.Rect.Dy()
	colH := make([]int, w)
	var metric frameContentMetric
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			a := fc.img.Pix[fc.img.PixOffset(x, y)+3]
			if a <= alphaThreshold {
				continue
			}
			metric.mass += float64(a)
			metric.opaquePixels++
			colH[x]++
		}
		if colH[x] > metric.maxColH {
			metric.maxColH = colH[x]
		}
	}
	return metric
}

func isOverflowFragment(m frameContentMetric, medianMass float64, medianHeight int) bool {
	if m.mass <= 0 || medianMass <= 0 || medianHeight <= 0 {
		return false
	}
	if m.mass < medianMass*0.28 {
		return true
	}
	return m.mass < medianMass*0.55 && float64(m.maxColH) < float64(medianHeight)*0.45
}

func contentEdgeGap(a, b frameContent) int {
	aStart, aEnd := a.minX, a.minX+a.img.Rect.Dx()
	bStart, bEnd := b.minX, b.minX+b.img.Rect.Dx()
	if aEnd < bStart {
		return bStart - aEnd
	}
	if bEnd < aStart {
		return aStart - bEnd
	}
	return 0
}

func mergeFrameContents(a, b frameContent) frameContent {
	if a.img == nil {
		return b
	}
	if b.img == nil {
		return a
	}
	aMinX, aMinY, aMaxX, aMaxY := frameContentBounds(a)
	bMinX, bMinY, bMaxX, bMaxY := frameContentBounds(b)
	minX, minY := min(aMinX, bMinX), min(aMinY, bMinY)
	maxX, maxY := max(aMaxX, bMaxX), max(aMaxY, bMaxY)
	dst := image.NewNRGBA(image.Rect(0, 0, maxX-minX+1, maxY-minY+1))
	copyFrameContent(dst, minX, minY, a)
	copyFrameContent(dst, minX, minY, b)

	var sumWX, sumW float64
	for x := 0; x < dst.Rect.Dx(); x++ {
		globalX := minX + x
		for y := 0; y < dst.Rect.Dy(); y++ {
			alpha := dst.Pix[dst.PixOffset(x, y)+3]
			if alpha <= alphaThreshold {
				continue
			}
			sumWX += float64(globalX) * float64(alpha)
			sumW += float64(alpha)
		}
	}
	cx := float64(minX+maxX+1) / 2
	if sumW > 0 {
		cx = sumWX / sumW
	}
	return frameContent{img: dst, minX: minX, cx: cx, bottom: maxY}
}

func frameContentBounds(fc frameContent) (minX, minY, maxX, maxY int) {
	minX = fc.minX
	minY = fc.bottom - fc.img.Rect.Dy() + 1
	maxX = minX + fc.img.Rect.Dx() - 1
	maxY = fc.bottom
	return
}

func copyFrameContent(dst *image.NRGBA, originX, originY int, fc frameContent) {
	minX, minY, _, _ := frameContentBounds(fc)
	for y := 0; y < fc.img.Rect.Dy(); y++ {
		for x := 0; x < fc.img.Rect.Dx(); x++ {
			src := fc.img.PixOffset(x, y)
			if fc.img.Pix[src+3] <= alphaThreshold {
				continue
			}
			di := dst.PixOffset(minX-originX+x, minY-originY+y)
			copy(dst.Pix[di:di+4], fc.img.Pix[src:src+4])
		}
	}
}

func padFrameContentsToExpectedSlots(fcs []frameContent, expected, stripW int) ([]frameContent, bool) {
	if len(fcs) == 0 || len(fcs) >= expected || stripW <= 0 {
		return fcs, false
	}
	occupied := make([]bool, expected)
	slots := make([]frameContent, expected)
	for _, fc := range fcs {
		slot := int(fc.cx * float64(expected) / float64(stripW))
		if slot < 0 {
			slot = 0
		}
		if slot >= expected {
			slot = expected - 1
		}
		if occupied[slot] {
			slot = nearestEmptySlot(slot, occupied)
			if slot < 0 {
				return fcs, false
			}
		}
		slots[slot] = fc
		occupied[slot] = true
	}
	for i := 0; i < expected; i++ {
		if occupied[i] {
			continue
		}
		src := nearestOccupiedSlot(i, occupied)
		if src < 0 {
			return fcs, false
		}
		slots[i] = slots[src]
		occupied[i] = true
	}
	return slots, true
}

func nearestEmptySlot(slot int, occupied []bool) int {
	for d := 1; d < len(occupied); d++ {
		left := slot - d
		if left >= 0 && !occupied[left] {
			return left
		}
		right := slot + d
		if right < len(occupied) && !occupied[right] {
			return right
		}
	}
	return -1
}

func nearestOccupiedSlot(slot int, occupied []bool) int {
	for d := 1; d < len(occupied); d++ {
		left := slot - d
		if left >= 0 && occupied[left] {
			return left
		}
		right := slot + d
		if right < len(occupied) && occupied[right] {
			return right
		}
	}
	return -1
}

func medianFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	return sorted[len(sorted)/2]
}

func medianInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	return sorted[len(sorted)/2]
}

// bodyExtent는 알파 질량 80%를 포함하는 최소 크기를 "실제 바디" extent로 반환합니다.
// 길게 뻗은 팔다리 outlier가 스케일을 과대 산정하는 것을 막습니다.
func bodyExtent(img *image.NRGBA) (int, int) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w == 0 || h == 0 {
		return 1, 1
	}
	alphaX := make([]float64, w)
	alphaY := make([]float64, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := float64(img.Pix[img.PixOffset(x, y)+3])
			alphaX[x] += a
			alphaY[y] += a
		}
	}
	cutX := cumulativeExtent(alphaX, 0.80)
	cutY := cumulativeExtent(alphaY, 0.80)
	if cutX < 1 {
		cutX = 1
	}
	if cutY < 1 {
		cutY = 1
	}
	return cutX, cutY
}

// cumulativeExtent는 질량 누적 비율 massFrac를 커버하는 가장 좁은 연속 구간의 길이를 반환합니다.
func cumulativeExtent(mass []float64, massFrac float64) int {
	total := 0.0
	for _, v := range mass {
		total += v
	}
	if total == 0 {
		return 0
	}
	target := total * massFrac
	n := len(mass)
	best := n
	left := 0
	cur := 0.0
	for right := 0; right < n; right++ {
		cur += mass[right]
		for cur >= target {
			if span := right - left + 1; span < best {
				best = span
			}
			cur -= mass[left]
			left++
		}
	}
	return best
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
