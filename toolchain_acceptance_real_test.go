package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"perfectpixel/internal/config"
	"perfectpixel/internal/gen"
	"perfectpixel/internal/uiassets"
)

const realAcceptanceEstimateLimitCNY = 25.0

type realAcceptanceCost struct {
	Name             string  `json:"name"`
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	EstimatedCostCNY float64 `json:"estimatedCostCny"`
}

var realAcceptanceCosts = []realAcceptanceCost{
	{Name: "prompt_optimization_image", Provider: gen.ProviderVolcengine, EstimatedCostCNY: 0.25},
	{Name: "prompt_optimization_ui", Provider: gen.ProviderVolcengine, EstimatedCostCNY: 0.25},
	{Name: "text_to_image", Provider: gen.ProviderZetaTechs, Model: "gpt-image-2", EstimatedCostCNY: 2.0},
	{Name: "reference_to_ui", Provider: gen.ProviderZetaTechs, Model: "gpt-image-2", EstimatedCostCNY: 2.0},
	{Name: "masked_ui_edit", Provider: gen.ProviderZetaTechs, Model: "gpt-image-2", EstimatedCostCNY: 2.0},
	{Name: "multimodal_ui_analysis", Provider: gen.ProviderZetaTechs, Model: "gpt-4o-mini", EstimatedCostCNY: 1.0},
	{Name: "text_to_video_4s_480p", Provider: gen.ProviderVolcengine, Model: "doubao-seedance-2-0-mini-260615", EstimatedCostCNY: 8.0},
	{Name: "image_to_video_4s_480p", Provider: gen.ProviderVolcengine, Model: "doubao-seedance-2-0-mini-260615", EstimatedCostCNY: 8.0},
}

type realAcceptanceCaseReport struct {
	Name              string   `json:"name"`
	Kind              string   `json:"kind"`
	Provider          string   `json:"provider"`
	Model             string   `json:"model"`
	EstimatedCostCNY  float64  `json:"estimatedCostCny"`
	ActualCostCNY     *float64 `json:"actualCostCny,omitempty"`
	ProductJobID      string   `json:"productJobId,omitempty"`
	ProviderRequestID string   `json:"providerRequestId,omitempty"`
	Status            string   `json:"status"`
	ElapsedMs         int64    `json:"elapsedMs"`
	OutputAssetIDs    []string `json:"outputAssetIds,omitempty"`
	OutputPaths       []string `json:"outputPaths,omitempty"`
	Validation        []string `json:"validation,omitempty"`
	Error             string   `json:"error,omitempty"`
}

type realAcceptanceReport struct {
	StartedAt             string                     `json:"startedAt"`
	CompletedAt           string                     `json:"completedAt,omitempty"`
	AuthorizedBudgetCNY   float64                    `json:"authorizedBudgetCny"`
	EstimateLimitCNY      float64                    `json:"estimateLimitCny"`
	EstimatedTotalCostCNY float64                    `json:"estimatedTotalCostCny"`
	ActualTotalCostCNY    *float64                   `json:"actualTotalCostCny,omitempty"`
	ActualCostNote        string                     `json:"actualCostNote"`
	ToolchainRoot         string                     `json:"toolchainRoot"`
	CostPlan              []realAcceptanceCost       `json:"costPlan"`
	Cases                 []realAcceptanceCaseReport `json:"cases"`
}

func TestRealToolchainAcceptance(t *testing.T) {
	if os.Getenv("PP_TOOLCHAIN_REAL_ACCEPTANCE") != "1" {
		t.Skip("set PP_TOOLCHAIN_REAL_ACCEPTANCE=1 and use scripts/toolchain-real-acceptance.ps1 to run paid acceptance")
	}

	budget, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv("PP_TOOLCHAIN_ACCEPTANCE_BUDGET_CNY")), 64)
	if err != nil || budget <= 0 || budget > 30 {
		t.Fatalf("PP_TOOLCHAIN_ACCEPTANCE_BUDGET_CNY must be greater than 0 and no more than 30")
	}
	estimatedTotal := 0.0
	for _, item := range realAcceptanceCosts {
		estimatedTotal += item.EstimatedCostCNY
	}
	if estimatedTotal > realAcceptanceEstimateLimitCNY {
		t.Fatalf("estimated cost %.2f CNY exceeds the %.2f CNY creation gate", estimatedTotal, realAcceptanceEstimateLimitCNY)
	}
	if estimatedTotal > budget {
		t.Fatalf("estimated cost %.2f CNY exceeds authorized budget %.2f CNY", estimatedTotal, budget)
	}

	root := strings.TrimSpace(os.Getenv("PP_TOOLCHAIN_ACCEPTANCE_ROOT"))
	if root == "" {
		root = filepath.Join(t.TempDir(), "toolchain")
	}
	reportPath := strings.TrimSpace(os.Getenv("PP_TOOLCHAIN_ACCEPTANCE_REPORT"))
	if reportPath == "" {
		reportPath = filepath.Join(root, "acceptance-report.json")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	reportPath, err = filepath.Abs(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	report := realAcceptanceReport{
		StartedAt:             time.Now().UTC().Format(time.RFC3339Nano),
		AuthorizedBudgetCNY:   budget,
		EstimateLimitCNY:      realAcceptanceEstimateLimitCNY,
		EstimatedTotalCostCNY: estimatedTotal,
		ActualCostNote:        "Providers did not expose per-request billing in their task responses; actual cost is intentionally left null.",
		ToolchainRoot:         root,
		CostPlan:              append([]realAcceptanceCost(nil), realAcceptanceCosts...),
	}
	defer func() {
		report.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if writeErr := writeRealAcceptanceReport(reportPath, report); writeErr != nil {
			t.Logf("write acceptance report: %v", writeErr)
		}
	}()

	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Fatalf("ffprobe is required before paid video cases: %v", err)
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Fatalf("ffmpeg is required before paid video cases: %v", err)
	}
	settings := config.Load()
	if strings.TrimSpace(settings.ZetaTechs.APIKey) == "" {
		t.Fatal("ZetaTechs is not configured")
	}
	if strings.TrimSpace(settings.Volcengine.APIKey) == "" {
		t.Fatal("Volcengine Ark is not configured")
	}
	if firstNonBlank(settings.Volcengine.TextAPIKey, settings.Volcengine.APIKey) == "" {
		t.Fatal("Volcengine text API is not configured")
	}

	textModel := firstNonBlank(settings.Volcengine.TextModel, gen.DefaultTextModelFor(gen.ProviderVolcengine))
	for index := range report.CostPlan {
		if report.CostPlan[index].Provider == gen.ProviderVolcengine && report.CostPlan[index].Model == "" {
			report.CostPlan[index].Model = textModel
		}
	}
	store, err := OpenToolchainStore(root)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{toolchainStore: store}
	appContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.ctx = appContext
	if err := app.initToolchain(appContext); err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(appContext)

	referencePNG, err := makeAcceptanceUIReference()
	if err != nil {
		t.Fatal(err)
	}
	referenceAsset, err := store.CreateAssetFromBytes(ToolAsset{
		Name:         "acceptance-ui-reference.png",
		MediaType:    AssetMediaImage,
		MIMEType:     "image/png",
		Usage:        "acceptance_reference",
		SourceModule: ToolchainModuleUI,
		Tags:         []string{"acceptance", "local"},
	}, referencePNG)
	if err != nil {
		t.Fatal(err)
	}

	run := func(costName, kind string, timeout time.Duration, create func() (GenerationJob, error), validate func(GenerationJob) ([]string, error)) GenerationJob {
		cost := acceptanceCostByName(costName)
		job, caseReport, runErr := runRealAcceptanceCase(app, store, cost, kind, timeout, create, validate)
		report.Cases = append(report.Cases, caseReport)
		if writeErr := writeRealAcceptanceReport(reportPath, report); writeErr != nil {
			t.Fatalf("write incremental acceptance report: %v", writeErr)
		}
		if runErr != nil {
			t.Fatalf("%s failed; later paid cases were not submitted: %v", costName, runErr)
		}
		return job
	}

	run("prompt_optimization_image", string(GenerationJobPromptOptimization), 3*time.Minute, func() (GenerationJob, error) {
		return app.CreatePromptOptimizationJob(map[string]any{
			"sourceModule": "image", "purpose": "image", "provider": gen.ProviderVolcengine, "model": textModel,
			"prompt": "A readable inventory icon for a compact sci-fi energy cell, isolated game asset, clean silhouette.",
		})
	}, func(job GenerationJob) ([]string, error) { return validateAcceptancePrompt(store, job) })

	run("prompt_optimization_ui", string(GenerationJobPromptOptimization), 3*time.Minute, func() (GenerationJob, error) {
		return app.CreatePromptOptimizationJob(map[string]any{
			"sourceModule": "ui", "purpose": "ui", "provider": gen.ProviderVolcengine, "model": textModel,
			"prompt": "A complete mobile RPG inventory screen with item grid, equipment panel, currencies, filters and clear selected states.",
		})
	}, func(job GenerationJob) ([]string, error) { return validateAcceptancePrompt(store, job) })

	textImageJob := run("text_to_image", string(GenerationJobImage), 8*time.Minute, func() (GenerationJob, error) {
		return app.CreateImageJob(map[string]any{
			"sourceModule": "image", "purpose": "image", "mode": "text", "provider": gen.ProviderZetaTechs, "model": "gpt-image-2",
			"prompt":      "A single compact sci-fi energy cell inventory icon for a strategy game, crisp readable silhouette, teal glass core, white ceramic shell, subtle amber warning stripe, transparent-looking neutral studio background, no text, no watermark.",
			"aspectRatio": "1:1", "variants": 1,
		})
	}, func(job GenerationJob) ([]string, error) { return validateAcceptanceImages(store, job) })
	textImageID := textImageJob.OutputAssetIDs[0]

	uiConceptJob := run("reference_to_ui", string(GenerationJobImage), 8*time.Minute, func() (GenerationJob, error) {
		return app.CreateImageJob(map[string]any{
			"sourceModule": "ui", "purpose": "ui_concept", "mode": "reference", "provider": gen.ProviderZetaTechs, "model": "gpt-image-2",
			"prompt":            "Turn this wireframe into a complete production-ready 16:9 science-fantasy RPG inventory screen. Preserve the information hierarchy and grid placement. Use restrained charcoal panels, ivory text, cyan selection accents, coral warnings, clear item rarity states, and legible labels. Show one coherent full screen, not a mood board, no device mockup, no watermark.",
			"referenceAssetIds": []string{referenceAsset.ID}, "aspectRatio": "16:9", "variants": 1,
		})
	}, func(job GenerationJob) ([]string, error) { return validateAcceptanceImages(store, job) })
	uiConceptID := uiConceptJob.OutputAssetIDs[0]
	uiConceptAsset, err := store.GetAsset(uiConceptID)
	if err != nil {
		t.Fatal(err)
	}
	maskWidth := max(32, uiConceptAsset.Width/3)
	maskHeight := max(32, uiConceptAsset.Height/4)
	maskX := max(0, (uiConceptAsset.Width-maskWidth)/2)
	maskY := max(0, (uiConceptAsset.Height-maskHeight)/2)

	maskedJob := run("masked_ui_edit", string(GenerationJobImage), 8*time.Minute, func() (GenerationJob, error) {
		return app.CreateImageJob(map[string]any{
			"sourceModule": "ui", "purpose": "ui_concept", "mode": "masked_edit", "provider": gen.ProviderZetaTechs, "model": "gpt-image-2",
			"prompt":            "Replace only the masked center panel with a detailed equipment comparison panel containing two weapon cards, stat deltas, upgrade cost, and a clear equip action. Match the existing screen exactly outside the mask.",
			"referenceAssetIds": []string{uiConceptID},
			"maskRegions":       []map[string]int{{"x": maskX, "y": maskY, "width": maskWidth, "height": maskHeight}},
			"aspectRatio":       "16:9", "variants": 1,
		})
	}, func(job GenerationJob) ([]string, error) { return validateAcceptanceImages(store, job) })
	maskedID := maskedJob.OutputAssetIDs[0]

	run("multimodal_ui_analysis", string(GenerationJobUIAnalysis), 6*time.Minute, func() (GenerationJob, error) {
		return app.CreateUIAnalysisJob(map[string]any{
			"sourceModule": "ui", "sourceAssetId": maskedID, "provider": gen.ProviderZetaTechs, "model": "gpt-4o-mini",
			"prompt": "Inventory screen for a desktop and controller-friendly science-fantasy RPG. Identify useful visible components and readable copy.",
		})
	}, func(job GenerationJob) ([]string, error) { return validateAcceptanceUIAnalysis(store, job) })

	run("text_to_video_4s_480p", string(GenerationJobVideo), 25*time.Minute, func() (GenerationJob, error) {
		return app.CreateVideoJob(map[string]any{
			"sourceModule": "video", "mode": "text", "referenceMode": "none", "provider": gen.ProviderVolcengine, "model": "doubao-seedance-2-0-mini-260615",
			"prompt":          "A four-second game-asset shot of a compact teal energy cell hovering above a clean neutral platform, slowly rotating once, locked camera, stable shape and materials, seamless calm motion, no text, no watermark.",
			"durationSeconds": 4, "resolution": "480p", "aspectRatio": "16:9", "audio": false, "watermark": false,
		})
	}, func(job GenerationJob) ([]string, error) { return validateAcceptanceVideo(store, job) })

	run("image_to_video_4s_480p", string(GenerationJobVideo), 25*time.Minute, func() (GenerationJob, error) {
		return app.CreateVideoJob(map[string]any{
			"sourceModule": "video", "mode": "image", "referenceMode": "first_frame", "provider": gen.ProviderVolcengine, "model": "doubao-seedance-2-0-mini-260615",
			"prompt":            "Animate this exact game item without redesigning it. The energy core pulses gently while the object makes a small controlled hover and returns to its starting pose. Locked camera, stable silhouette, no added objects, no text, no watermark.",
			"referenceAssetIds": []string{textImageID}, "durationSeconds": 4, "resolution": "480p", "aspectRatio": "1:1", "audio": false, "watermark": false,
		})
	}, func(job GenerationJob) ([]string, error) { return validateAcceptanceVideo(store, job) })
}

func acceptanceCostByName(name string) realAcceptanceCost {
	for _, item := range realAcceptanceCosts {
		if item.Name == name {
			return item
		}
	}
	return realAcceptanceCost{Name: name}
}

func runRealAcceptanceCase(app *App, store *ToolchainStore, cost realAcceptanceCost, kind string, timeout time.Duration, create func() (GenerationJob, error), validate func(GenerationJob) ([]string, error)) (GenerationJob, realAcceptanceCaseReport, error) {
	started := time.Now()
	report := realAcceptanceCaseReport{Name: cost.Name, Kind: kind, Provider: cost.Provider, Model: cost.Model, EstimatedCostCNY: cost.EstimatedCostCNY, Status: "creating"}
	job, err := create()
	if err != nil {
		report.Status, report.Error, report.ElapsedMs = "create_failed", err.Error(), time.Since(started).Milliseconds()
		return GenerationJob{}, report, err
	}
	report.ProductJobID = job.ID
	completed, err := waitRealAcceptanceJob(app, job.ID, timeout)
	report.ElapsedMs = time.Since(started).Milliseconds()
	report.Status = string(completed.Status)
	report.Provider = completed.Provider
	report.Model = completed.Model
	report.ProviderRequestID = completed.ProviderTaskID
	report.OutputAssetIDs = append([]string(nil), completed.OutputAssetIDs...)
	for _, id := range completed.OutputAssetIDs {
		if path, pathErr := store.ResolveAssetContentPath(id); pathErr == nil {
			report.OutputPaths = append(report.OutputPaths, path)
		}
	}
	if err != nil {
		report.Error = err.Error()
		return completed, report, err
	}
	validation, err := validate(completed)
	report.Validation = validation
	if err != nil {
		report.Status = "validation_failed"
		report.Error = err.Error()
		return completed, report, err
	}
	return completed, report, nil
}

func waitRealAcceptanceJob(app *App, id string, timeout time.Duration) (GenerationJob, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		job, err := app.GetGenerationJob(id)
		if err != nil {
			return GenerationJob{}, err
		}
		switch job.Status {
		case GenerationJobCompleted:
			return job, nil
		case GenerationJobFailed, GenerationJobCancelled, GenerationJobStopped, GenerationJobNeedsReview:
			return job, fmt.Errorf("job %s ended as %s: %s", job.ID, job.Status, job.Error)
		}
		select {
		case <-deadline.C:
			_, _ = app.CancelGenerationJob(id)
			return job, fmt.Errorf("job %s exceeded %s", id, timeout)
		case <-ticker.C:
		}
	}
}

func validateAcceptancePrompt(store *ToolchainStore, job GenerationJob) ([]string, error) {
	if len(job.OutputAssetIDs) != 1 {
		return nil, fmt.Errorf("prompt job returned %d assets", len(job.OutputAssetIDs))
	}
	path, err := store.ResolveAssetContentPath(job.OutputAssetIDs[0])
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) < 40 {
		return nil, errors.New("optimized prompt is unexpectedly short")
	}
	return []string{"optimized prompt is non-empty", fmt.Sprintf("text bytes=%d", len(data))}, nil
}

func validateAcceptanceImages(store *ToolchainStore, job GenerationJob) ([]string, error) {
	if len(job.OutputAssetIDs) == 0 {
		return nil, errors.New("image job returned no assets")
	}
	checks := make([]string, 0, len(job.OutputAssetIDs))
	for _, id := range job.OutputAssetIDs {
		path, err := store.ResolveAssetContentPath(id)
		if err != nil {
			return checks, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return checks, err
		}
		img, info, err := uiassets.DecodeBytes(data, uiassets.DefaultDecodeLimits())
		if err != nil {
			return checks, err
		}
		opaque := 0
		for index := 3; index < len(img.Pix); index += 4 {
			if img.Pix[index] > 0 {
				opaque++
			}
		}
		if info.Width <= 0 || info.Height <= 0 || opaque == 0 {
			return checks, errors.New("decoded image is empty")
		}
		checks = append(checks, fmt.Sprintf("%s decodes as %dx%d %s (%d bytes)", id, info.Width, info.Height, info.MIMEType, len(data)))
	}
	return checks, nil
}

func validateAcceptanceUIAnalysis(store *ToolchainStore, job GenerationJob) ([]string, error) {
	if len(job.ProviderResponseSummary) == 0 {
		return nil, errors.New("UI analysis summary is empty")
	}
	var document uiAnalysisDocument
	if err := json.Unmarshal(job.ProviderResponseSummary, &document); err != nil {
		return nil, err
	}
	if len(document.Regions) == 0 || document.Canvas.Width <= 0 || document.Canvas.Height <= 0 {
		return nil, errors.New("UI analysis has no usable regions or canvas")
	}
	canvas := uiassets.Size{Width: document.Canvas.Width, Height: document.Canvas.Height}
	for _, region := range document.Regions {
		bounds := uiassets.PixelRect(region.Bounds)
		if bounds.Empty() || bounds.Intersect(canvas) != bounds {
			return nil, fmt.Errorf("region %s is outside the source canvas: %#v", region.ID, region.Bounds)
		}
	}
	if len(job.OutputAssetIDs) < 2 {
		return nil, errors.New("UI analysis did not persist its JSON and visible crops")
	}
	for _, id := range job.OutputAssetIDs {
		if _, err := store.ResolveAssetContentPath(id); err != nil {
			return nil, err
		}
	}
	return []string{fmt.Sprintf("regions=%d", len(document.Regions)), "all region coordinates are inside the source canvas", "analysis JSON and crops are persisted"}, nil
}

func validateAcceptanceVideo(store *ToolchainStore, job GenerationJob) ([]string, error) {
	if len(job.OutputAssetIDs) != 1 {
		return nil, fmt.Errorf("video job returned %d assets", len(job.OutputAssetIDs))
	}
	path, err := store.ResolveAssetContentPath(job.OutputAssetIDs[0])
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	width, height, durationMs, err := probeVideoMetadata(context.Background(), data, filepath.Ext(path))
	if err != nil {
		return nil, fmt.Errorf("ffprobe validation: %w", err)
	}
	if durationMs < 2500 || durationMs > 8000 {
		return nil, fmt.Errorf("unexpected video duration %dms", durationMs)
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, err
	}
	decodeContext, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(decodeContext, ffmpeg, "-v", "error", "-i", path, "-frames:v", "1", "-f", "null", "-")
	if output, err := command.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("video decode/playback check: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return []string{fmt.Sprintf("ffprobe=%dx%d duration=%dms", width, height, durationMs), "ffmpeg decoded the first playback frame", fmt.Sprintf("video bytes=%d", len(data))}, nil
}

func makeAcceptanceUIReference() ([]byte, error) {
	img := image.NewNRGBA(image.Rect(0, 0, 1024, 576))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.NRGBA{R: 22, G: 25, B: 31, A: 255}}, image.Point{}, draw.Src)
	fill := func(rect image.Rectangle, value color.NRGBA) {
		draw.Draw(img, rect, &image.Uniform{C: value}, image.Point{}, draw.Src)
	}
	fill(image.Rect(0, 0, 1024, 64), color.NRGBA{R: 42, G: 47, B: 57, A: 255})
	fill(image.Rect(24, 88, 668, 548), color.NRGBA{R: 35, G: 39, B: 48, A: 255})
	fill(image.Rect(692, 88, 1000, 548), color.NRGBA{R: 31, G: 35, B: 43, A: 255})
	for row := 0; row < 4; row++ {
		for column := 0; column < 6; column++ {
			x := 48 + column*98
			y := 126 + row*96
			fill(image.Rect(x, y, x+78, y+76), color.NRGBA{R: uint8(45 + column*5), G: uint8(68 + row*8), B: uint8(80 + column*9), A: 255})
			fill(image.Rect(x+8, y+8, x+70, y+12), color.NRGBA{R: 98, G: 205, B: 214, A: 255})
		}
	}
	fill(image.Rect(718, 120, 974, 302), color.NRGBA{R: 51, G: 57, B: 68, A: 255})
	fill(image.Rect(718, 326, 974, 382), color.NRGBA{R: 74, G: 86, B: 96, A: 255})
	fill(image.Rect(718, 404, 974, 470), color.NRGBA{R: 26, G: 149, B: 160, A: 255})
	fill(image.Rect(718, 486, 840, 522), color.NRGBA{R: 211, G: 95, B: 79, A: 255})
	fill(image.Rect(852, 486, 974, 522), color.NRGBA{R: 226, G: 222, B: 207, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}

func writeRealAcceptanceReport(path string, report realAcceptanceReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".acceptance-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err == nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryPath, path)
}
