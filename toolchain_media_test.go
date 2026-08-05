package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestToolchainAssetHandlerServesImageAndHead(t *testing.T) {
	t.Parallel()
	store, err := OpenToolchainStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pngData := toolchainMediaTestPNG(t)
	asset, err := store.CreateAssetFromBytes(ToolAsset{
		Name:         "concept.png",
		MediaType:    AssetMediaImage,
		MIMEType:     "image/png",
		SourceModule: ToolchainModuleImage,
	}, pngData)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewToolchainAssetHandler(store)

	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/toolchain/assets/"+asset.ID+"/content", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", get.Code, get.Body.String())
	}
	if !bytes.Equal(get.Body.Bytes(), pngData) {
		t.Fatal("served image differs from stored content")
	}
	if got := get.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := get.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}

	head := httptest.NewRecorder()
	handler.ServeHTTP(head, httptest.NewRequest(http.MethodHead, "/toolchain/assets/"+asset.ID+"/content", nil))
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("HEAD status/body = %d/%d", head.Code, head.Body.Len())
	}
	if got := head.Header().Get("Content-Length"); got == "" {
		t.Fatal("HEAD response omitted Content-Length")
	}
}

func TestToolchainAssetHandlerSupportsVideoRange(t *testing.T) {
	t.Parallel()
	store, err := OpenToolchainStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	video := append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, bytes.Repeat([]byte{7}, 52)...)
	asset, err := store.CreateAssetFromBytes(ToolAsset{
		Name:         "result.mp4",
		MediaType:    AssetMediaVideo,
		MIMEType:     "video/mp4",
		SourceModule: ToolchainModuleVideo,
	}, video)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/toolchain/assets/"+asset.ID+"/content", nil)
	req.Header.Set("Range", "bytes=4-11")
	recorder := httptest.NewRecorder()
	NewToolchainAssetHandler(store).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("range status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Range"); got != "bytes 4-11/64" {
		t.Fatalf("Content-Range = %q", got)
	}
	if !bytes.Equal(recorder.Body.Bytes(), video[4:12]) {
		t.Fatalf("range body = %v", recorder.Body.Bytes())
	}
}

func TestToolchainAssetHandlerServesRegisteredThumbnail(t *testing.T) {
	t.Parallel()
	store, err := OpenToolchainStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	asset, err := store.CreateAssetFromBytes(ToolAsset{Name: "result.mp4", MediaType: AssetMediaVideo, MIMEType: "video/mp4"}, []byte{0, 0, 0, 1})
	if err != nil {
		t.Fatal(err)
	}
	thumbnail := toolchainMediaTestPNG(t)
	if _, err := store.SetAssetThumbnail(asset.ID, "image/png", thumbnail); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	NewToolchainAssetHandler(store).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/toolchain/assets/"+asset.ID+"/thumbnail", nil))
	if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), thumbnail) {
		t.Fatalf("thumbnail status/body = %d/%d bytes", recorder.Code, recorder.Body.Len())
	}
}

func TestToolchainAssetHandlerRejectsPathsMethodsAndActiveContent(t *testing.T) {
	t.Parallel()
	store, err := OpenToolchainStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	asset, err := store.CreateAssetFromBytes(ToolAsset{Name: "unsafe.svg", MediaType: AssetMediaImage, MIMEType: "image/svg+xml"}, []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"><script/></svg>"))
	if err != nil {
		t.Fatal(err)
	}
	handler := NewToolchainAssetHandler(store)

	unsafePaths := []string{
		"/toolchain/assets/../content",
		"/toolchain/assets//content",
		"/toolchain/assets/not-an-id/content/extra",
		"/toolchain/assets/not-an-id/file",
		"/toolchain/assets/C:%5Cwindows/content",
	}
	for _, requestPath := range unsafePaths {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("path %q returned %d", requestPath, recorder.Code)
		}
	}

	post := httptest.NewRecorder()
	handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/toolchain/assets/"+asset.ID+"/content", nil))
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST status/Allow = %d/%q", post.Code, post.Header().Get("Allow"))
	}

	active := httptest.NewRecorder()
	handler.ServeHTTP(active, httptest.NewRequest(http.MethodGet, "/toolchain/assets/"+asset.ID+"/content", nil))
	if active.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("SVG status = %d", active.Code)
	}
}

func TestToolchainAssetMiddlewareFallsThroughOnlyOutsideRoute(t *testing.T) {
	t.Parallel()
	store, err := OpenToolchainStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	nextCalls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalls++
		w.WriteHeader(http.StatusTeapot)
	})
	handler := ToolchainAssetMiddleware(store)(next)

	outside := httptest.NewRecorder()
	handler.ServeHTTP(outside, httptest.NewRequest(http.MethodGet, "/settings", nil))
	if outside.Code != http.StatusTeapot || nextCalls != 1 {
		t.Fatalf("outside route status/calls = %d/%d", outside.Code, nextCalls)
	}

	inside := httptest.NewRecorder()
	handler.ServeHTTP(inside, httptest.NewRequest(http.MethodGet, "/toolchain/assets/bad/route", nil))
	if inside.Code != http.StatusNotFound || nextCalls != 1 {
		t.Fatalf("inside route status/calls = %d/%d", inside.Code, nextCalls)
	}
}

func toolchainMediaTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(&output)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
