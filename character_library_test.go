package main

import (
	"image"
	"image/color"
	"testing"
)

func TestCharacterLibraryPersistsMultipleCharacters(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	app := NewApp()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	img.SetNRGBA(3, 3, color.NRGBA{R: 255, A: 255})
	dataURL, err := pngDataURL(img)
	if err != nil {
		t.Fatal(err)
	}
	first, err := app.SaveCharacterLibraryItem(SaveCharacterLibraryItemArgs{
		Name:     "角色 A",
		StyleKey: "pixel",
		Image:    dataURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.SaveCharacterLibraryItem(SaveCharacterLibraryItemArgs{
		Name:        "角色 B",
		Description: "第二个角色",
		StyleKey:    "chibi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("character ids must be unique")
	}
	items, err := app.ListCharacterLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two persisted characters, got %d", len(items))
	}
	if first.Image == "" {
		t.Fatal("saved character should return a preview image")
	}
	updated, err := app.SaveCharacterLibraryItem(SaveCharacterLibraryItemArgs{
		ID:          first.ID,
		Name:        "角色 A+",
		Description: "更新后",
		StyleKey:    "pixel",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Image == "" || updated.Name != "角色 A+" {
		t.Fatalf("update should preserve image and metadata: %#v", updated)
	}
	if err := app.DeleteCharacterLibraryItem(second.ID); err != nil {
		t.Fatal(err)
	}
	items, err = app.ListCharacterLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != first.ID {
		t.Fatalf("unexpected character list after delete: %#v", items)
	}
}
