package sprite

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"sort"
)

// EncodeGIF는 프레임들을 투명 배경 애니메이션 GIF로 인코딩합니다.
func EncodeGIF(frames []*image.NRGBA, fps int, loop bool) ([]byte, error) {
	if fps < 1 {
		fps = 8
	}
	delays := gifFrameDelays(len(frames), fps)

	pal := buildPalette(frames)
	out := &gif.GIF{}
	if loop {
		out.LoopCount = 0 // 무한 반복
	} else {
		out.LoopCount = -1 // 1회 재생
	}

	// 팔레트 인덱스 캐시 (양자화 버킷 → 인덱스)
	cache := map[uint32]uint8{}
	nearest := func(r, g, b uint8) uint8 {
		key := uint32(r>>3)<<10 | uint32(g>>3)<<5 | uint32(b>>3)
		if idx, ok := cache[key]; ok {
			return idx
		}
		best, bestDist := 1, 1<<30
		for i := 1; i < len(pal); i++ {
			pr, pg, pb, _ := pal[i].RGBA()
			dr := int(pr>>8) - int(r)
			dg := int(pg>>8) - int(g)
			db := int(pb>>8) - int(b)
			d := dr*dr + dg*dg + db*db
			if d < bestDist {
				best, bestDist = i, d
			}
		}
		cache[key] = uint8(best)
		return uint8(best)
	}

	for frameIndex, frame := range frames {
		w, h := frame.Rect.Dx(), frame.Rect.Dy()
		p := image.NewPaletted(image.Rect(0, 0, w, h), pal)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := frame.PixOffset(x, y)
				r, g, b, a := frame.Pix[i], frame.Pix[i+1], frame.Pix[i+2], frame.Pix[i+3]
				if a < 128 {
					p.SetColorIndex(x, y, 0) // 투명 인덱스
					continue
				}
				p.SetColorIndex(x, y, nearest(r, g, b))
			}
		}
		out.Image = append(out.Image, p)
		out.Delay = append(out.Delay, delays[frameIndex])
		out.Disposal = append(out.Disposal, gif.DisposalBackground)
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// GIF stores frame delays as integer centiseconds. Distributing rounding error
// across the sequence preserves the configured average FPS instead of making
// every frame slightly too short.
func gifFrameDelays(frameCount, fps int) []int {
	if frameCount <= 0 {
		return nil
	}
	if fps < 1 {
		fps = 8
	}
	out := make([]int, frameCount)
	previous := 0
	for i := 0; i < frameCount; i++ {
		target := ((i+1)*100 + fps/2) / fps
		delay := target - previous
		if delay < 1 {
			delay = 1
		}
		out[i] = delay
		previous += delay
	}
	return out
}

// buildPalette는 프레임 전체에서 최빈 색 255개 + 투명 1개의 팔레트를 만듭니다.
func buildPalette(frames []*image.NRGBA) color.Palette {
	type bucket struct {
		count   int
		r, g, b int
	}
	buckets := map[uint32]*bucket{}
	for _, frame := range frames {
		for i := 0; i < len(frame.Pix); i += 4 {
			if frame.Pix[i+3] < 128 {
				continue
			}
			r, g, b := frame.Pix[i], frame.Pix[i+1], frame.Pix[i+2]
			key := uint32(r>>3)<<10 | uint32(g>>3)<<5 | uint32(b>>3)
			bk := buckets[key]
			if bk == nil {
				bk = &bucket{}
				buckets[key] = bk
			}
			bk.count++
			bk.r += int(r)
			bk.g += int(g)
			bk.b += int(b)
		}
	}
	type entry struct {
		count   int
		r, g, b uint8
	}
	entries := make([]entry, 0, len(buckets))
	for _, bk := range buckets {
		entries = append(entries, entry{
			count: bk.count,
			r:     uint8(bk.r / bk.count),
			g:     uint8(bk.g / bk.count),
			b:     uint8(bk.b / bk.count),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].count > entries[j].count })
	if len(entries) > 255 {
		entries = entries[:255]
	}

	pal := make(color.Palette, 0, len(entries)+1)
	pal = append(pal, color.NRGBA{0, 0, 0, 0}) // index 0 = 투명
	for _, e := range entries {
		pal = append(pal, color.NRGBA{e.r, e.g, e.b, 255})
	}
	if len(pal) == 1 {
		pal = append(pal, color.NRGBA{0, 0, 0, 255})
	}
	return pal
}
