package sprite

import (
	"context"
	"image"
	"strings"
	"time"
)

// BackgroundRemovalMode controls how generated media is matted.
// inherit is resolved by the caller against the application setting.
type BackgroundRemovalMode string

const (
	BackgroundRemovalInherit BackgroundRemovalMode = "inherit"
	BackgroundRemovalChroma  BackgroundRemovalMode = "chroma"
	BackgroundRemovalNeural  BackgroundRemovalMode = "neural"
	BackgroundRemovalHybrid  BackgroundRemovalMode = "hybrid"
)

type BackgroundRemovalTrace struct {
	RequestedMode string
	ActualMode    string
	Model         string
	ModelSHA256   string
	InferenceMs   int64
	Fallback      string
	Warnings      []string
}

// NeuralSegmenter is intentionally small so the sprite pipeline does not
// depend on a particular ONNX binding. The Windows build can provide a real
// U²-NetP implementation; unavailable/corrupt runtimes safely fall back to
// the existing chroma matte.
type NeuralSegmenter interface {
	Segment(*image.NRGBA) (*image.NRGBA, error)
	ModelName() string
	ModelSHA256() string
}

type ContextNeuralSegmenter interface {
	SegmentContext(context.Context, *image.NRGBA) (*image.NRGBA, error)
}

type BackgroundRemover struct {
	Mode   BackgroundRemovalMode
	Neural NeuralSegmenter
}

func NewBackgroundRemover(mode string, neural NeuralSegmenter) *BackgroundRemover {
	m := BackgroundRemovalMode(strings.ToLower(strings.TrimSpace(mode)))
	if m != BackgroundRemovalChroma && m != BackgroundRemovalNeural && m != BackgroundRemovalHybrid {
		m = BackgroundRemovalHybrid
	}
	return &BackgroundRemover{Mode: m, Neural: neural}
}

func (r *BackgroundRemover) Remove(src image.Image) (*image.NRGBA, BackgroundRemovalTrace) {
	return r.RemoveContext(context.Background(), src)
}

func (r *BackgroundRemover) RemoveContext(ctx context.Context, src image.Image) (*image.NRGBA, BackgroundRemovalTrace) {
	started := time.Now()
	requested := string(r.Mode)
	trace := BackgroundRemovalTrace{RequestedMode: requested, ActualMode: string(BackgroundRemovalChroma)}
	if r.Mode == BackgroundRemovalChroma {
		out := RemoveBackground(src)
		trace.InferenceMs = time.Since(started).Milliseconds()
		return out, trace
	}
	if r.Neural == nil {
		trace.Fallback = "神经抠图运行时或模型不可用，已降级为色键。"
		trace.Warnings = []string{trace.Fallback}
		out := RemoveBackground(src)
		trace.InferenceMs = time.Since(started).Milliseconds()
		return out, trace
	}
	if err := ctx.Err(); err != nil {
		trace.Fallback = err.Error()
		trace.Warnings = []string{trace.Fallback}
		return nil, trace
	}
	var mask *image.NRGBA
	var err error
	if contextual, ok := r.Neural.(ContextNeuralSegmenter); ok {
		mask, err = contextual.SegmentContext(ctx, ToNRGBA(src))
	} else {
		mask, err = r.Neural.Segment(ToNRGBA(src))
	}
	if ctx.Err() != nil {
		trace.Fallback = ctx.Err().Error()
		trace.Warnings = []string{trace.Fallback}
		return nil, trace
	}
	if err != nil || mask == nil {
		if err != nil {
			trace.Fallback = "神经抠图推理失败，已降级为色键：" + err.Error()
		} else {
			trace.Fallback = "神经抠图未返回有效 mask，已降级为色键。"
		}
		trace.Warnings = []string{trace.Fallback}
		out := RemoveBackground(src)
		trace.InferenceMs = time.Since(started).Milliseconds()
		return out, trace
	}
	trace.Model = r.Neural.ModelName()
	trace.ModelSHA256 = r.Neural.ModelSHA256()
	trace.ActualMode = string(BackgroundRemovalNeural)
	if r.Mode == BackgroundRemovalHybrid {
		// Hybrid keeps the neural silhouette but lets the existing key-color
		// pass clean exterior background and despill contaminated edges.
		mask = hybridizeMask(ToNRGBA(src), mask)
		trace.ActualMode = string(BackgroundRemovalHybrid)
	}
	trace.InferenceMs = time.Since(started).Milliseconds()
	return mask, trace
}

func hybridizeMask(src, neural *image.NRGBA) *image.NRGBA {
	keyed := RemoveBackground(src)
	out := ToNRGBA(neural)
	if out.Rect != keyed.Rect {
		return keyed
	}
	for i := 0; i+3 < len(out.Pix) && i+3 < len(keyed.Pix); i += 4 {
		if keyed.Pix[i+3] == 0 {
			out.Pix[i+3] = 0
			continue
		}
		// Reuse the chroma pass' despilled edge color while retaining the
		// neural silhouette alpha.
		out.Pix[i] = keyed.Pix[i]
		out.Pix[i+1] = keyed.Pix[i+1]
		out.Pix[i+2] = keyed.Pix[i+2]
	}
	cleanupAlpha(out)
	return out
}

func RemoveBackgroundWithMode(src image.Image, mode string, neural NeuralSegmenter) (*image.NRGBA, BackgroundRemovalTrace) {
	return NewBackgroundRemover(mode, neural).Remove(src)
}
