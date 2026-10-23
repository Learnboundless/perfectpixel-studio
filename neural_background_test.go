package main

import (
	"container/list"
	"context"
	"crypto/sha256"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"perfectpixel/internal/sprite"

	xdraw "golang.org/x/image/draw"
)

func TestU2NetPMaskCacheRestoresOriginalColor(t *testing.T) {
	segmenter := &u2netpSegmenter{cache: map[[32]byte]*list.Element{}, cacheLRU: list.New(), cacheMax: 2}
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 20, G: 40, B: 60, A: 255})
	src.SetNRGBA(1, 0, color.NRGBA{R: 90, G: 70, B: 50, A: 128})
	mask := image.NewNRGBA(src.Rect)
	mask.SetNRGBA(0, 0, color.NRGBA{R: 20, G: 40, B: 60, A: 200})
	mask.SetNRGBA(1, 0, color.NRGBA{R: 90, G: 70, B: 50, A: 64})
	key := sha256.Sum256([]byte("cache-key"))
	segmenter.storeMask(key, mask)
	got := segmenter.cachedMask(key, src)
	if got == nil {
		t.Fatal("cached mask was not returned")
	}
	first := got.NRGBAAt(0, 0)
	second := got.NRGBAAt(1, 0)
	if first.R != 20 || first.G != 40 || first.B != 60 || first.A != 200 {
		t.Fatalf("first cached pixel = %#v", first)
	}
	if second.R != 90 || second.G != 70 || second.B != 50 || second.A != 32 {
		t.Fatalf("second cached pixel = %#v", second)
	}
}

func TestRealU2NetPSmoke(t *testing.T) {
	inputPath := os.Getenv("MOTIONPIXEL_NEURAL_INPUT")
	if inputPath == "" {
		t.Skip("set MOTIONPIXEL_NEURAL_INPUT to run real U²-NetP smoke test")
	}
	inputs, err := neuralSmokeInputs(inputPath)
	if err != nil || len(inputs) == 0 {
		t.Fatalf("load neural smoke inputs: %v", err)
	}
	if os.Getenv("MOTIONPIXEL_NEURAL_FORCE_1024") == "1" {
		for i, src := range inputs {
			dst := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
			for offset := 0; offset < len(dst.Pix); offset += 4 {
				dst.Pix[offset] = 255
				dst.Pix[offset+2] = 255
				dst.Pix[offset+3] = 255
			}
			xdraw.CatmullRom.Scale(dst, dst.Rect, src, src.Rect, xdraw.Src, nil)
			inputs[i] = dst
		}
	}
	segmenter, err := newU2NetPSegmenter()
	if err != nil {
		t.Fatal(err)
	}
	remover := sprite.NewBackgroundRemover("hybrid", segmenter)
	started := time.Now()
	clean, trace := remover.RemoveContext(context.Background(), inputs[0])
	firstElapsed := time.Since(started)
	if clean == nil || trace.ActualMode != "hybrid" || trace.Fallback != "" {
		t.Fatalf("real neural smoke failed: %#v", trace)
	}
	transparent, visible := 0, 0
	for i := 3; i < len(clean.Pix); i += 4 {
		if clean.Pix[i] == 0 {
			transparent++
		}
		if clean.Pix[i] >= 224 {
			visible++
		}
	}
	pixels := max(1, clean.Rect.Dx()*clean.Rect.Dy())
	transparentRatio := float64(transparent) / float64(pixels)
	visibleRatio := float64(visible) / float64(pixels)
	if transparentRatio < 0.60 {
		t.Errorf("background transparency regressed: %.2f%%", transparentRatio*100)
	}
	if visibleRatio < 0.002 {
		t.Errorf("subject was nearly erased: visible %.3f%%", visibleRatio*100)
	}
	started = time.Now()
	_, cachedTrace := remover.RemoveContext(context.Background(), inputs[0])
	cachedElapsed := time.Since(started)
	batchStarted := time.Now()
	_, batchTrace, batchErr := sprite.PrepareVideoFramesWithBackgroundRemoverContext(context.Background(), inputs, len(inputs), inputs[0].Rect.Dx(), inputs[0].Rect.Dy(), 0, sprite.AlignmentPreserveSourceCanvas, sprite.FrameFitContain, remover, 2, nil)
	batchElapsed := time.Since(batchStarted)
	if batchErr != nil {
		t.Fatal(batchErr)
	}
	outputPath := os.Getenv("MOTIONPIXEL_NEURAL_OUTPUT")
	if outputPath != "" {
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			t.Fatal(err)
		}
		output, err := os.Create(outputPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(output, clean); err != nil {
			_ = output.Close()
			t.Fatal(err)
		}
		_ = output.Close()
	}
	if clean.Rect.Dx() == 1024 && clean.Rect.Dy() == 1024 && firstElapsed > 2*time.Second {
		t.Errorf("first 1024x1024 inference exceeded 2s target: %s", firstElapsed)
	}
	if cachedElapsed > time.Second {
		t.Errorf("warm inference exceeded 1s target: %s", cachedElapsed)
	}
	if len(inputs) >= 40 && batchElapsed > 30*time.Second {
		t.Errorf("40-frame batch exceeded 30s target: %s", batchElapsed)
	}
	t.Logf("U²-NetP inputs=%d size=%dx%d first=%s cached=%s batch=%s transparent=%.2f%% visible=%.2f%% model=%s sha=%s cachedTraceMs=%d batchTraceMs=%d", len(inputs), clean.Rect.Dx(), clean.Rect.Dy(), firstElapsed, cachedElapsed, batchElapsed, transparentRatio*100, visibleRatio*100, trace.Model, trace.ModelSHA256, cachedTrace.InferenceMs, batchTrace.InferenceMs)
}

func neuralSmokeInputs(path string) ([]*image.NRGBA, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	paths := []string{path}
	if info.IsDir() {
		paths, err = filepath.Glob(filepath.Join(path, "raw-candidate-*.png"))
		if err != nil {
			return nil, err
		}
		if len(paths) == 0 {
			paths, err = filepath.Glob(filepath.Join(path, "*.png"))
			if err != nil {
				return nil, err
			}
		}
		if len(paths) > 40 {
			paths = paths[:40]
		}
	}
	inputs := make([]*image.NRGBA, 0, len(paths))
	for _, input := range paths {
		file, openErr := os.Open(input)
		if openErr != nil {
			return nil, openErr
		}
		img, _, decodeErr := image.Decode(file)
		_ = file.Close()
		if decodeErr != nil {
			return nil, decodeErr
		}
		inputs = append(inputs, sprite.ToNRGBA(img))
	}
	return inputs, nil
}

type blockingSegmenter struct {
	calls int
}

func (s *blockingSegmenter) Segment(src *image.NRGBA) (*image.NRGBA, error) {
	s.calls++
	return sprite.ToNRGBA(src), nil
}

func (s *blockingSegmenter) SegmentContext(ctx context.Context, src *image.NRGBA) (*image.NRGBA, error) {
	s.calls++
	<-ctx.Done()
	return nil, ctx.Err()
}

func (*blockingSegmenter) ModelName() string   { return "fake" }
func (*blockingSegmenter) ModelSHA256() string { return "fake-sha" }

func TestBackgroundRemovalSequenceCanBeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	segmenter := &blockingSegmenter{}
	frames := []*image.NRGBA{image.NewNRGBA(image.Rect(0, 0, 8, 8)), image.NewNRGBA(image.Rect(0, 0, 8, 8))}
	done := make(chan error, 1)
	go func() {
		_, _, err := sprite.PrepareVideoFramesWithBackgroundRemoverContext(ctx, frames, 2, 8, 8, 0, sprite.AlignmentPreserveSourceCanvas, sprite.FrameFitContain, sprite.NewBackgroundRemover("neural", segmenter), 2, nil)
		done <- err
	}()
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancelled background removal returned no error")
	}
}
