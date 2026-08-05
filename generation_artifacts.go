package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"perfectpixel/internal/config"
	"perfectpixel/internal/sprite"

	xdraw "golang.org/x/image/draw"
)

const candidateThumbnailMaxDim = 192

func ensureGenerationArtifactDir(stateName, requestID string) (string, error) {
	root, err := config.GenerationArtifactsDir()
	if err != nil {
		return "", err
	}
	requestID = sanitizeName(requestID)
	if requestID == "" || requestID == "character" {
		requestID = "generation-" + galleryStamp()
	}
	dir := filepath.Join(root, requestID+"-"+sanitizeName(stateName))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func generationArtifactDir(trace GenerationTrace, stateName, requestID string) (string, error) {
	if trace.Parameters != nil {
		if dir := strings.TrimSpace(trace.Parameters["artifactDir"]); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", err
			}
			return dir, nil
		}
	}
	return ensureGenerationArtifactDir(stateName, requestID)
}

func thumbnailDataURL(img image.Image, maxDim int) (string, error) {
	if maxDim <= 0 {
		maxDim = candidateThumbnailMaxDim
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxDim && h <= maxDim {
		return pngDataURL(img)
	}
	scale := float64(maxDim) / float64(max(w, h))
	nw := max(1, int(float64(w)*scale))
	nh := max(1, int(float64(h)*scale))
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	xdraw.NearestNeighbor.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	return pngDataURL(dst)
}

func persistCandidateFrameSet(dir, prefix string, frames []*image.NRGBA, metas []sprite.FrameMeta, source, label string) ([]string, []CandidateFrame, error) {
	paths := make([]string, 0, len(frames))
	candidates := make([]CandidateFrame, 0, len(frames))
	for i, frame := range frames {
		if frame == nil {
			continue
		}
		path := filepath.Join(dir, fmt.Sprintf("%s-%03d.png", sanitizeName(prefix), i+1))
		if err := writePNG(path, frame); err != nil {
			return nil, nil, err
		}
		thumb, err := thumbnailDataURL(frame, candidateThumbnailMaxDim)
		if err != nil {
			return nil, nil, err
		}
		meta := sprite.FrameMeta{Index: i}
		if i < len(metas) {
			meta = metas[i]
		}
		paths = append(paths, path)
		candidates = append(candidates, CandidateFrame{
			ID:            fmt.Sprintf("%s-%d-%s", source, i+1, filepath.Base(dir)),
			Index:         meta.Index,
			PNG:           thumb,
			Source:        source,
			Label:         label,
			TimeMs:        meta.TimeMs,
			SourcePath:    path,
			Quality:       meta.Quality,
			Motion:        meta.Motion,
			Similarity:    meta.Similarity,
			Continuity:    meta.Continuity,
			NearDuplicate: meta.NearDuplicate,
			Fingerprint:   candidateFingerprint(frame),
			Reason:        meta.Reason,
			Selected:      meta.Selected,
		})
	}
	return paths, candidates, nil
}

func candidateFingerprint(img image.Image) string {
	if img == nil || img.Bounds().Empty() {
		return ""
	}
	bounds := img.Bounds()
	samples := [8][9]float64{}
	for y := 0; y < 8; y++ {
		y0 := bounds.Min.Y + y*bounds.Dy()/8
		y1 := bounds.Min.Y + (y+1)*bounds.Dy()/8
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < 9; x++ {
			x0 := bounds.Min.X + x*bounds.Dx()/9
			x1 := bounds.Min.X + (x+1)*bounds.Dx()/9
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var total float64
			var count int
			for sy := y0; sy < min(y1, bounds.Max.Y); sy++ {
				for sx := x0; sx < min(x1, bounds.Max.X); sx++ {
					r, g, b, a := img.At(sx, sy).RGBA()
					alpha := float64(a) / 65535
					luma := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 65535
					total += alpha * (0.72 + 0.28*luma)
					count++
				}
			}
			if count > 0 {
				samples[y][x] = total / float64(count)
			}
		}
	}
	var hash uint64
	var bit uint
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if samples[y][x] > samples[y][x+1] {
				hash |= uint64(1) << bit
			}
			bit++
		}
	}
	return fmt.Sprintf("%016x", hash)
}
