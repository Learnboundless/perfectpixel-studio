package main

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"testing"

	"perfectpixel/internal/gen"
	"perfectpixel/internal/uiassets"
)

func TestValidateUIAnalysisClipsBoundsAndDefaultsExport(t *testing.T) {
	raw := json.RawMessage(`{
		"regions":[
			{"id":"root","name":"HUD","type":"panel","bounds":{"x":-5,"y":2,"width":30,"height":20},"zIndex":0,"confidence":1.4},
			{"id":"button","parentId":"root","name":"Play","type":"button","bounds":{"x":10,"y":10,"width":50,"height":40},"zIndex":1,"confidence":0.8,"text":"PLAY"}
		],
		"tokens":{"colors":{"accent":"#ffcc00"}},
		"copy":"PLAY",
		"recreationPrompt":"Recreate the visible HUD"
	}`)
	document, err := validateUIAnalysis(raw, 40, 30)
	if err != nil {
		t.Fatal(err)
	}
	if document.Canvas != (uiassets.Size{Width: 40, Height: 30}) {
		t.Fatalf("unexpected canvas: %#v", document.Canvas)
	}
	if len(document.Regions) != 2 {
		t.Fatalf("regions = %d", len(document.Regions))
	}
	root := document.Regions[0]
	if root.Bounds != (UIPixelBounds{X: 0, Y: 2, Width: 25, Height: 20}) {
		t.Fatalf("root bounds were not clipped: %#v", root.Bounds)
	}
	if root.Confidence != 1 || !root.ExportEnabled || root.ParentID != "" {
		t.Fatalf("unexpected root normalization: %#v", root)
	}
	child := document.Regions[1]
	if child.Bounds != (UIPixelBounds{X: 10, Y: 10, Width: 30, Height: 20}) || child.ParentID != root.ID {
		t.Fatalf("unexpected child normalization: %#v", child)
	}
}

func TestValidateUIAnalysisDoesNotAssignMissingParent(t *testing.T) {
	raw := json.RawMessage(`{"regions":[
		{"name":"One","type":"panel","bounds":{"x":0,"y":0,"width":10,"height":10},"confidence":1},
		{"name":"Two","type":"button","bounds":{"x":10,"y":0,"width":10,"height":10},"confidence":1}
	]}`)
	document, err := validateUIAnalysis(raw, 20, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, region := range document.Regions {
		if region.ParentID != "" {
			t.Fatalf("region without parentId acquired parent %q", region.ParentID)
		}
	}
}

func TestValidateUIAnalysisBreaksParentCycles(t *testing.T) {
	raw := json.RawMessage(`{"regions":[
		{"id":"a","parentId":"b","name":"A","type":"panel","bounds":{"x":0,"y":0,"width":10,"height":10},"confidence":1},
		{"id":"b","parentId":"a","name":"B","type":"panel","bounds":{"x":10,"y":0,"width":10,"height":10},"confidence":1}
	]}`)
	document, err := validateUIAnalysis(raw, 20, 10)
	if err != nil {
		t.Fatal(err)
	}
	parents := map[string]string{}
	for _, region := range document.Regions {
		parents[region.ID] = region.ParentID
	}
	for id := range parents {
		seen := map[string]bool{id: true}
		for current := parents[id]; current != ""; current = parents[current] {
			if seen[current] {
				t.Fatalf("parent cycle remains: %#v", parents)
			}
			seen[current] = true
		}
	}
}

func TestValidateUIAnalysisRejectsUnknownFields(t *testing.T) {
	_, err := validateUIAnalysis(json.RawMessage(`{"regions":[{"id":"a","name":"A","type":"panel","bounds":{"x":0,"y":0,"width":1,"height":1},"confidence":1,"invented":true}]}`), 1, 1)
	if err == nil {
		t.Fatal("unknown provider fields should be rejected")
	}
}

func TestClassifyPaidSubmissionErrorMarksUnknownNetworkOutcome(t *testing.T) {
	err := classifyPaidSubmissionError(context.DeadlineExceeded)
	if !errors.Is(err, ErrSubmissionOutcomeUnknown) {
		t.Fatalf("deadline should require review, got %v", err)
	}
	known := errors.New("HTTP 400 invalid prompt")
	if got := classifyPaidSubmissionError(known); !errors.Is(got, known) || errors.Is(got, ErrSubmissionOutcomeUnknown) {
		t.Fatalf("known validation error was reclassified: %v", got)
	}
	if got := classifyPaidSubmissionError(errors.New("provider error (HTTP 503)")); !errors.Is(got, ErrSubmissionOutcomeUnknown) {
		t.Fatalf("5xx submission should require review, got %v", got)
	}
}

func TestVideoDimensionsUsesShortEdgeResolution(t *testing.T) {
	if width, height := videoDimensions("480p", "16:9"); width != 853 || height != 480 {
		t.Fatalf("16:9 dimensions = %dx%d", width, height)
	}
	if width, height := videoDimensions("720p", "9:16"); width != 720 || height != 1280 {
		t.Fatalf("9:16 dimensions = %dx%d", width, height)
	}
}

func TestBuildVideoRequestUsesGenericReferenceImage(t *testing.T) {
	store, err := OpenToolchainStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reference := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for index := 3; index < len(reference.Pix); index += 4 {
		reference.Pix[index] = 255
	}
	raw, err := encodeRasterPNG(reference)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := store.CreateAssetFromBytes(ToolAsset{
		Name:      "reference.png",
		MediaType: AssetMediaImage,
		MIMEType:  "image/png",
	}, raw)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{toolchainStore: store}

	request, err := app.buildVideoRequest(videoJobRequest{
		Mode:              "image",
		ReferenceMode:     gen.VideoReferenceModeNone,
		ReferenceAssetIDs: []string{asset.ID},
	}, "doubao-seedance-2-0-mini-260615")
	if err != nil {
		t.Fatal(err)
	}
	if request.ReferenceMode != gen.VideoReferenceModeIdentityReference || request.Workflow != gen.VideoWorkflowReferenceToVideo {
		t.Fatalf("generic Seedance reference was not routed as identity reference: mode=%q workflow=%q", request.ReferenceMode, request.Workflow)
	}
	if len(request.MediaSlots[gen.VideoSlotReferenceImages]) != 1 {
		t.Fatalf("generic reference image was omitted: %#v", request.MediaSlots)
	}

	request, err = app.buildVideoRequest(videoJobRequest{
		Mode:              "image",
		ReferenceMode:     gen.VideoReferenceModeNone,
		ReferenceAssetIDs: []string{asset.ID},
	}, "kling-v2-master")
	if err != nil {
		t.Fatal(err)
	}
	if request.ReferenceMode != gen.VideoReferenceModeFirstFrame || request.Workflow != gen.VideoWorkflowImageToVideo {
		t.Fatalf("generic Kling reference was not routed as a first frame: mode=%q workflow=%q", request.ReferenceMode, request.Workflow)
	}
	if len(request.MediaSlots[gen.VideoSlotStartImage]) != 1 {
		t.Fatalf("generic first frame was omitted: %#v", request.MediaSlots)
	}
}
