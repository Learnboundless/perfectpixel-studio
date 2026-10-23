package main

import (
	"container/list"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"perfectpixel/internal/config"
	"perfectpixel/internal/sprite"

	ort "github.com/yalue/onnxruntime_go"
	xdraw "golang.org/x/image/draw"
)

const (
	u2netpModelName   = "U²-NetP"
	u2netpModelSHA256 = "309c8469258dda742793dce0ebea8e6dd393174f89934733ecc8b14c76f4ddd8"
	ortDLLSHA256      = "18370c375f07357fa5874344a9d9ac17e6b6fe1eb18b1dd209d79483b4470257"
	u2netpInputSize   = 320
)

//go:embed resources/background-removal/u2netp.onnx resources/background-removal/onnxruntime.dll
var backgroundRemovalAssets embed.FS

type u2netpSegmenter struct {
	mu       sync.Mutex
	session  *ort.DynamicAdvancedSession
	input    *ort.Tensor[float32]
	output   *ort.Tensor[float32]
	inputBuf []float32
	cacheMu  sync.Mutex
	cache    map[[32]byte]*list.Element
	cacheLRU *list.List
	cacheMax int
}

type u2netpMaskCacheEntry struct {
	key    [32]byte
	alpha  []byte
	rect   image.Rectangle
	stride int
}

var (
	segmenterOnce sync.Once
	segmenterInst sprite.NeuralSegmenter
	segmenterErr  error
)

func defaultNeuralSegmenter() (sprite.NeuralSegmenter, error) {
	if strings.HasSuffix(strings.ToLower(os.Args[0]), ".test.exe") && os.Getenv("MOTIONPIXEL_ENABLE_NEURAL_TEST") != "1" {
		return nil, errors.New("测试进程默认禁用神经运行时")
	}
	segmenterOnce.Do(func() {
		segmenterInst, segmenterErr = newU2NetPSegmenter()
	})
	return segmenterInst, segmenterErr
}

func newU2NetPSegmenter() (*u2netpSegmenter, error) {
	root, err := backgroundRuntimeDir()
	if err != nil {
		return nil, err
	}
	modelPath, err := extractVerifiedAsset(root, "resources/background-removal/u2netp.onnx", "u2netp.onnx", u2netpModelSHA256)
	if err != nil {
		return nil, fmt.Errorf("释放 U²-NetP 模型: %w", err)
	}
	dllPath := adjacentVerifiedRuntimeDLL()
	if dllPath == "" {
		dllPath, err = extractVerifiedAsset(root, "resources/background-removal/onnxruntime.dll", "onnxruntime.dll", ortDLLSHA256)
		if err != nil {
			return nil, fmt.Errorf("释放 ONNX Runtime: %w", err)
		}
	}
	ort.SetSharedLibraryPath(dllPath)
	if !ort.IsInitialized() {
		if err := ort.InitializeEnvironment(); err != nil {
			return nil, fmt.Errorf("初始化 ONNX Runtime: %w", err)
		}
	}
	inputs, outputs, err := ort.GetInputOutputInfo(modelPath)
	if err != nil || len(inputs) == 0 || len(outputs) == 0 {
		return nil, fmt.Errorf("读取 U²-NetP 输入输出失败: %w", err)
	}
	inputBuf := make([]float32, 1*3*u2netpInputSize*u2netpInputSize)
	input, err := ort.NewTensor(ort.NewShape(1, 3, u2netpInputSize, u2netpInputSize), inputBuf)
	if err != nil {
		return nil, err
	}
	output, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 1, u2netpInputSize, u2netpInputSize))
	if err != nil {
		_ = input.Destroy()
		return nil, err
	}
	session, err := ort.NewDynamicAdvancedSession(modelPath, []string{inputs[0].Name}, []string{outputs[0].Name}, nil)
	if err != nil {
		_ = input.Destroy()
		_ = output.Destroy()
		return nil, err
	}
	return &u2netpSegmenter{session: session, input: input, output: output, inputBuf: inputBuf, cache: map[[32]byte]*list.Element{}, cacheLRU: list.New(), cacheMax: 96}, nil
}

func adjacentVerifiedRuntimeDLL() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	path := filepath.Join(filepath.Dir(executable), "onnxruntime.dll")
	data, err := os.ReadFile(path)
	if err != nil || sha256Hex(data) != ortDLLSHA256 {
		return ""
	}
	return path
}

func backgroundRuntimeDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "perfectpixel", "models", "u2netp-1"), nil
}

func extractVerifiedAsset(root, embeddedPath, name, expectedSHA string) (string, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	target := filepath.Join(root, name)
	if data, err := os.ReadFile(target); err == nil && sha256Hex(data) == expectedSHA {
		return target, nil
	}
	data, err := backgroundRemovalAssets.ReadFile(embeddedPath)
	if err != nil {
		return "", err
	}
	if sha256Hex(data) != expectedSHA {
		return "", errors.New("内置资源校验失败")
	}
	if err := atomicWritePrivate(target, data); err != nil {
		return "", err
	}
	return target, nil
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (s *u2netpSegmenter) ModelName() string   { return u2netpModelName }
func (s *u2netpSegmenter) ModelSHA256() string { return u2netpModelSHA256 }

func (s *u2netpSegmenter) Segment(src *image.NRGBA) (*image.NRGBA, error) {
	return s.SegmentContext(context.Background(), src)
}

func (s *u2netpSegmenter) SegmentContext(ctx context.Context, src *image.NRGBA) (*image.NRGBA, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if src == nil || src.Rect.Empty() {
		return nil, errors.New("输入图片为空")
	}
	hasher := sha256.New()
	_, _ = hasher.Write(src.Pix)
	_, _ = hasher.Write([]byte(fmt.Sprintf("%d:%d:%d", src.Rect.Dx(), src.Rect.Dy(), src.Stride)))
	var key [32]byte
	copy(key[:], hasher.Sum(nil))
	if cached := s.cachedMask(key, src); cached != nil {
		return cached, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cached := s.cachedMask(key, src); cached != nil {
		return cached, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resized, box := letterboxNRGBA(src, u2netpInputSize)
	data := s.input.GetData()
	means := [3]float32{0.485, 0.456, 0.406}
	stds := [3]float32{0.229, 0.224, 0.225}
	plane := u2netpInputSize * u2netpInputSize
	for y := 0; y < u2netpInputSize; y++ {
		for x := 0; x < u2netpInputSize; x++ {
			r, g, b, _ := resized.At(x, y).RGBA()
			values := [3]float32{float32(r>>8) / 255, float32(g>>8) / 255, float32(b>>8) / 255}
			pos := y*u2netpInputSize + x
			for channel := 0; channel < 3; channel++ {
				data[channel*plane+pos] = (values[channel] - means[channel]) / stds[channel]
			}
		}
	}
	if err := s.session.Run([]ort.Value{s.input}, []ort.Value{s.output}); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	outData := s.output.GetData()
	minValue, maxValue := float32(math.MaxFloat32), float32(-math.MaxFloat32)
	for _, value := range outData {
		if value < minValue {
			minValue = value
		}
		if value > maxValue {
			maxValue = value
		}
	}
	rangeValue := maxValue - minValue
	if rangeValue < 1e-6 {
		return nil, errors.New("U²-NetP 输出 mask 无有效范围")
	}
	mask := image.NewGray(image.Rect(0, 0, box.Dx(), box.Dy()))
	for y := 0; y < box.Dy(); y++ {
		for x := 0; x < box.Dx(); x++ {
			value := (outData[(box.Min.Y+y)*u2netpInputSize+box.Min.X+x] - minValue) / rangeValue
			mask.SetGray(x, y, color.Gray{Y: uint8(max(0, min(255, int(value*255+0.5))))})
		}
	}
	fullMask := image.NewGray(image.Rect(0, 0, src.Rect.Dx(), src.Rect.Dy()))
	xdraw.CatmullRom.Scale(fullMask, fullMask.Rect, mask, mask.Rect, xdraw.Src, nil)
	result := sprite.ToNRGBA(src)
	for y := 0; y < result.Rect.Dy(); y++ {
		for x := 0; x < result.Rect.Dx(); x++ {
			offset := result.PixOffset(x, y)
			alpha := uint16(result.Pix[offset+3]) * uint16(fullMask.GrayAt(x, y).Y) / 255
			result.Pix[offset+3] = uint8(alpha)
		}
	}
	s.storeMask(key, result)
	return result, nil
}

func cloneNRGBA(src *image.NRGBA) *image.NRGBA {
	if src == nil {
		return nil
	}
	out := image.NewNRGBA(src.Rect)
	copy(out.Pix, src.Pix)
	return out
}

func (s *u2netpSegmenter) cachedMask(key [32]byte, src *image.NRGBA) *image.NRGBA {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	element := s.cache[key]
	if element == nil {
		return nil
	}
	s.cacheLRU.MoveToFront(element)
	entry := element.Value.(u2netpMaskCacheEntry)
	if src == nil || src.Rect != entry.rect {
		return nil
	}
	mask := cloneNRGBA(src)
	for y := 0; y < entry.rect.Dy(); y++ {
		for x := 0; x < entry.rect.Dx(); x++ {
			offset := mask.PixOffset(entry.rect.Min.X+x, entry.rect.Min.Y+y)
			alpha := entry.alpha[y*entry.stride+x]
			mask.Pix[offset+3] = uint8(uint16(mask.Pix[offset+3]) * uint16(alpha) / 255)
		}
	}
	return mask
}

func (s *u2netpSegmenter) storeMask(key [32]byte, mask *image.NRGBA) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	alpha := make([]byte, mask.Rect.Dx()*mask.Rect.Dy())
	for y := 0; y < mask.Rect.Dy(); y++ {
		for x := 0; x < mask.Rect.Dx(); x++ {
			alpha[y*mask.Rect.Dx()+x] = mask.Pix[mask.PixOffset(mask.Rect.Min.X+x, mask.Rect.Min.Y+y)+3]
		}
	}
	entry := u2netpMaskCacheEntry{key: key, alpha: alpha, rect: mask.Rect, stride: mask.Rect.Dx()}
	if element := s.cache[key]; element != nil {
		element.Value = entry
		s.cacheLRU.MoveToFront(element)
		return
	}
	element := s.cacheLRU.PushFront(entry)
	s.cache[key] = element
	for s.cacheLRU.Len() > s.cacheMax {
		oldest := s.cacheLRU.Back()
		entry := oldest.Value.(u2netpMaskCacheEntry)
		delete(s.cache, entry.key)
		s.cacheLRU.Remove(oldest)
	}
}

func letterboxNRGBA(src *image.NRGBA, size int) (*image.NRGBA, image.Rectangle) {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	scale := math.Min(float64(size)/float64(w), float64(size)/float64(h))
	newW := max(1, int(float64(w)*scale+0.5))
	newH := max(1, int(float64(h)*scale+0.5))
	left, top := (size-newW)/2, (size-newH)/2
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	box := image.Rect(left, top, left+newW, top+newH)
	xdraw.CatmullRom.Scale(dst, box, src, src.Rect, xdraw.Src, nil)
	return dst, box
}

func appBackgroundRemover(requested string) (*sprite.BackgroundRemover, string) {
	settings := config.Load()
	mode := resolveBackgroundRemovalMode(requested, settings)
	segmenter, _ := defaultNeuralSegmenter()
	return sprite.NewBackgroundRemover(mode, segmenter), mode
}
