// Command ppark-video-smoke runs paid, end-to-end Volcengine Ark video checks.
//
// The API key is accepted only through VOLCENGINE_ARK_API_KEY or ARK_CN_API_KEY.
// Reports contain request metadata and task results, but never credentials or
// inline reference-image data.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"perfectpixel/internal/gen"
)

const defaultModel = "doubao-seedance-2-0-mini-260615"

type options struct {
	model        string
	cases        string
	outDir       string
	reference    string
	seconds      int
	resolution   string
	ratio        string
	pollInterval time.Duration
	pollTimeout  time.Duration
	confirmCost  bool
}

type statusEvent struct {
	At       string `json:"at"`
	Status   string `json:"status"`
	Progress int    `json:"progress,omitempty"`
	Message  string `json:"message,omitempty"`
}

type requestSummary struct {
	Workflow      string `json:"workflow"`
	ReferenceMode string `json:"referenceMode"`
	Prompt        string `json:"prompt"`
	ReferenceFile string `json:"referenceFile,omitempty"`
	Duration      int    `json:"duration"`
	Resolution    string `json:"resolution"`
	Ratio         string `json:"ratio"`
	GenerateAudio bool   `json:"generateAudio"`
	Watermark     bool   `json:"watermark"`
}

type caseResult struct {
	Name          string         `json:"name"`
	Request       requestSummary `json:"request"`
	TaskID        string         `json:"taskId,omitempty"`
	Events        []statusEvent  `json:"events,omitempty"`
	FinalStatus   string         `json:"finalStatus,omitempty"`
	Downloaded    bool           `json:"downloaded"`
	DownloadBytes int            `json:"downloadBytes,omitempty"`
	VideoPath     string         `json:"videoPath,omitempty"`
	ElapsedMs     int64          `json:"elapsedMs"`
	Error         string         `json:"error,omitempty"`
}

type report struct {
	GeneratedAt     string       `json:"generatedAt"`
	Provider        string       `json:"provider"`
	BaseURL         string       `json:"baseUrl"`
	Model           string       `json:"model"`
	ModelListed     bool         `json:"modelListed"`
	AvailableModels []string     `json:"availableModels"`
	KeySource       string       `json:"keySource"`
	Cases           []caseResult `json:"cases"`
}

func main() {
	var opt options
	flag.StringVar(&opt.model, "model", defaultModel, "Ark video model ID.")
	flag.StringVar(&opt.cases, "case", "all", "Cases to run: all, t2v, i2v, or comma-separated values.")
	flag.StringVar(&opt.outDir, "out", filepath.Join(".codex", "ark-video-smoke-real", time.Now().Format("20060102-150405")), "Output directory.")
	flag.StringVar(&opt.reference, "reference", filepath.Join("samples", "idle.gif"), "Local image/GIF used for identity-reference I2V.")
	flag.IntVar(&opt.seconds, "seconds", 4, "Requested duration in seconds.")
	flag.StringVar(&opt.resolution, "resolution", "480p", "Requested resolution: 480p or 720p.")
	flag.StringVar(&opt.ratio, "ratio", "1:1", "Requested aspect ratio.")
	flag.DurationVar(&opt.pollInterval, "poll-interval", 5*time.Second, "Polling interval.")
	flag.DurationVar(&opt.pollTimeout, "poll-timeout", 20*time.Minute, "Per-task polling timeout.")
	flag.BoolVar(&opt.confirmCost, "confirm-cost", false, "Required because this command creates paid tasks.")
	flag.Parse()

	if err := run(opt); err != nil {
		fmt.Fprintf(os.Stderr, "ppark-video-smoke 失败: %v\n", err)
		os.Exit(1)
	}
}

func run(opt options) error {
	if !opt.confirmCost && os.Getenv("PP_ARK_REAL_VIDEO_SMOKE") != "1" {
		return errors.New("真实 Ark 视频 smoke 会创建付费任务；请加 -confirm-cost 或设置 PP_ARK_REAL_VIDEO_SMOKE=1")
	}
	key, keySource := resolveKey()
	if key == "" {
		return errors.New("请通过 VOLCENGINE_ARK_API_KEY 或 ARK_CN_API_KEY 提供火山方舟密钥")
	}
	if opt.seconds < 4 {
		opt.seconds = 4
	}
	if opt.seconds > 15 {
		opt.seconds = 15
	}
	opt.resolution = strings.ToLower(strings.TrimSpace(opt.resolution))
	if opt.resolution != "480p" && opt.resolution != "720p" {
		return fmt.Errorf("Seedance 2.0 Mini 仅支持 480p 或 720p，收到 %q", opt.resolution)
	}
	if strings.TrimSpace(opt.ratio) == "" {
		opt.ratio = "1:1"
	}
	if opt.pollInterval <= 0 {
		opt.pollInterval = 5 * time.Second
	}
	if opt.pollTimeout <= 0 {
		opt.pollTimeout = 20 * time.Minute
	}
	if err := os.MkdirAll(opt.outDir, 0o755); err != nil {
		return err
	}

	client := gen.NewVolcengine(key, opt.model)
	modelCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	models, err := client.ListVideoModels(modelCtx)
	cancel()
	rep := report{
		GeneratedAt:     time.Now().Format(time.RFC3339),
		Provider:        gen.ProviderVolcengine,
		BaseURL:         "https://ark.cn-beijing.volces.com/api/v3",
		Model:           opt.model,
		AvailableModels: models,
		KeySource:       keySource,
	}
	if err != nil {
		_ = writeReport(opt.outDir, rep)
		return fmt.Errorf("读取 Ark 视频模型列表失败: %w", err)
	}
	rep.ModelListed = containsFold(models, opt.model)
	if !rep.ModelListed {
		_ = writeReport(opt.outDir, rep)
		return fmt.Errorf("上游模型列表没有 %s，未创建付费任务", opt.model)
	}

	referencePNG, err := readReferencePNG(opt.reference)
	if err != nil && selectedCases(opt.cases)["i2v"] {
		return err
	}
	for _, sc := range buildCases(opt, referencePNG) {
		fmt.Printf("创建 %s：model=%s duration=%ds resolution=%s ratio=%s\n",
			sc.Name, opt.model, opt.seconds, opt.resolution, opt.ratio)
		result := runCase(client, opt, sc)
		rep.Cases = append(rep.Cases, result)
		if err := writeReport(opt.outDir, rep); err != nil {
			return err
		}
		if result.Error != "" {
			fmt.Printf("%s 失败：%s\n", sc.Name, result.Error)
		} else {
			fmt.Printf("%s 成功：%s (%d bytes)\n", sc.Name, result.VideoPath, result.DownloadBytes)
		}
	}
	return writeReport(opt.outDir, rep)
}

type smokeCase struct {
	Name    string
	Request gen.VideoRequest
	Summary requestSummary
}

func buildCases(opt options, referencePNG []byte) []smokeCase {
	selected := selectedCases(opt.cases)
	out := make([]smokeCase, 0, 2)
	if selected["t2v"] {
		prompt := "A single low-resolution pixel-art game character performs one clean melee attack cycle with anticipation, strike, follow-through, and recovery. Fixed orthographic camera, full body always visible with generous padding, centered stable feet, pure magenta background, no camera motion, no cuts, no text, no particles, no motion blur."
		out = append(out, smokeCase{
			Name: "t2v",
			Request: gen.VideoRequest{
				Model:         opt.model,
				Prompt:        prompt,
				Workflow:      gen.VideoWorkflowTextToVideo,
				ReferenceMode: gen.VideoReferenceModeNone,
				Seconds:       opt.seconds,
				Resolution:    opt.resolution,
				AspectRatio:   opt.ratio,
			},
			Summary: requestSummary{
				Workflow:      gen.VideoWorkflowTextToVideo,
				ReferenceMode: gen.VideoReferenceModeNone,
				Prompt:        prompt,
				Duration:      opt.seconds,
				Resolution:    opt.resolution,
				Ratio:         opt.ratio,
			},
		})
	}
	if selected["i2v"] {
		prompt := "Use the attached image only as the canonical character identity reference. Preserve the same face, silhouette, outfit, palette, proportions, and pixel-art style while the character performs one readable melee attack cycle. Do not treat the image as a mandatory frozen first frame. Fixed orthographic camera, full body always visible with generous padding, stable feet, pure magenta background, no camera motion, no cuts, no text, no particles, no motion blur."
		out = append(out, smokeCase{
			Name: "i2v_identity",
			Request: gen.VideoRequest{
				Model:         opt.model,
				Prompt:        prompt,
				Workflow:      gen.VideoWorkflowReferenceToVideo,
				ReferenceMode: gen.VideoReferenceModeIdentityReference,
				MediaSlots: map[string][]gen.VideoMediaInput{
					gen.VideoSlotReferenceImages: {{Data: referencePNG, MIME: "image/png", Label: filepath.Base(opt.reference)}},
				},
				Seconds:     opt.seconds,
				Resolution:  opt.resolution,
				AspectRatio: opt.ratio,
			},
			Summary: requestSummary{
				Workflow:      gen.VideoWorkflowReferenceToVideo,
				ReferenceMode: gen.VideoReferenceModeIdentityReference,
				Prompt:        prompt,
				ReferenceFile: filepath.Base(opt.reference),
				Duration:      opt.seconds,
				Resolution:    opt.resolution,
				Ratio:         opt.ratio,
			},
		})
	}
	return out
}

func runCase(client *gen.Volcengine, opt options, sc smokeCase) caseResult {
	started := time.Now()
	result := caseResult{Name: sc.Name, Request: sc.Summary}
	ctx, cancel := context.WithTimeout(context.Background(), opt.pollTimeout)
	defer cancel()

	task, err := client.GenerateVideo(ctx, sc.Request)
	if err != nil {
		result.Error = err.Error()
		result.ElapsedMs = time.Since(started).Milliseconds()
		return result
	}
	result.TaskID = task.ID
	result.Events = append(result.Events, eventFor(task))
	fmt.Printf("任务已创建：%s status=%s\n", task.ID, task.Status)

	consecutiveErrors := 0
	for !task.Done() && !task.Failed() {
		if err := sleepContext(ctx, opt.pollInterval); err != nil {
			result.Error = "等待任务完成超时或已取消: " + err.Error()
			result.ElapsedMs = time.Since(started).Milliseconds()
			return result
		}
		next, err := client.PollVideo(ctx, task.ID)
		if err != nil {
			consecutiveErrors++
			result.Events = append(result.Events, statusEvent{
				At:      time.Now().Format(time.RFC3339),
				Status:  "poll_error",
				Message: err.Error(),
			})
			if consecutiveErrors >= 5 {
				result.Error = fmt.Sprintf("连续 %d 次轮询失败: %v", consecutiveErrors, err)
				result.ElapsedMs = time.Since(started).Milliseconds()
				return result
			}
			continue
		}
		consecutiveErrors = 0
		task = next
		result.Events = append(result.Events, eventFor(task))
		fmt.Printf("轮询：%s status=%s progress=%d\n", task.ID, task.Status, task.Progress)
	}
	result.FinalStatus = task.Status
	if task.Failed() {
		result.Error = strings.TrimSpace(task.Message)
		if result.Error == "" {
			result.Error = "Ark 视频任务返回失败状态"
		}
		result.ElapsedMs = time.Since(started).Milliseconds()
		return result
	}

	video, err := client.DownloadVideo(ctx, task.URL)
	if err != nil {
		result.Error = err.Error()
		result.ElapsedMs = time.Since(started).Milliseconds()
		return result
	}
	if len(video) < 1024 {
		result.Error = fmt.Sprintf("下载内容过小，疑似不是有效 MP4：%d bytes", len(video))
		result.ElapsedMs = time.Since(started).Milliseconds()
		return result
	}
	videoPath := filepath.Join(opt.outDir, safeName(opt.model)+"-"+sc.Name+".mp4")
	if err := os.WriteFile(videoPath, video, 0o600); err != nil {
		result.Error = err.Error()
		result.ElapsedMs = time.Since(started).Milliseconds()
		return result
	}
	result.Downloaded = true
	result.DownloadBytes = len(video)
	result.VideoPath = videoPath
	result.ElapsedMs = time.Since(started).Milliseconds()
	return result
}

func eventFor(task gen.VideoResult) statusEvent {
	return statusEvent{
		At:       time.Now().Format(time.RFC3339),
		Status:   task.Status,
		Progress: task.Progress,
		Message:  task.Message,
	}
}

func selectedCases(raw string) map[string]bool {
	out := map[string]bool{}
	for _, item := range strings.Split(strings.ToLower(raw), ",") {
		switch strings.TrimSpace(item) {
		case "all":
			out["t2v"] = true
			out["i2v"] = true
		case "t2v":
			out["t2v"] = true
		case "i2v", "i2v_identity":
			out["i2v"] = true
		}
	}
	if len(out) == 0 {
		out["t2v"] = true
		out["i2v"] = true
	}
	return out
}

func resolveKey() (string, string) {
	if key := strings.TrimSpace(os.Getenv("VOLCENGINE_ARK_API_KEY")); key != "" {
		return key, "VOLCENGINE_ARK_API_KEY"
	}
	if key := strings.TrimSpace(os.Getenv("ARK_CN_API_KEY")); key != "" {
		return key, "ARK_CN_API_KEY"
	}
	return "", ""
}

func readReferencePNG(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 I2V 参考图失败: %w", err)
	}
	defer file.Close()
	img, _, err := image.Decode(io.LimitReader(file, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("解码 I2V 参考图失败: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("编码 I2V 参考图失败: %w", err)
	}
	return buf.Bytes(), nil
}

func writeReport(outDir string, value report) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(outDir, "report.json.tmp")
	final := filepath.Join(outDir, "report.json")
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func safeName(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
