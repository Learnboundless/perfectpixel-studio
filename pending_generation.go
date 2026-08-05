package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"perfectpixel/internal/config"
)

// PendingGeneration is a recovery journal entry written immediately after a
// paid video result is downloaded. It remains until session.json contains the
// card update, so a WebView or process crash cannot orphan the new video.
type PendingGeneration struct {
	RequestID    string `json:"requestId"`
	StateID      string `json:"stateId,omitempty"`
	StateName    string `json:"stateName"`
	RawVideoPath string `json:"rawVideoPath"`
	Status       string `json:"status"`
	StartedAt    string `json:"startedAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type pendingGenerationStore struct {
	Items []PendingGeneration `json:"items"`
}

func atomicWritePrivate(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readPendingGenerationStore() (pendingGenerationStore, error) {
	path, err := config.PendingGenerationsPath()
	if err != nil {
		return pendingGenerationStore{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return pendingGenerationStore{}, nil
	}
	if err != nil {
		return pendingGenerationStore{}, err
	}
	var store pendingGenerationStore
	if err := json.Unmarshal(data, &store); err != nil {
		return pendingGenerationStore{}, err
	}
	return store, nil
}

func writePendingGenerationStore(store pendingGenerationStore) error {
	path, err := config.PendingGenerationsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return atomicWritePrivate(path, data)
}

func (a *App) recordPendingGeneration(item PendingGeneration) error {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()

	now := time.Now().Format(time.RFC3339)
	item.RequestID = strings.TrimSpace(item.RequestID)
	if item.RequestID == "" {
		item.RequestID = "generation-" + galleryStamp()
	}
	item.RawVideoPath = filepath.Clean(strings.TrimSpace(item.RawVideoPath))
	if item.Status == "" {
		item.Status = "downloaded"
	}
	if item.StartedAt == "" {
		item.StartedAt = now
	}
	item.UpdatedAt = now

	store, err := readPendingGenerationStore()
	if err != nil {
		return err
	}
	replaced := false
	for i := range store.Items {
		if store.Items[i].RequestID == item.RequestID {
			if item.StartedAt == "" {
				item.StartedAt = store.Items[i].StartedAt
			}
			store.Items[i] = item
			replaced = true
			break
		}
	}
	if !replaced {
		store.Items = append(store.Items, item)
	}
	return writePendingGenerationStore(store)
}

// ListPendingGenerations returns downloaded videos whose card/session commit
// did not finish. The frontend uses this during startup recovery.
func (a *App) ListPendingGenerations() ([]PendingGeneration, error) {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	store, err := readPendingGenerationStore()
	if err != nil {
		return nil, err
	}
	items := append([]PendingGeneration(nil), store.Items...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	return items, nil
}

// AcknowledgeGeneration removes a recovery entry only after the frontend has
// atomically saved the card containing RawVideoPath.
func (a *App) AcknowledgeGeneration(requestID string) error {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil
	}
	store, err := readPendingGenerationStore()
	if err != nil {
		return err
	}
	next := store.Items[:0]
	for _, item := range store.Items {
		if item.RequestID != requestID {
			next = append(next, item)
		}
	}
	store.Items = next
	return writePendingGenerationStore(store)
}
