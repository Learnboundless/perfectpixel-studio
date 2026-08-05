package main

import (
	"image"
	"image/color"
	"os"
	"runtime"
	"testing"

	"perfectpixel/internal/sprite"
)

func isolateMotionLibraryConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", dir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("FAL_KEY", "")
	t.Setenv("FAL_API_KEY", "")
	t.Setenv("BYTEPLUS_API_KEY", "")
	t.Setenv("ARK_API_KEY", "")
	t.Setenv("ZETATECHS_API_KEY", "")
	t.Setenv("ZETA_API_KEY", "")
}

func testFrameURL(t *testing.T, c color.NRGBA, dx int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 18; y < 52; y++ {
		for x := 20 + dx; x < 42+dx; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	u, err := pngDataURL(img)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestMotionLibrarySaveListGetDelete(t *testing.T) {
	isolateMotionLibraryConfig(t)
	app := NewApp()
	frame := testFrameURL(t, color.NRGBA{R: 220, G: 50, B: 40, A: 255}, 0)

	item, err := app.SaveMotionLibraryItem(SaveMotionLibraryItemArgs{
		Name:            "walk-ref",
		Action:          "walk forward",
		Frames:          1,
		FPS:             8,
		Loop:            true,
		SourceKind:      "test",
		ProcessedFrames: []string{frame},
		DirectUsable:    true,
	})
	if err != nil {
		t.Fatalf("save motion item: %v", err)
	}
	if item.ID == "" {
		t.Fatal("expected generated motion id")
	}
	items, err := app.ListMotionLibrary()
	if err != nil {
		t.Fatalf("list motion library: %v", err)
	}
	if len(items) != 1 || items[0].Name != "walk-ref" {
		t.Fatalf("unexpected library list: %+v", items)
	}
	got, err := app.GetMotionLibraryItem(item.ID)
	if err != nil {
		t.Fatalf("get motion item: %v", err)
	}
	if got.Action != "walk forward" {
		t.Fatalf("unexpected action: %+v", got)
	}
	if err := app.DeleteMotionLibraryItem(item.ID); err != nil {
		t.Fatalf("delete motion item: %v", err)
	}
	items, err = app.ListMotionLibrary()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty library, got %+v", items)
	}
}

func TestGenerateStateMotionLibraryDirectDoesNotRequireProvider(t *testing.T) {
	isolateMotionLibraryConfig(t)
	app := NewApp()
	frames := []string{testFrameURL(t, color.NRGBA{R: 220, G: 50, B: 40, A: 255}, 0)}
	item, err := app.SaveMotionLibraryItem(SaveMotionLibraryItemArgs{
		Name:            "direct-walk",
		Action:          "walk",
		Frames:          1,
		FPS:             8,
		Loop:            true,
		SourceKind:      "test",
		ProcessedFrames: frames,
		DirectUsable:    true,
	})
	if err != nil {
		t.Fatalf("save motion item: %v", err)
	}

	res, err := app.GenerateState(GenerateStateArgs{
		StyleKey:         "pixel",
		CellSize:         64,
		MotionSourceID:   item.ID,
		MotionSourceMode: motionSourceModeDirect,
		State: sprite.StateSpec{
			Name:   "walk",
			Frames: 1,
			FPS:    8,
			Loop:   true,
			Action: "walk",
		},
	})
	if err != nil {
		t.Fatalf("direct generate failed: %v", err)
	}
	if res.Trace.CalledAI {
		t.Fatal("direct motion library generation must not call AI")
	}
	if len(res.Frames) != 1 || len(res.CandidateFrames) == 0 {
		t.Fatalf("unexpected direct result: frames=%d candidates=%d", len(res.Frames), len(res.CandidateFrames))
	}
	if _, err := os.Stat(mustMotionLibraryIndexPath(t)); err != nil {
		t.Fatalf("motion library index missing: %v", err)
	}
}

func mustMotionLibraryIndexPath(t *testing.T) string {
	t.Helper()
	path, err := motionLibraryIndexPath()
	if err != nil {
		t.Fatal(err)
	}
	return path
}
