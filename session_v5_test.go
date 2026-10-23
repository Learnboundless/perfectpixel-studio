package main

import (
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"perfectpixel/internal/config"
)

func TestRealSessionV5MigrationSmoke(t *testing.T) {
	sessionPath := os.Getenv("MOTIONPIXEL_V4_SESSION")
	artifactsPath := os.Getenv("MOTIONPIXEL_V4_ARTIFACTS")
	characterLibraryPath := os.Getenv("MOTIONPIXEL_V4_CHARACTER_LIBRARY")
	if sessionPath == "" || artifactsPath == "" {
		t.Skip("set MOTIONPIXEL_V4_SESSION and MOTIONPIXEL_V4_ARTIFACTS to run real migration smoke")
	}
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("APPDATA", root)
	configRoot := filepath.Join(root, "perfectpixel")
	if err := os.MkdirAll(configRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if characterLibraryPath != "" {
		if err := copyDirectoryForTest(characterLibraryPath, filepath.Join(configRoot, "character-library")); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(artifactsPath, filepath.Join(configRoot, "generation-artifacts")); err != nil {
		t.Skipf("cannot create artifact symlink for migration smoke: %v", err)
	}
	app := NewApp()
	migrated, changed, err := app.migrateSessionToV5(filepath.Join(configRoot, "session.json"), data)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("real v4 session was not migrated")
	}
	var rootJSON map[string]any
	if err := json.Unmarshal(migrated, &rootJSON); err != nil {
		t.Fatal(err)
	}
	if intFromAny(rootJSON["v"]) != sessionSchemaV5 {
		t.Fatalf("migrated schema = %v", rootJSON["v"])
	}
	stats, err := app.GetAnimationVersionStorageStats()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("real migration versions=%d trash=%d bytes=%d", stats.VersionCount, stats.TrashCount, stats.TotalBytes)
}

func copyDirectoryForTest(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0o600)
	})
}

func TestSessionV5MigrationThreeCharactersIsIdempotent(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("APPDATA", root)
	app := NewApp()
	workspaces := make([]any, 0, 3)
	for i, characterID := range []string{"character-a", "character-b", "character-c"} {
		workspaces = append(workspaces, map[string]any{
			"characterId": characterID,
			"character":   map[string]any{"libraryId": characterID, "name": characterID},
			"states": []any{map[string]any{
				"id": "idle-" + characterID, "name": "idle", "status": "done", "frames": 1,
				"items": []any{map[string]any{"id": "frame", "png": versionTestFrame(t, color.NRGBA{R: uint8(40 + i), A: 255}), "selected": true}},
			}},
		})
	}
	payload, _ := json.Marshal(map[string]any{"v": 4, "workspaces": workspaces})
	sessionPath, _ := config.SessionPath()
	if err := atomicWritePrivate(sessionPath, payload); err != nil {
		t.Fatal(err)
	}
	first := app.LoadSession()
	second := app.LoadSession()
	if first == "" || second != first {
		t.Fatal("v5 migration was not stable across repeated loads")
	}
	for _, characterID := range []string{"character-a", "character-b", "character-c"} {
		versions, err := app.ListAnimationVersions(characterID, "idle-"+characterID, false)
		if err != nil || len(versions) != 1 || versions[0].VersionNumber != 1 {
			t.Fatalf("unexpected versions for %s: %#v, %v", characterID, versions, err)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(sessionPath), "session.v4.backup.json")); err != nil {
		t.Fatal("v4 backup was not created")
	}
}

func TestSessionV5ArtifactRecoveryUsesExactTraceAndBoxesAmbiguousResults(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("APPDATA", root)
	artifacts, _ := config.GenerationArtifactsDir()
	exactDir := filepath.Join(artifacts, "generation-request-exact-idle")
	ambiguousDir := filepath.Join(artifacts, "generation-unknown-idle")
	for _, dir := range []string{exactDir, ambiguousDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		img := versionTestFrame(t, color.NRGBA{G: 180, A: 255})
		raw, _ := decodeDataURL(img)
		if err := os.WriteFile(filepath.Join(dir, "final-frame-001.png"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app := NewApp()
	payload, _ := json.Marshal(map[string]any{
		"v": 4,
		"workspaces": []any{map[string]any{
			"characterId": "hero", "character": map[string]any{"libraryId": "hero"},
			"states": []any{map[string]any{
				"id": "idle-state", "name": "idle", "frames": 1, "status": "idle", "items": []any{},
				"trace": map[string]any{"parameters": map[string]any{"artifactDir": exactDir, "requestId": "request-exact"}},
			}},
		}},
	})
	sessionPath, _ := config.SessionPath()
	if err := atomicWritePrivate(sessionPath, payload); err != nil {
		t.Fatal(err)
	}
	if app.LoadSession() == "" {
		t.Fatal("migration returned an empty session")
	}
	versions, err := app.ListAnimationVersions("hero", "idle-state", false)
	if err != nil || len(versions) != 1 || versions[0].Source != "migration_artifact" {
		t.Fatalf("exact artifact was not restored: %#v, %v", versions, err)
	}
	inbox, err := app.ListLegacyAnimationRecovery(false)
	if err != nil || len(inbox) != 1 || canonicalPath(inbox[0].ArtifactDir) != canonicalPath(ambiguousDir) {
		t.Fatalf("ambiguous artifact was not boxed: %#v, %v", inbox, err)
	}
	if app.LoadSession() == "" {
		t.Fatal("second load failed")
	}
	versions, _ = app.ListAnimationVersions("hero", "idle-state", false)
	if len(versions) != 1 {
		t.Fatalf("repeated migration duplicated exact artifact: %#v", versions)
	}
}
