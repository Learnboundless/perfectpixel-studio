package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

const (
	toolchainDirName       = "toolchain"
	toolchainAssetMetaName = "asset.json"
	toolchainMaxListItems  = 10000
)

var (
	ErrToolchainNotFound  = errors.New("toolchain record not found")
	ErrInvalidToolchainID = errors.New("invalid toolchain id")
)

// ToolchainStore persists assets and jobs below one private root. Use
// NewToolchainStore in the app and OpenToolchainStore in tests or migrations.
type ToolchainStore struct {
	root string
	mu   sync.RWMutex
	now  func() time.Time
}

type storedToolAsset struct {
	SchemaVersion int `json:"schemaVersion"`
	ToolAsset
	ContentFile   string `json:"contentFile,omitempty"`
	ThumbnailFile string `json:"thumbnailFile,omitempty"`
	ExternalPath  string `json:"externalPath,omitempty"`
}

// DefaultToolchainRoot returns the private root used by the four-module
// toolchain without changing the legacy animation session location.
func DefaultToolchainRoot() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "perfectpixel", toolchainDirName), nil
}

func NewToolchainStore() (*ToolchainStore, error) {
	root, err := DefaultToolchainRoot()
	if err != nil {
		return nil, err
	}
	return OpenToolchainStore(root)
}

// OpenToolchainStore opens an explicit toolchain root and creates its private
// asset and job directories. It is safe to call repeatedly.
func OpenToolchainStore(root string) (*ToolchainStore, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return nil, errors.New("toolchain root is empty")
	}
	s := &ToolchainStore{root: root, now: time.Now}
	if err := s.ensureLayout(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *ToolchainStore) Root() string { return s.root }

func (s *ToolchainStore) ensureLayout() error {
	for _, dir := range []string{s.root, s.assetsDir(), s.jobsDir()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create toolchain directory: %w", err)
		}
	}
	return nil
}

func (s *ToolchainStore) assetsDir() string { return filepath.Join(s.root, "assets") }
func (s *ToolchainStore) jobsDir() string   { return filepath.Join(s.root, "jobs") }

func (s *ToolchainStore) assetDir(id string) (string, error) {
	if !validToolchainID(id) {
		return "", ErrInvalidToolchainID
	}
	return filepath.Join(s.assetsDir(), id), nil
}

func (s *ToolchainStore) jobPath(id string) (string, error) {
	if !validToolchainID(id) {
		return "", ErrInvalidToolchainID
	}
	return filepath.Join(s.jobsDir(), id+".json"), nil
}

func validToolchainID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for i, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		if i > 0 && (r == '-' || r == '_' || r == '.') {
			continue
		}
		return false
	}
	return id != "." && id != ".."
}

func newToolchainID(prefix string) string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return prefix + "-" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func toolchainTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// ImportAssetFromPath copies source into managed storage and writes asset.json
// only after the content rename succeeds.
func (s *ToolchainStore) ImportAssetFromPath(source string, metadata ToolAsset) (ToolAsset, error) {
	source = filepath.Clean(strings.TrimSpace(source))
	info, err := os.Stat(source)
	if err != nil {
		return ToolAsset{}, fmt.Errorf("inspect asset source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return ToolAsset{}, errors.New("asset source must be a regular file")
	}
	f, err := os.Open(source)
	if err != nil {
		return ToolAsset{}, fmt.Errorf("open asset source: %w", err)
	}
	defer f.Close()
	if metadata.Name == "" {
		metadata.Name = filepath.Base(source)
	}
	return s.importAssetReader(metadata, f, info.Size(), filepath.Ext(source))
}

// CreateAssetFromBytes stores generated content without exposing its path over
// the Wails bridge. metadata.Name supplies the preferred file extension.
func (s *ToolchainStore) CreateAssetFromBytes(metadata ToolAsset, content []byte) (ToolAsset, error) {
	return s.importAssetReader(metadata, bytes.NewReader(content), int64(len(content)), filepath.Ext(metadata.Name))
}

func (s *ToolchainStore) importAssetReader(metadata ToolAsset, content io.Reader, sizeHint int64, extension string) (ToolAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if metadata.ID == "" {
		metadata.ID = newToolchainID("asset")
	}
	dir, err := s.assetDir(metadata.ID)
	if err != nil {
		return ToolAsset{}, err
	}
	if _, err := os.Stat(dir); err == nil {
		return ToolAsset{}, fmt.Errorf("asset %q already exists", metadata.ID)
	} else if !os.IsNotExist(err) {
		return ToolAsset{}, err
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return ToolAsset{}, fmt.Errorf("create asset directory: %w", err)
	}
	removeDir := true
	defer func() {
		if removeDir {
			_ = os.RemoveAll(dir)
		}
	}()

	extension = safeAssetExtension(extension)
	contentFile := "content" + extension
	contentPath := filepath.Join(dir, contentFile)
	written, prefix, err := atomicCopyPrivate(contentPath, content)
	if err != nil {
		return ToolAsset{}, err
	}
	if sizeHint >= 0 && written != sizeHint {
		return ToolAsset{}, fmt.Errorf("asset size changed while importing: expected %d bytes, copied %d", sizeHint, written)
	}
	metadata.SizeBytes = written
	if metadata.Name == "" {
		metadata.Name = metadata.ID + extension
	}
	if metadata.MIMEType == "" {
		metadata.MIMEType = detectAssetMIME(extension, prefix)
	}
	if metadata.MediaType == "" {
		metadata.MediaType = mediaTypeForMIME(metadata.MIMEType)
	}
	fillImageDimensions(contentPath, &metadata)
	now := toolchainTimestamp(s.now())
	if metadata.CreatedAt == "" {
		metadata.CreatedAt = now
	}
	metadata.UpdatedAt = now
	metadata.Tags = normalizedStrings(metadata.Tags)

	stored := storedToolAsset{SchemaVersion: ToolchainSchemaVersion, ToolAsset: metadata, ContentFile: contentFile}
	if err := writeJSONAtomic(filepath.Join(dir, toolchainAssetMetaName), stored); err != nil {
		return ToolAsset{}, err
	}
	removeDir = false
	return cloneToolAsset(metadata), nil
}

// SetAssetThumbnail stores or replaces an optional thumbnail beside the asset.
func (s *ToolchainStore) SetAssetThumbnail(id, mimeType string, content []byte) (ToolAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, dir, err := s.readStoredAssetLocked(id)
	if err != nil {
		return ToolAsset{}, err
	}
	ext := safeAssetExtension(extensionForMIME(mimeType))
	if ext == "" {
		ext = ".bin"
	}
	name := "thumbnail" + ext
	if err := atomicWritePrivateFile(filepath.Join(dir, name), content); err != nil {
		return ToolAsset{}, err
	}
	if stored.ThumbnailFile != "" && stored.ThumbnailFile != name {
		_ = os.Remove(filepath.Join(dir, stored.ThumbnailFile))
	}
	stored.ThumbnailFile = name
	stored.ThumbnailAvailable = true
	stored.UpdatedAt = toolchainTimestamp(s.now())
	if err := writeJSONAtomic(filepath.Join(dir, toolchainAssetMetaName), stored); err != nil {
		return ToolAsset{}, err
	}
	return cloneToolAsset(stored.ToolAsset), nil
}

func (s *ToolchainStore) GetAsset(id string) (ToolAsset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stored, _, err := s.readStoredAssetLocked(id)
	if err != nil {
		return ToolAsset{}, err
	}
	return cloneToolAsset(stored.ToolAsset), nil
}

func (s *ToolchainStore) ListAssets() ([]ToolAsset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.assetsDir())
	if err != nil {
		return nil, err
	}
	assets := make([]ToolAsset, 0, min(len(entries), toolchainMaxListItems))
	for _, entry := range entries {
		if !entry.IsDir() || !validToolchainID(entry.Name()) {
			continue
		}
		stored, _, err := s.readStoredAssetLocked(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read asset %q: %w", entry.Name(), err)
		}
		assets = append(assets, cloneToolAsset(stored.ToolAsset))
		if len(assets) >= toolchainMaxListItems {
			break
		}
	}
	sort.SliceStable(assets, func(i, j int) bool { return assets[i].CreatedAt > assets[j].CreatedAt })
	return assets, nil
}

// DeleteAsset deletes managed content and metadata. For a legacy index it only
// removes the index directory; the external gallery file is never touched.
func (s *ToolchainStore) DeleteAsset(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.assetDir(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, toolchainAssetMetaName)); err != nil {
		if os.IsNotExist(err) {
			return ErrToolchainNotFound
		}
		return err
	}
	return os.RemoveAll(dir)
}

// ResolveAssetContentPath resolves only registered asset IDs. Callers should
// never accept an arbitrary path from the frontend as a substitute.
func (s *ToolchainStore) ResolveAssetContentPath(id string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stored, dir, err := s.readStoredAssetLocked(id)
	if err != nil {
		return "", err
	}
	var path string
	if stored.ExternalPath != "" {
		path = filepath.Clean(stored.ExternalPath)
	} else {
		if !validStoredFilename(stored.ContentFile) {
			return "", errors.New("asset content filename is invalid")
		}
		path = filepath.Join(dir, stored.ContentFile)
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrToolchainNotFound
		}
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("asset content is not a regular file")
	}
	return path, nil
}

func (s *ToolchainStore) ResolveAssetThumbnailPath(id string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stored, dir, err := s.readStoredAssetLocked(id)
	if err != nil {
		return "", err
	}
	if !validStoredFilename(stored.ThumbnailFile) {
		return "", ErrToolchainNotFound
	}
	path := filepath.Join(dir, stored.ThumbnailFile)
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		if err == nil || os.IsNotExist(err) {
			return "", ErrToolchainNotFound
		}
		return "", err
	}
	return path, nil
}

// IndexLegacyGallery creates deterministic metadata records pointing at legacy
// gallery files. It copies, moves, renames, and deletes none of those files.
func (s *ToolchainStore) IndexLegacyGallery(galleryDir string) ([]ToolAsset, error) {
	abs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(galleryDir)))
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if os.IsNotExist(err) {
		return []ToolAsset{}, nil
	}
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	indexed := make([]ToolAsset, 0, min(len(entries), toolchainMaxListItems))
	for _, entry := range entries {
		if entry.IsDir() || len(indexed) >= toolchainMaxListItems {
			continue
		}
		path := filepath.Join(abs, entry.Name())
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		ext := safeAssetExtension(filepath.Ext(entry.Name()))
		mimeType := detectAssetMIME(ext, nil)
		mediaType := mediaTypeForMIME(mimeType)
		if mediaType != AssetMediaImage && mediaType != AssetMediaVideo {
			continue
		}
		id := legacyAssetID(path)
		dir, _ := s.assetDir(id)
		stored, _, readErr := s.readStoredAssetLocked(id)
		if readErr != nil && !errors.Is(readErr, ErrToolchainNotFound) {
			return nil, readErr
		}
		now := toolchainTimestamp(s.now())
		asset := stored.ToolAsset
		if asset.CreatedAt == "" {
			asset.CreatedAt = toolchainTimestamp(info.ModTime())
		}
		asset.ID = id
		asset.Name = entry.Name()
		asset.MediaType = mediaType
		asset.MIMEType = mimeType
		asset.Usage = "gallery"
		asset.SizeBytes = info.Size()
		asset.SourceModule = ToolchainModuleAnimation
		asset.LegacyIndexed = true
		asset.Tags = normalizedStrings(append(asset.Tags, "legacy-gallery"))
		asset.UpdatedAt = now
		fillImageDimensions(path, &asset)
		stored = storedToolAsset{
			SchemaVersion: ToolchainSchemaVersion,
			ToolAsset:     asset,
			ExternalPath:  path,
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
		if err := writeJSONAtomic(filepath.Join(dir, toolchainAssetMetaName), stored); err != nil {
			return nil, err
		}
		indexed = append(indexed, cloneToolAsset(asset))
	}
	sort.SliceStable(indexed, func(i, j int) bool { return indexed[i].CreatedAt > indexed[j].CreatedAt })
	return indexed, nil
}

func DefaultLegacyGalleryDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "perfectpixel", "gallery"), nil
}

func (s *ToolchainStore) IndexDefaultLegacyGallery() ([]ToolAsset, error) {
	dir, err := DefaultLegacyGalleryDir()
	if err != nil {
		return nil, err
	}
	return s.IndexLegacyGallery(dir)
}

func (s *ToolchainStore) readStoredAssetLocked(id string) (storedToolAsset, string, error) {
	dir, err := s.assetDir(id)
	if err != nil {
		return storedToolAsset{}, "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, toolchainAssetMetaName))
	if err != nil {
		if os.IsNotExist(err) {
			return storedToolAsset{}, "", ErrToolchainNotFound
		}
		return storedToolAsset{}, "", err
	}
	var stored storedToolAsset
	if err := json.Unmarshal(data, &stored); err != nil {
		return storedToolAsset{}, "", err
	}
	if stored.ID != id {
		return storedToolAsset{}, "", errors.New("asset metadata id does not match directory")
	}
	return stored, dir, nil
}

// SaveGenerationJob is the durable primitive used by GenerationJobManager.
func (s *ToolchainStore) SaveGenerationJob(job GenerationJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validToolchainID(job.ID) {
		return ErrInvalidToolchainID
	}
	request, err := normalizeJobRequest(job.Request)
	if err != nil {
		return err
	}
	job.Request = request
	path, _ := s.jobPath(job.ID)
	wrapper := struct {
		SchemaVersion int `json:"schemaVersion"`
		GenerationJob
	}{SchemaVersion: ToolchainSchemaVersion, GenerationJob: cloneGenerationJob(job)}
	return writeJSONAtomic(path, wrapper)
}

func (s *ToolchainStore) GetGenerationJob(id string) (GenerationJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readGenerationJobLocked(id)
}

func (s *ToolchainStore) ListGenerationJobs() ([]GenerationJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.jobsDir())
	if err != nil {
		return nil, err
	}
	jobs := make([]GenerationJob, 0, min(len(entries), toolchainMaxListItems))
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if !validToolchainID(id) {
			continue
		}
		job, err := s.readGenerationJobLocked(id)
		if err != nil {
			return nil, fmt.Errorf("read job %q: %w", id, err)
		}
		jobs = append(jobs, job)
		if len(jobs) >= toolchainMaxListItems {
			break
		}
	}
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].CreatedAt > jobs[j].CreatedAt })
	return jobs, nil
}

func (s *ToolchainStore) DeleteGenerationJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.jobPath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrToolchainNotFound
		}
		return err
	}
	return nil
}

func (s *ToolchainStore) readGenerationJobLocked(id string) (GenerationJob, error) {
	path, err := s.jobPath(id)
	if err != nil {
		return GenerationJob{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return GenerationJob{}, ErrToolchainNotFound
		}
		return GenerationJob{}, err
	}
	var wrapper struct {
		SchemaVersion int `json:"schemaVersion"`
		GenerationJob
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return GenerationJob{}, err
	}
	if wrapper.ID != id {
		return GenerationJob{}, errors.New("job metadata id does not match filename")
	}
	return cloneGenerationJob(wrapper.GenerationJob), nil
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWritePrivateFile(path, data)
}

func atomicWritePrivateFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".toolchain-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func atomicCopyPrivate(path string, source io.Reader) (int64, []byte, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return 0, nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".content-*.tmp")
	if err != nil {
		return 0, nil, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return 0, nil, err
	}
	peek := &prefixWriter{limit: 512}
	written, err := io.Copy(io.MultiWriter(tmp, peek), source)
	if err != nil {
		_ = tmp.Close()
		return 0, nil, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return 0, nil, err
	}
	if err := tmp.Close(); err != nil {
		return 0, nil, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return 0, nil, err
	}
	return written, peek.data, nil
}

type prefixWriter struct {
	limit int
	data  []byte
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	if len(w.data) < w.limit {
		remaining := w.limit - len(w.data)
		w.data = append(w.data, p[:min(len(p), remaining)]...)
	}
	return len(p), nil
}

func safeAssetExtension(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if len(ext) < 2 || len(ext) > 12 || ext[0] != '.' {
		return ""
	}
	for _, r := range ext[1:] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return ""
		}
	}
	return ext
}

func validStoredFilename(name string) bool {
	return name != "" && filepath.Base(name) == name && name != "." && name != ".."
}

func detectAssetMIME(ext string, prefix []byte) string {
	if fromExt := mime.TypeByExtension(ext); fromExt != "" {
		if i := strings.IndexByte(fromExt, ';'); i >= 0 {
			fromExt = fromExt[:i]
		}
		return fromExt
	}
	if len(prefix) > 0 {
		return http.DetectContentType(prefix)
	}
	return "application/octet-stream"
}

func extensionForMIME(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0])) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		exts, _ := mime.ExtensionsByType(mimeType)
		if len(exts) > 0 {
			return exts[0]
		}
		return ""
	}
}

func mediaTypeForMIME(mimeType string) AssetMediaType {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return AssetMediaImage
	case strings.HasPrefix(mimeType, "video/"):
		return AssetMediaVideo
	case strings.HasPrefix(mimeType, "audio/"):
		return AssetMediaAudio
	case strings.Contains(mimeType, "json"):
		return AssetMediaJSON
	case strings.HasPrefix(mimeType, "text/"):
		return AssetMediaText
	case strings.Contains(mimeType, "zip") || strings.Contains(mimeType, "archive"):
		return AssetMediaArchive
	default:
		return ""
	}
}

func fillImageDimensions(path string, asset *ToolAsset) {
	if asset == nil || asset.MediaType != AssetMediaImage || (asset.Width > 0 && asset.Height > 0) {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	config, _, err := image.DecodeConfig(f)
	if err == nil {
		asset.Width = config.Width
		asset.Height = config.Height
	}
}

func legacyAssetID(path string) string {
	canonical := filepath.Clean(path)
	if filepath.Separator == '\\' {
		canonical = strings.ToLower(canonical)
	}
	hash := sha256.Sum256([]byte(canonical))
	return "legacy-" + hex.EncodeToString(hash[:12])
}

func normalizedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func cloneToolAsset(asset ToolAsset) ToolAsset {
	asset.Tags = append([]string(nil), asset.Tags...)
	return asset
}

func cloneGenerationJob(job GenerationJob) GenerationJob {
	job.InputAssetIDs = append([]string(nil), job.InputAssetIDs...)
	job.OutputAssetIDs = append([]string(nil), job.OutputAssetIDs...)
	job.ProviderResponseSummary = append(json.RawMessage(nil), job.ProviderResponseSummary...)
	if job.Request != nil {
		data, err := json.Marshal(job.Request)
		if err == nil {
			var cloned map[string]any
			if json.Unmarshal(data, &cloned) == nil {
				job.Request = cloned
			}
		}
	}
	return job
}
