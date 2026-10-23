package main

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func versionTestFrame(t *testing.T, c color.NRGBA) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	value, err := pngDataURL(img)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestAnimationVersionLifecycle(t *testing.T) {
	store, err := OpenAnimationVersionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Commit(CommitAnimationVersionArgs{CharacterID: "hero", StateID: "idle", StateName: "idle", Source: "generation", Status: "done", Frames: []AnimationVersionFrame{{ID: "one", PNG: versionTestFrame(t, color.NRGBA{R: 255, A: 255}), Selected: true}}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Commit(CommitAnimationVersionArgs{CharacterID: "hero", StateID: "idle", StateName: "idle", Source: "regeneration", Status: "done", ParentVersionID: first.ID, Frames: []AnimationVersionFrame{{ID: "two", PNG: versionTestFrame(t, color.NRGBA{G: 255, A: 255}), Selected: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if first.VersionNumber != 1 || second.VersionNumber != 2 {
		t.Fatalf("version numbers = %d,%d", first.VersionNumber, second.VersionNumber)
	}
	list, err := store.List("hero", "idle", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || !list[0].Current {
		t.Fatalf("unexpected versions: %#v", list)
	}
	selected, err := store.SetCurrent("hero", "idle", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != first.ID || len(selected.WorkingComposition) != 1 || selected.WorkingComposition[0].PNG == "" {
		t.Fatal("version did not hydrate")
	}
	if _, err := store.move(first.ID, false); err != nil {
		t.Fatal(err)
	}
	list, _ = store.List("hero", "idle", false)
	if len(list) != 1 || !list[0].Current || list[0].ID != second.ID {
		t.Fatalf("delete did not select fallback: %#v", list)
	}
	if _, err := store.move(first.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := store.Purge(first.ID); err == nil {
		t.Fatal("active version was purged")
	}
	if _, err := store.move(first.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := store.Purge(first.ID); err != nil {
		t.Fatal(err)
	}
	stats, err := store.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.VersionCount != 1 || stats.TrashCount != 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestAnimationVersionSourceKeyIsIdempotent(t *testing.T) {
	store, err := OpenAnimationVersionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	args := CommitAnimationVersionArgs{CharacterID: "hero", StateID: "walk", SourceKey: "legacy:hero:walk:v4", Source: "migration", Status: "done", Frames: []AnimationVersionFrame{{PNG: versionTestFrame(t, color.NRGBA{B: 255, A: 255}), Selected: true}}}
	first, err := store.Commit(args)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Commit(args)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("source key duplicated version: %s != %s", first.ID, second.ID)
	}
}

func TestAnimationVersionConcurrentNumbersAreMonotonic(t *testing.T) {
	store, err := OpenAnimationVersionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const count = 12
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := store.Commit(CommitAnimationVersionArgs{
				CharacterID: "hero", StateID: "attack", Source: "generation", Status: "done",
				Frames: []AnimationVersionFrame{{PNG: versionTestFrame(t, color.NRGBA{R: uint8(index), A: 255}), Selected: true}},
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	versions, err := store.List("hero", "attack", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != count {
		t.Fatalf("version count = %d", len(versions))
	}
	seen := map[int]bool{}
	for _, version := range versions {
		seen[version.VersionNumber] = true
	}
	for number := 1; number <= count; number++ {
		if !seen[number] {
			t.Fatalf("missing version number %d: %#v", number, versions)
		}
	}
}

func TestAnimationVersionResourcesUseOpaqueTokens(t *testing.T) {
	store, err := OpenAnimationVersionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sourceDir := t.TempDir()
	videoPath := filepath.Join(sourceDir, "source.mp4")
	if err := os.WriteFile(videoPath, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	framePath := filepath.Join(sourceDir, "raw.png")
	raw, err := decodeDataURL(versionTestFrame(t, color.NRGBA{B: 220, A: 255}))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(framePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	version, err := store.Commit(CommitAnimationVersionArgs{
		CharacterID: "hero", StateID: "idle", Source: "generation", Status: "done",
		Frames:       []AnimationVersionFrame{{PNG: versionTestFrame(t, color.NRGBA{G: 255, A: 255}), Selected: true}},
		RawVideoPath: videoPath, RawExtractedFrames: []string{framePath},
		CandidateFrames: []CandidateFrame{{ID: "candidate", SourcePath: framePath, Source: "raw", Label: "raw"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{version.RawVideoPath, version.RawExtractedFrames[0], version.CandidateFrames[0].SourcePath} {
		if !strings.HasPrefix(token, animationVersionResourcePrefix) || strings.Contains(token, sourceDir) {
			t.Fatalf("resource leaked a path: %q", token)
		}
		resolved, resolveErr := store.ResolveResource(token)
		if resolveErr != nil {
			t.Fatal(resolveErr)
		}
		if _, statErr := os.Stat(resolved); statErr != nil {
			t.Fatal(statErr)
		}
	}
	if _, err := store.ResolveResource("version-resource:../../etc:video:0"); err == nil {
		t.Fatal("forged resource token was accepted")
	}
}
