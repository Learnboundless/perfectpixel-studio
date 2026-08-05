package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"perfectpixel/internal/config"
)

type CharacterLibraryItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	StyleKey    string `json:"styleKey"`
	StyleCustom string `json:"styleCustom,omitempty"`
	ImagePath   string `json:"imagePath,omitempty"`
	Image       string `json:"image,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type SaveCharacterLibraryItemArgs struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	StyleKey    string `json:"styleKey"`
	StyleCustom string `json:"styleCustom,omitempty"`
	Image       string `json:"image,omitempty"`
}

type characterLibraryStore struct {
	Items []CharacterLibraryItem `json:"items"`
}

var characterLibraryMu sync.Mutex

func characterLibraryDir() (string, error) {
	dir, err := config.CharacterLibraryDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func characterLibraryIndexPath() (string, error) {
	dir, err := characterLibraryDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "index.json"), nil
}

func readCharacterLibraryStore() (characterLibraryStore, error) {
	path, err := characterLibraryIndexPath()
	if err != nil {
		return characterLibraryStore{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return characterLibraryStore{}, nil
		}
		return characterLibraryStore{}, err
	}
	var store characterLibraryStore
	if err := json.Unmarshal(data, &store); err != nil {
		return characterLibraryStore{}, fmt.Errorf("角色库索引损坏: %w", err)
	}
	return store, nil
}

func writeCharacterLibraryStore(store characterLibraryStore) error {
	path, err := characterLibraryIndexPath()
	if err != nil {
		return err
	}
	sort.SliceStable(store.Items, func(i, j int) bool {
		return store.Items[i].UpdatedAt > store.Items[j].UpdatedAt
	})
	for i := range store.Items {
		store.Items[i].Image = ""
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return atomicWritePrivate(path, data)
}

func loadCharacterImage(item CharacterLibraryItem) CharacterLibraryItem {
	path := strings.TrimSpace(item.ImagePath)
	if path == "" {
		return item
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return item
	}
	img, err := decodeImage(raw)
	if err != nil {
		return item
	}
	item.Image, _ = pngDataURL(img)
	return item
}

func (a *App) ListCharacterLibrary() ([]CharacterLibraryItem, error) {
	characterLibraryMu.Lock()
	defer characterLibraryMu.Unlock()
	store, err := readCharacterLibraryStore()
	if err != nil {
		return nil, err
	}
	out := make([]CharacterLibraryItem, 0, len(store.Items))
	for _, item := range store.Items {
		out = append(out, loadCharacterImage(item))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out, nil
}

func (a *App) SaveCharacterLibraryItem(args SaveCharacterLibraryItemArgs) (CharacterLibraryItem, error) {
	characterLibraryMu.Lock()
	defer characterLibraryMu.Unlock()
	store, err := readCharacterLibraryStore()
	if err != nil {
		return CharacterLibraryItem{}, err
	}
	id := strings.TrimSpace(args.ID)
	if id == "" {
		id = "character-" + galleryStamp()
	}
	now := time.Now().Format(time.RFC3339)
	index := -1
	for i := range store.Items {
		if store.Items[i].ID == id {
			index = i
			break
		}
	}
	item := CharacterLibraryItem{
		ID:          id,
		Name:        strings.TrimSpace(args.Name),
		Description: strings.TrimSpace(args.Description),
		StyleKey:    strings.TrimSpace(args.StyleKey),
		StyleCustom: strings.TrimSpace(args.StyleCustom),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if item.Name == "" {
		item.Name = "未命名角色"
	}
	if item.StyleKey == "" {
		item.StyleKey = "pixel"
	}
	if index >= 0 {
		item.CreatedAt = store.Items[index].CreatedAt
		item.ImagePath = store.Items[index].ImagePath
	}
	if imageData := strings.TrimSpace(args.Image); imageData != "" {
		raw, err := decodeDataURL(imageData)
		if err != nil {
			return CharacterLibraryItem{}, fmt.Errorf("角色图片格式错误: %w", err)
		}
		img, err := decodeImage(raw)
		if err != nil {
			return CharacterLibraryItem{}, fmt.Errorf("角色图片无法解码: %w", err)
		}
		dir, err := characterLibraryDir()
		if err != nil {
			return CharacterLibraryItem{}, err
		}
		imagePath := filepath.Join(dir, "assets", sanitizeName(id)+".png")
		if err := writePNG(imagePath, img); err != nil {
			return CharacterLibraryItem{}, fmt.Errorf("角色图片保存失败: %w", err)
		}
		item.ImagePath = imagePath
	}
	if index >= 0 {
		store.Items[index] = item
	} else {
		store.Items = append(store.Items, item)
	}
	if err := writeCharacterLibraryStore(store); err != nil {
		return CharacterLibraryItem{}, err
	}
	return loadCharacterImage(item), nil
}

func (a *App) DeleteCharacterLibraryItem(id string) error {
	characterLibraryMu.Lock()
	defer characterLibraryMu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("请选择一个角色库条目。")
	}
	store, err := readCharacterLibraryStore()
	if err != nil {
		return err
	}
	next := make([]CharacterLibraryItem, 0, len(store.Items))
	var deleted *CharacterLibraryItem
	for i := range store.Items {
		if store.Items[i].ID == id {
			copy := store.Items[i]
			deleted = &copy
			continue
		}
		next = append(next, store.Items[i])
	}
	if deleted == nil {
		return errors.New("未找到角色库条目。")
	}
	store.Items = next
	if err := writeCharacterLibraryStore(store); err != nil {
		return err
	}
	if deleted.ImagePath != "" {
		dir, dirErr := characterLibraryDir()
		clean := filepath.Clean(deleted.ImagePath)
		rel, relErr := filepath.Rel(dir, clean)
		if dirErr == nil && relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			_ = os.Remove(clean)
		}
	}
	return nil
}
