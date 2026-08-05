package main

import (
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

const toolchainAssetRoutePrefix = "/toolchain/assets/"

// NewToolchainAssetHandler serves registered image and video assets by ID. It
// never accepts a filesystem path from the request.
func NewToolchainAssetHandler(store *ToolchainStore) http.Handler {
	return &toolchainAssetHandler{store: store}
}

// ToolchainAssetMiddleware routes controlled media requests before Wails'
// embedded/static asset handler. Unknown toolchain asset subroutes return 404
// instead of falling through to the SPA.
func ToolchainAssetMiddleware(store *ToolchainStore) assetserver.Middleware {
	handler := NewToolchainAssetHandler(store)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, toolchainAssetRoutePrefix) {
				handler.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type toolchainAssetHandler struct {
	store *ToolchainStore
}

func (h *toolchainAssetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, variant, ok := parseToolchainAssetRoute(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if h.store == nil {
		http.Error(w, "asset service unavailable", http.StatusServiceUnavailable)
		return
	}
	asset, err := h.store.GetAsset(id)
	if err != nil {
		serveToolchainAssetError(w, r, err)
		return
	}

	var contentPath string
	switch variant {
	case "content":
		if asset.MediaType != AssetMediaImage && asset.MediaType != AssetMediaVideo {
			http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
			return
		}
		contentPath, err = h.store.ResolveAssetContentPath(id)
	case "thumbnail":
		contentPath, err = h.store.ResolveAssetThumbnailPath(id)
	}
	if err != nil {
		serveToolchainAssetError(w, r, err)
		return
	}

	file, err := os.Open(contentPath)
	if err != nil {
		serveToolchainAssetError(w, r, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		if err == nil {
			err = ErrToolchainNotFound
		}
		serveToolchainAssetError(w, r, err)
		return
	}

	mimeType, ok := safeServedMediaMIME(file, contentPath, asset, variant)
	if !ok {
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return
	}
	if _, err := file.Seek(0, 0); err != nil {
		http.Error(w, "asset service error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Wails' WebView response writer cannot use 304 reliably. These private,
	// no-store routes therefore omit Last-Modified while retaining Range support.
	http.ServeContent(w, r, filepath.Base(contentPath), time.Time{}, file)
}

func parseToolchainAssetRoute(requestPath string) (id, variant string, ok bool) {
	if !strings.HasPrefix(requestPath, toolchainAssetRoutePrefix) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(requestPath, toolchainAssetRoutePrefix), "/")
	if len(parts) != 2 || !validToolchainID(parts[0]) {
		return "", "", false
	}
	if parts[1] != "content" && parts[1] != "thumbnail" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func safeServedMediaMIME(file *os.File, contentPath string, asset ToolAsset, variant string) (string, bool) {
	var header [512]byte
	n, err := file.Read(header[:])
	if err != nil && n == 0 {
		return "", false
	}
	sniffed := normalizedToolchainMIME(http.DetectContentType(header[:n]))
	extension := normalizedToolchainMIME(mime.TypeByExtension(strings.ToLower(filepath.Ext(contentPath))))
	declared := normalizedToolchainMIME(asset.MIMEType)

	expectedPrefix := "image/"
	if variant == "content" && asset.MediaType == AssetMediaVideo {
		expectedPrefix = "video/"
	}
	if sniffed != "" && sniffed != "application/octet-stream" && !strings.HasPrefix(sniffed, expectedPrefix) {
		return "", false
	}
	for _, candidate := range []string{sniffed, declared, extension} {
		if strings.HasPrefix(candidate, expectedPrefix) {
			if candidate == "image/svg+xml" {
				return "", false
			}
			return candidate, true
		}
	}
	return "", false
}

func normalizedToolchainMIME(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return ""
	}
	return strings.ToLower(mediaType)
}

func serveToolchainAssetError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrToolchainNotFound) || errors.Is(err, ErrInvalidToolchainID) || errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	http.Error(w, "asset service error", http.StatusInternalServerError)
}
