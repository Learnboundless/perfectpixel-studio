package main

import (
	"errors"
	"testing"

	"perfectpixel/internal/gen"
)

func TestBuildCasesExpandsDynamicVideoModels(t *testing.T) {
	opt := options{
		models:        "@dynamic-video",
		cases:         "auto",
		requireListed: true,
		seconds:       4,
		fps:           12,
	}
	dynamicModels := []string{
		"gpt-image-2",
		"wan2.6-t2v",
		"wan2.6-i2v",
		"wan2.6-r2v",
		"doubao-seedance-1-0-lite-i2v-250428",
	}

	cases, skipped := buildCases(opt, dynamicModels, fixturePNG(false), fixturePNG(true))
	if len(skipped) != 0 {
		t.Fatalf("unexpected skipped cases: %+v", skipped)
	}
	if len(cases) != 4 {
		t.Fatalf("expected one auto case for each dynamic video model, got %d: %+v", len(cases), cases)
	}

	byModel := map[string]smokeCase{}
	for _, sc := range cases {
		byModel[sc.Model] = sc
	}
	if byModel["wan2.6-t2v"].Workflow != gen.VideoWorkflowTextToVideo {
		t.Fatalf("wan t2v should use text workflow: %+v", byModel["wan2.6-t2v"])
	}
	if byModel["wan2.6-i2v"].Workflow != gen.VideoWorkflowImageToVideo {
		t.Fatalf("wan i2v should use image workflow: %+v", byModel["wan2.6-i2v"])
	}
	if byModel["wan2.6-r2v"].Workflow != gen.VideoWorkflowReferenceToVideo {
		t.Fatalf("wan r2v should use reference video workflow: %+v", byModel["wan2.6-r2v"])
	}
	if byModel["doubao-seedance-1-0-lite-i2v-250428"].Workflow != gen.VideoWorkflowImageToVideo {
		t.Fatalf("seedance i2v should use image workflow: %+v", byModel["doubao-seedance-1-0-lite-i2v-250428"])
	}
}

func TestBuildCasesAutoUsesReferenceVideoForR2VModels(t *testing.T) {
	opt := options{
		models:        "@dynamic-video",
		cases:         "auto",
		requireListed: true,
		seconds:       4,
		fps:           12,
	}

	cases, skipped := buildCases(opt, []string{"wan2.6-r2v"}, fixturePNG(false), fixturePNG(true))
	if len(skipped) != 0 {
		t.Fatalf("unexpected skipped cases: %+v", skipped)
	}
	if len(cases) != 1 {
		t.Fatalf("expected one r2v case, got %+v", cases)
	}
	if cases[0].Workflow != gen.VideoWorkflowReferenceToVideo {
		t.Fatalf("r2v should use reference video workflow: %+v", cases[0])
	}
	if cases[0].MediaSlots[gen.VideoSlotStartImage] != 1 || cases[0].MediaSlots[gen.VideoSlotReferenceVideos] != 1 {
		t.Fatalf("r2v smoke should include a reference video URL slot: %+v", cases[0])
	}
	if cases[0].request.MediaSlots[gen.VideoSlotReferenceVideos][0].URL == "" {
		t.Fatalf("r2v smoke should use a public reference video URL: %+v", cases[0].request.MediaSlots)
	}
}

func TestBuildCasesAutoPrefersStartEndForKeyframeModels(t *testing.T) {
	opt := options{
		models:        "@dynamic-video",
		cases:         "auto",
		requireListed: true,
		seconds:       4,
		fps:           12,
	}

	cases, skipped := buildCases(opt, []string{"wan2.2-kf2v-flash"}, fixturePNG(false), fixturePNG(true))
	if len(skipped) != 0 {
		t.Fatalf("unexpected skipped cases: %+v", skipped)
	}
	if len(cases) != 1 {
		t.Fatalf("expected one keyframe case, got %+v", cases)
	}
	if cases[0].Workflow != gen.VideoWorkflowFirstLastToVideo {
		t.Fatalf("keyframe model should use first-last workflow: %+v", cases[0])
	}
	if cases[0].MediaSlots[gen.VideoSlotStartImage] != 1 || cases[0].MediaSlots[gen.VideoSlotEndImage] != 1 {
		t.Fatalf("keyframe smoke should include start/end URL slots: %+v", cases[0])
	}
}

func TestBuildCasesAutoPrefersMotionTransferForAnimateModels(t *testing.T) {
	opt := options{
		models:        "@dynamic-video",
		cases:         "auto",
		requireListed: true,
		seconds:       4,
		fps:           12,
	}

	cases, skipped := buildCases(opt, []string{"wan2.2-animate-move"}, fixturePNG(false), fixturePNG(true))
	if len(skipped) != 0 {
		t.Fatalf("unexpected skipped cases: %+v", skipped)
	}
	if len(cases) != 1 {
		t.Fatalf("expected one animate case, got %+v", cases)
	}
	if cases[0].Workflow != gen.VideoWorkflowMotionTransfer {
		t.Fatalf("animate model should use motion-transfer workflow: %+v", cases[0])
	}
	if cases[0].MediaSlots[gen.VideoSlotStartImage] != 1 || cases[0].MediaSlots[gen.VideoSlotDriverVideo] != 1 {
		t.Fatalf("animate smoke should include image + driver video URL slots: %+v", cases[0])
	}
	req := cases[0].request
	if req.MediaSlots[gen.VideoSlotStartImage][0].URL == "" || req.MediaSlots[gen.VideoSlotDriverVideo][0].URL == "" {
		t.Fatalf("animate smoke should use public URLs: %+v", req.MediaSlots)
	}
}

func TestBuildCasesSkipsExplicitModelsMissingFromDynamicList(t *testing.T) {
	opt := options{
		models:        "wan2.6-i2v,missing-video-model",
		cases:         "auto",
		requireListed: true,
		seconds:       4,
		fps:           12,
	}

	cases, skipped := buildCases(opt, []string{"wan2.6-i2v"}, fixturePNG(false), fixturePNG(true))
	if len(cases) != 1 || cases[0].Model != "wan2.6-i2v" {
		t.Fatalf("expected only listed model to run, got %+v", cases)
	}
	if len(skipped) != 1 || skipped[0].Case.Model != "missing-video-model" {
		t.Fatalf("expected missing model skip, got %+v", skipped)
	}
}

func TestBuildCasesAutoAddsAudioForS2VModels(t *testing.T) {
	opt := options{
		models:        "@dynamic-video",
		cases:         "auto",
		requireListed: true,
		seconds:       4,
		fps:           12,
		audioURL:      defaultSmokeS2VAudioURL,
	}

	cases, skipped := buildCases(opt, []string{"wan2.2-s2v"}, fixturePNG(false), fixturePNG(true))
	if len(skipped) != 0 {
		t.Fatalf("unexpected skipped cases: %+v", skipped)
	}
	if len(cases) != 1 {
		t.Fatalf("expected one s2v case, got %+v", cases)
	}
	if cases[0].MediaSlots[gen.VideoSlotStartImage] != 1 || cases[0].MediaSlots[gen.VideoSlotReferenceAudios] != 1 {
		t.Fatalf("s2v smoke should include image + audio slots: %+v", cases[0])
	}
	if cases[0].request.MediaSlots[gen.VideoSlotReferenceAudios][0].URL == "" {
		t.Fatalf("s2v smoke should use a public audio URL: %+v", cases[0].request.MediaSlots)
	}
}

func TestIsTransientPollError(t *testing.T) {
	if !isTransientPollError(errors.New(`视频网络错误: Get "https://ent.zetatechs.com/v1/video/generations/task": unexpected EOF`)) {
		t.Fatal("unexpected EOF should be retryable")
	}
	if isTransientPollError(errors.New("ZetaTechs 视频错误（403）：token quota is not enough")) {
		t.Fatal("quota errors must not be retried as transient poll errors")
	}
}
