package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestToolchainAssetImportPersistsAndResolvesByID(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "concept.png")
	writeToolchainTestPNG(t, source, 3, 2)

	store, err := OpenToolchainStore(root)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := store.ImportAssetFromPath(source, ToolAsset{
		Name:         "concept.png",
		Usage:        "ui-concept",
		SourceModule: ToolchainModuleUI,
		Tags:         []string{"UI", "ui", "concept"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if asset.ID == "" || asset.MediaType != AssetMediaImage || asset.MIMEType != "image/png" {
		t.Fatalf("unexpected imported metadata: %+v", asset)
	}
	if asset.Width != 3 || asset.Height != 2 || len(asset.Tags) != 2 {
		t.Fatalf("image metadata was not normalized: %+v", asset)
	}
	contentPath, err := store.ResolveAssetContentPath(asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantContent := filepath.Join(root, "assets", asset.ID, "content.png")
	if contentPath != wantContent {
		t.Fatalf("content path = %q, want %q", contentPath, wantContent)
	}
	if _, err := os.Stat(filepath.Join(root, "assets", asset.ID, "asset.json")); err != nil {
		t.Fatalf("asset metadata missing: %v", err)
	}

	thumb := []byte("small-thumbnail")
	updated, err := store.SetAssetThumbnail(asset.ID, "image/png", thumb)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.ThumbnailAvailable {
		t.Fatal("thumbnail flag was not persisted")
	}
	thumbPath, err := store.ResolveAssetThumbnailPath(asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(thumbPath); err != nil || string(got) != string(thumb) {
		t.Fatalf("thumbnail content = %q, err = %v", got, err)
	}

	reopened, err := OpenToolchainStore(root)
	if err != nil {
		t.Fatal(err)
	}
	assets, err := reopened.ListAssets()
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].ID != asset.ID || !assets[0].ThumbnailAvailable {
		t.Fatalf("reopened assets = %+v", assets)
	}
	if _, err := reopened.ResolveAssetContentPath("../outside"); !errors.Is(err, ErrInvalidToolchainID) {
		t.Fatalf("path traversal id error = %v", err)
	}
	if err := reopened.DeleteAsset(asset.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.GetAsset(asset.ID); !errors.Is(err, ErrToolchainNotFound) {
		t.Fatalf("deleted asset error = %v", err)
	}
	assertNoToolchainTempFiles(t, root)
}

func TestIndexLegacyGalleryIsStableAndNonDestructive(t *testing.T) {
	gallery := t.TempDir()
	source := filepath.Join(gallery, "old-sprite.png")
	writeToolchainTestPNG(t, source, 4, 5)
	store, err := OpenToolchainStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.IndexLegacyGallery(gallery)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.IndexLegacyGallery(gallery)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].ID != second[0].ID {
		t.Fatalf("legacy ids are not stable: first=%+v second=%+v", first, second)
	}
	if !first[0].LegacyIndexed || first[0].Width != 4 || first[0].Height != 5 {
		t.Fatalf("unexpected legacy metadata: %+v", first[0])
	}
	resolved, err := store.ResolveAssetContentPath(first[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != source {
		t.Fatalf("legacy content path = %q, want original %q", resolved, source)
	}
	if err := store.DeleteAsset(first[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("deleting legacy index touched source: %v", err)
	}
}

func TestGenerationJobManagerSerializesProviderSubmissions(t *testing.T) {
	store, err := OpenToolchainStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var active atomic.Int32
	var maximum atomic.Int32
	var eventMu sync.Mutex
	var eventTypes []GenerationJobEventType
	manager, err := NewGenerationJobManager(store, GenerationJobManagerOptions{
		MaxConcurrentJobs: 4,
		OnEvent: func(event GenerationJobEvent) {
			eventMu.Lock()
			eventTypes = append(eventTypes, event.Type)
			eventMu.Unlock()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.RegisterHandler(GenerationJobImage, GenerationJobHandler{
		Run: func(ctx context.Context, job GenerationJob, control *GenerationJobControl) error {
			_, err := control.Submit(func(context.Context) (string, error) {
				current := active.Add(1)
				for {
					old := maximum.Load()
					if current <= old || maximum.CompareAndSwap(old, current) {
						break
					}
				}
				time.Sleep(40 * time.Millisecond)
				active.Add(-1)
				return "remote-" + job.ID, nil
			})
			if err != nil {
				return err
			}
			_, err = control.SetPolling("polling", 0.7)
			return err
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	first, err := manager.CreateJob(GenerationJob{
		Kind:     GenerationJobImage,
		Provider: "zetatechs",
		Request: map[string]any{
			"prompt": "a game HUD",
			"apiKey": "must-not-persist",
			"nested": map[string]any{"Authorization": "Bearer secret"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.CreateJob(GenerationJob{Kind: GenerationJobImage, Provider: "zetatechs"})
	if err != nil {
		t.Fatal(err)
	}
	waitForJobStatus(t, manager, first.ID, GenerationJobCompleted)
	waitForJobStatus(t, manager, second.ID, GenerationJobCompleted)
	if maximum.Load() != 1 {
		t.Fatalf("maximum concurrent paid submissions = %d, want 1", maximum.Load())
	}
	persisted, err := store.GetGenerationJob(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Request["apiKey"] != "[redacted]" {
		t.Fatalf("api key was persisted: %#v", persisted.Request)
	}
	nested := persisted.Request["nested"].(map[string]any)
	if nested["Authorization"] != "[redacted]" {
		t.Fatalf("authorization was persisted: %#v", nested)
	}
	eventMu.Lock()
	defer eventMu.Unlock()
	if len(eventTypes) < 4 || eventTypes[0] != GenerationJobEventCreated {
		t.Fatalf("unexpected events: %v", eventTypes)
	}
}

func TestGenerationJobManagerAllowsDifferentProvidersConcurrently(t *testing.T) {
	store, err := OpenToolchainStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewGenerationJobManager(store, GenerationJobManagerOptions{MaxConcurrentJobs: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	entered := make(chan string, 2)
	release := make(chan struct{})
	if err := manager.RegisterHandler(GenerationJobImage, GenerationJobHandler{
		Run: func(_ context.Context, job GenerationJob, control *GenerationJobControl) error {
			_, err := control.Submit(func(context.Context) (string, error) {
				entered <- job.Provider
				<-release
				return "remote-" + job.Provider, nil
			})
			return err
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, err := manager.CreateJob(GenerationJob{Kind: GenerationJobImage, Provider: "zetatechs"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.CreateJob(GenerationJob{Kind: GenerationJobImage, Provider: "volcengine"})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for range 2 {
		select {
		case provider := <-entered:
			seen[provider] = true
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatalf("different providers were globally serialized; entered=%v", seen)
		}
	}
	close(release)
	if !seen["zetatechs"] || !seen["volcengine"] {
		t.Fatalf("unexpected providers entered: %v", seen)
	}
	waitForJobStatus(t, manager, first.ID, GenerationJobCompleted)
	waitForJobStatus(t, manager, second.ID, GenerationJobCompleted)
}

func TestUnknownSubmissionOutcomeRequiresReview(t *testing.T) {
	store, err := OpenToolchainStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewGenerationJobManager(store, GenerationJobManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.RegisterHandler(GenerationJobVideo, GenerationJobHandler{
		Run: func(_ context.Context, _ GenerationJob, control *GenerationJobControl) error {
			_, err := control.Submit(func(context.Context) (string, error) {
				return "", fmt.Errorf("%w: connection dropped after upload", ErrSubmissionOutcomeUnknown)
			})
			return err
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	job, err := manager.CreateJob(GenerationJob{Kind: GenerationJobVideo, Provider: "volcengine"})
	if err != nil {
		t.Fatal(err)
	}
	needsReview := waitForJobStatus(t, manager, job.ID, GenerationJobNeedsReview)
	if !needsReview.UpstreamMayContinue || !strings.Contains(needsReview.Error, "connection dropped") {
		t.Fatalf("unexpected needs-review job: %+v", needsReview)
	}
}

func TestGenerationJobStartupRecoveryNeverResubmits(t *testing.T) {
	store, err := OpenToolchainStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := toolchainTimestamp(time.Now())
	remote := GenerationJob{ID: "job-remote", Kind: GenerationJobVideo, Status: GenerationJobPolling, Provider: "volcengine", ProviderTaskID: "task-123", CreatedAt: now, UpdatedAt: now, StartedAt: now, Attempt: 1}
	unknown := GenerationJob{ID: "job-unknown", Kind: GenerationJobVideo, Status: GenerationJobSubmitting, Provider: "volcengine", CreatedAt: now, UpdatedAt: now, StartedAt: now, Attempt: 1}
	queued := GenerationJob{ID: "job-queued", Kind: GenerationJobVideo, Status: GenerationJobQueued, Provider: "volcengine", CreatedAt: now, UpdatedAt: now, Attempt: 1}
	for _, job := range []GenerationJob{remote, unknown, queued} {
		if err := store.SaveGenerationJob(job); err != nil {
			t.Fatal(err)
		}
	}

	var resumed atomic.Int32
	var submitted atomic.Int32
	manager, err := NewGenerationJobManager(store, GenerationJobManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.RegisterHandler(GenerationJobVideo, GenerationJobHandler{
		Run: func(context.Context, GenerationJob, *GenerationJobControl) error {
			submitted.Add(1)
			return nil
		},
		Resume: func(_ context.Context, job GenerationJob, control *GenerationJobControl) error {
			if job.ProviderTaskID != "task-123" {
				return errors.New("resume lost provider task id")
			}
			resumed.Add(1)
			_, err := control.SetPolling("recovered", 0.9)
			return err
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForJobStatus(t, manager, remote.ID, GenerationJobCompleted)
	unknownAfter := waitForJobStatus(t, manager, unknown.ID, GenerationJobNeedsReview)
	queuedAfter, err := manager.GetJob(queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Load() != 1 || submitted.Load() != 0 {
		t.Fatalf("resumed=%d submitted=%d", resumed.Load(), submitted.Load())
	}
	if !unknownAfter.UpstreamMayContinue || queuedAfter.Status != GenerationJobQueued {
		t.Fatalf("recovery states: unknown=%+v queued=%+v", unknownAfter, queuedAfter)
	}
}

func TestGenerationJobCancelWithoutRemoteCancelStopsOnlyWaiting(t *testing.T) {
	store, err := OpenToolchainStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewGenerationJobManager(store, GenerationJobManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	submitted := make(chan struct{})
	if err := manager.RegisterHandler(GenerationJobVideo, GenerationJobHandler{
		Run: func(ctx context.Context, _ GenerationJob, control *GenerationJobControl) error {
			if _, err := control.Submit(func(context.Context) (string, error) { return "remote-video", nil }); err != nil {
				return err
			}
			close(submitted)
			<-ctx.Done()
			return ctx.Err()
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	job, err := manager.CreateJob(GenerationJob{Kind: GenerationJobVideo, Provider: "volcengine"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-submitted:
	case <-time.After(2 * time.Second):
		t.Fatal("provider submission did not finish")
	}
	stopped, err := manager.CancelJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Status != GenerationJobStopped || !stopped.UpstreamMayContinue || stopped.ProviderTaskID != "remote-video" {
		t.Fatalf("unexpected stopped job: %+v", stopped)
	}
}

func TestGenerationJobRetryUsesRegisteredClone(t *testing.T) {
	store, err := OpenToolchainStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewGenerationJobManager(store, GenerationJobManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	var clones atomic.Int32
	if err := manager.RegisterHandler(GenerationJobPromptOptimization, GenerationJobHandler{
		Run: func(context.Context, GenerationJob, *GenerationJobControl) error {
			return errors.New("provider unavailable")
		},
		Clone: func(original GenerationJob) (GenerationJob, error) {
			clones.Add(1)
			original.Request["retry"] = true
			return original, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	original, err := manager.CreateJob(GenerationJob{Kind: GenerationJobPromptOptimization, Request: map[string]any{"prompt": "walk"}})
	if err != nil {
		t.Fatal(err)
	}
	waitForJobStatus(t, manager, original.ID, GenerationJobFailed)
	retry, err := manager.RetryJob(original.ID)
	if err != nil {
		t.Fatal(err)
	}
	waitForJobStatus(t, manager, retry.ID, GenerationJobFailed)
	if clones.Load() != 1 || retry.RetryOfJobID != original.ID || retry.Attempt != 2 || retry.Request["retry"] != true {
		t.Fatalf("unexpected retry: %+v, clone count=%d", retry, clones.Load())
	}
}

func TestGenerationJobManagerClosePreservesResumableStatus(t *testing.T) {
	store, err := OpenToolchainStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewGenerationJobManager(store, GenerationJobManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	submitted := make(chan struct{})
	if err := manager.RegisterHandler(GenerationJobVideo, GenerationJobHandler{
		Run: func(ctx context.Context, _ GenerationJob, control *GenerationJobControl) error {
			if _, err := control.Submit(func(context.Context) (string, error) { return "resume-me", nil }); err != nil {
				return err
			}
			close(submitted)
			<-ctx.Done()
			return ctx.Err()
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	job, err := manager.CreateJob(GenerationJob{Kind: GenerationJobVideo, Provider: "volcengine"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-submitted:
	case <-time.After(2 * time.Second):
		t.Fatal("provider submission did not finish")
	}
	manager.Close()
	persisted, err := store.GetGenerationJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != GenerationJobRunning || persisted.ProviderTaskID != "resume-me" {
		t.Fatalf("shutdown destroyed resumable state: %+v", persisted)
	}
}

func TestGenerationJobStateMachineRejectsTerminalRewrite(t *testing.T) {
	job := GenerationJob{Status: GenerationJobCompleted}
	if _, err := applyGenerationJobUpdate(job, GenerationJobUpdate{Status: GenerationJobRunning}); !errors.Is(err, ErrInvalidJobTransition) {
		t.Fatalf("terminal transition error = %v", err)
	}
	job = GenerationJob{Status: GenerationJobRunning}
	updated, err := applyGenerationJobUpdate(job, GenerationJobUpdate{Progress: 3, SetProgress: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Progress != 1 {
		t.Fatalf("clamped progress = %v", updated.Progress)
	}
}

func waitForJobStatus(t *testing.T, manager *GenerationJobManager, id string, status GenerationJobStatus) GenerationJob {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		job, err := manager.GetJob(id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status == status {
			return job
		}
		if job.Status.Terminal() && job.Status != status {
			t.Fatalf("job %s reached %s, want %s: %s", id, job.Status, status, job.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, _ := manager.GetJob(id)
	t.Fatalf("timed out waiting for job %s status %s; last=%+v", id, status, job)
	return GenerationJob{}
}

func writeToolchainTestPNG(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(30 + x), G: uint8(50 + y), B: 90, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertNoToolchainTempFiles(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("temporary file was left behind: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
