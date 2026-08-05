package safemedia

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type staticResolver map[string][]string

func (r staticResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	values, ok := r[host]
	if !ok {
		return nil, fmt.Errorf("unknown test host %q", host)
	}
	result := make([]net.IPAddr, 0, len(values))
	for _, value := range values {
		result = append(result, net.IPAddr{IP: net.ParseIP(value)})
	}
	return result, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestIsPublicIP(t *testing.T) {
	t.Parallel()
	blocked := []string{
		"0.0.0.0",
		"10.0.0.1",
		"100.64.0.1",
		"127.0.0.1",
		"169.254.169.254",
		"172.16.0.1",
		"192.168.1.1",
		"198.18.0.1",
		"192.0.2.1",
		"224.0.0.1",
		"255.255.255.255",
		"::1",
		"::ffff:127.0.0.1",
		"fc00::1",
		"fe80::1",
		"2001:db8::1",
		"3fff::1",
		"5f00::1",
		"fec0::1",
	}
	for _, value := range blocked {
		if IsPublicIP(net.ParseIP(value)) {
			t.Errorf("expected %s to be blocked", value)
		}
	}
	for _, value := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !IsPublicIP(net.ParseIP(value)) {
			t.Errorf("expected %s to be public", value)
		}
	}
}

func TestValidatePublicHTTPSURLChecksEveryResolvedAddress(t *testing.T) {
	t.Parallel()
	resolver := staticResolver{
		"public.example": {"93.184.216.34"},
		"mixed.example":  {"93.184.216.34", "127.0.0.1"},
	}
	parsed, err := ValidatePublicHTTPSURL(context.Background(), "https://public.example/output.png?signature=secret", resolver)
	if err != nil {
		t.Fatalf("public URL rejected: %v", err)
	}
	if parsed.Hostname() != "public.example" {
		t.Fatalf("unexpected hostname %q", parsed.Hostname())
	}
	if _, err := ValidatePublicHTTPSURL(context.Background(), "https://mixed.example/output.png", resolver); !errors.Is(err, ErrUnsafeRemoteURL) {
		t.Fatalf("mixed public/private DNS should be rejected, got %v", err)
	}
}

func TestValidatePublicHTTPSURLRejectsUnsafeURLForms(t *testing.T) {
	t.Parallel()
	resolver := staticResolver{"public.example": {"93.184.216.34"}}
	cases := []string{
		"http://public.example/output.png",
		"https://user:password@public.example/output.png",
		"https://public.example/output.png#fragment",
		" https://public.example/output.png",
		"https://127.0.0.1/output.png",
		"https://169.254.169.254/latest/meta-data",
		"https://[::1]/output.png",
	}
	for _, rawURL := range cases {
		if _, err := ValidatePublicHTTPSURL(context.Background(), rawURL, resolver); !errors.Is(err, ErrUnsafeRemoteURL) {
			t.Errorf("expected URL to be rejected: %q, got %v", rawURL, err)
		}
	}
}

func TestSafeDialContextRejectsMixedDNSBeforeDial(t *testing.T) {
	t.Parallel()
	resolver := staticResolver{"mixed.example": {"93.184.216.34", "10.0.0.5"}}
	called := false
	dial := safeDialContext(resolver, func(context.Context, string, string) (net.Conn, error) {
		called = true
		return nil, errors.New("unexpected dial")
	})
	if _, err := dial(context.Background(), "tcp", "mixed.example:443"); !errors.Is(err, ErrUnsafeRemoteURL) {
		t.Fatalf("expected unsafe address error, got %v", err)
	}
	if called {
		t.Fatal("dialer was called before all DNS answers were validated")
	}
}

func TestDownloaderEnforcesSizeAndMIME(t *testing.T) {
	t.Parallel()
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 24)...)
	tests := []struct {
		name        string
		contentType string
		body        []byte
		length      int64
		maxBytes    int64
		wantErr     error
	}{
		{name: "valid png", contentType: "image/png", body: png, length: int64(len(png)), maxBytes: 64},
		{name: "declared too large", contentType: "image/png", body: png, length: 65, maxBytes: 64, wantErr: ErrDownloadTooLarge},
		{name: "stream too large", contentType: "image/png", body: bytes.Repeat([]byte{0}, 65), length: -1, maxBytes: 64, wantErr: ErrDownloadTooLarge},
		{name: "html disguised as png", contentType: "image/png", body: []byte("<!doctype html><title>no</title>"), length: 31, maxBytes: 64, wantErr: ErrUnsupportedMIME},
		{name: "empty", contentType: "image/png", body: nil, length: 0, maxBytes: 64, wantErr: ErrEmptyDownload},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			d := testDownloader(staticResolver{"public.example": {"93.184.216.34"}}, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    http.StatusOK,
					Header:        http.Header{"Content-Type": []string{test.contentType}},
					Body:          io.NopCloser(bytes.NewReader(test.body)),
					ContentLength: test.length,
					Request:       req,
				}, nil
			}))
			result, err := d.Download(context.Background(), "https://public.example/output.png", DownloadLimits{
				MaxBytes:         test.maxBytes,
				AllowedMIMETypes: []string{"image/png"},
			})
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("expected %v, got %v", test.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("download failed: %v", err)
			}
			if result.MIMEType != "image/png" || !bytes.Equal(result.Data, png) || result.SizeBytes != int64(len(png)) {
				t.Fatalf("unexpected result: %#v", result)
			}
		})
	}
}

func TestDownloaderRejectsInvalidAllowedMIMEs(t *testing.T) {
	t.Parallel()
	d := testDownloader(staticResolver{"public.example": {"93.184.216.34"}}, roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("request should not be sent")
		return nil, nil
	}))
	for _, value := range []string{"", "*/*", "image", "image/", "/png", "image/**", "image/png; version=1"} {
		_, err := d.Download(context.Background(), "https://public.example/output.png", DownloadLimits{MaxBytes: 1, AllowedMIMETypes: []string{value}})
		if err == nil {
			t.Errorf("expected MIME pattern %q to be rejected", value)
		}
	}
}

func TestDownloaderRevalidatesRedirects(t *testing.T) {
	t.Parallel()
	resolver := staticResolver{
		"public.example":  {"93.184.216.34"},
		"private.example": {"127.0.0.1"},
	}
	requests := 0
	d := testDownloader(resolver, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.URL.Hostname() != "public.example" {
			t.Fatalf("redirect destination should not be requested: %s", req.URL.Hostname())
		}
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"https://private.example/output.mp4"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	}))
	_, err := d.Download(context.Background(), "https://public.example/output.mp4", VideoDownloadLimits())
	if !errors.Is(err, ErrUnsafeRemoteURL) {
		t.Fatalf("expected redirect to private address to fail, got %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected one request, got %d", requests)
	}
}

func TestDownloaderErrorsDoNotExposeSignedURLs(t *testing.T) {
	t.Parallel()
	const secret = "provider-signature-secret"
	d := testDownloader(staticResolver{"public.example": {"93.184.216.34"}}, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: req.URL.String(), Err: errors.New("upstream error mentions " + secret)}
	}))
	_, err := d.Download(context.Background(), "https://public.example/output.png?signature="+secret, ImageDownloadLimits())
	if err == nil {
		t.Fatal("expected download error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("download error leaked signed URL material: %v", err)
	}
}

func TestRedirectPolicyDropsSensitiveHeaders(t *testing.T) {
	t.Parallel()
	d := &Downloader{resolver: staticResolver{"public.example": {"93.184.216.34"}}, maxRedirects: 5}
	req, err := http.NewRequest(http.MethodGet, "https://public.example/output.png", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Authorization", "Cookie", "Proxy-Authorization", "Referer"} {
		req.Header.Set(name, "secret")
	}
	if err := d.checkRedirect(req, []*http.Request{{}}); err != nil {
		t.Fatalf("redirect rejected: %v", err)
	}
	for _, name := range []string{"Authorization", "Cookie", "Proxy-Authorization", "Referer"} {
		if req.Header.Get(name) != "" {
			t.Errorf("%s was retained across redirect", name)
		}
	}
}

func testDownloader(resolver Resolver, transport http.RoundTripper) *Downloader {
	d := &Downloader{resolver: resolver, maxRedirects: 5}
	d.client = &http.Client{Transport: transport, CheckRedirect: d.checkRedirect}
	return d
}
