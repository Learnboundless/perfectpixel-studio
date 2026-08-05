package safemedia

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultImageMaxBytes = 25 << 20
	DefaultVideoMaxBytes = 256 << 20
	defaultMaxRedirects  = 5
	defaultTimeout       = 5 * time.Minute
	maxRemoteURLBytes    = 16 << 10
	maxInt64             = int64(1<<63 - 1)
)

var (
	ErrUnsafeRemoteURL  = errors.New("unsafe remote media URL")
	ErrDownloadTooLarge = errors.New("remote media exceeds the size limit")
	ErrUnsupportedMIME  = errors.New("remote media MIME type is not allowed")
	ErrEmptyDownload    = errors.New("remote media response is empty")
)

var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:10::/28"),
	netip.MustParsePrefix("2001:20::/28"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fec0::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

// Resolver is the subset of net.Resolver needed to validate every address a
// remote media hostname can resolve to.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// DownloaderOptions controls safe network behavior. A nil Resolver uses the
// system resolver. Redirect and timeout zero values use conservative defaults.
type DownloaderOptions struct {
	Resolver     Resolver
	Timeout      time.Duration
	MaxRedirects int
}

// DownloadLimits must be supplied for every download so callers explicitly
// choose both a byte ceiling and the accepted media types. Entries may be exact
// MIME types or a family wildcard such as "image/*".
type DownloadLimits struct {
	MaxBytes         int64
	AllowedMIMETypes []string
}

// DownloadResult contains validated bytes and their normalized MIME type.
// The source URL is deliberately omitted because provider URLs often contain
// signed credentials in their query string.
type DownloadResult struct {
	Data      []byte
	MIMEType  string
	SizeBytes int64
}

// Downloader fetches provider media through a transport that revalidates DNS
// at connect time and never uses an environment-configured proxy.
type Downloader struct {
	resolver     Resolver
	client       *http.Client
	maxRedirects int
}

// NewDownloader creates a downloader that permits only HTTPS destinations
// resolving exclusively to publicly routable addresses.
func NewDownloader(options DownloaderOptions) *Downloader {
	resolver := options.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	maxRedirects := options.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = defaultMaxRedirects
	}

	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = safeDialContext(resolver, dialer.DialContext)
	transport.DialTLSContext = nil
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.MaxResponseHeaderBytes = 1 << 20
	transport.ForceAttemptHTTP2 = true

	d := &Downloader{resolver: resolver, maxRedirects: maxRedirects}
	d.client = &http.Client{
		Transport:     transport,
		Timeout:       timeout,
		CheckRedirect: d.checkRedirect,
	}
	return d
}

// ImageDownloadLimits returns the formats accepted for generated and reference
// raster images. SVG is intentionally excluded because it can contain active
// content when rendered by a browser.
func ImageDownloadLimits() DownloadLimits {
	return DownloadLimits{
		MaxBytes: DefaultImageMaxBytes,
		AllowedMIMETypes: []string{
			"image/png",
			"image/jpeg",
			"image/jpg",
			"image/webp",
			"image/gif",
			"image/bmp",
			"image/x-ms-bmp",
		},
	}
}

// VideoDownloadLimits returns the formats accepted for generated video output.
func VideoDownloadLimits() DownloadLimits {
	return DownloadLimits{
		MaxBytes: DefaultVideoMaxBytes,
		AllowedMIMETypes: []string{
			"video/mp4",
			"video/webm",
			"video/quicktime",
		},
	}
}

// ValidatePublicHTTPSURL parses a URL, resolves its hostname, and rejects the
// whole destination if any returned address is non-public.
func ValidatePublicHTTPSURL(ctx context.Context, rawURL string, resolver Resolver) (*url.URL, error) {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if len(rawURL) == 0 || len(rawURL) > maxRemoteURLBytes || strings.TrimSpace(rawURL) != rawURL {
		return nil, fmt.Errorf("%w: malformed URL", ErrUnsafeRemoteURL)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed URL", ErrUnsafeRemoteURL)
	}
	if !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" || parsed.Hostname() == "" || parsed.Opaque != "" {
		return nil, fmt.Errorf("%w: HTTPS with an absolute host is required", ErrUnsafeRemoteURL)
	}
	if parsed.User != nil || parsed.Fragment != "" || strings.Contains(parsed.Hostname(), "%") {
		return nil, fmt.Errorf("%w: credentials, fragments, and address zones are not allowed", ErrUnsafeRemoteURL)
	}
	if _, err := resolvePublicIPs(ctx, parsed.Hostname(), resolver); err != nil {
		return nil, err
	}
	parsed.Scheme = "https"
	return parsed, nil
}

// IsPublicIP reports whether an address is suitable for a remote provider
// download. It excludes private, loopback, link-local, multicast, documentation,
// benchmarking, translation, and other special-purpose networks.
func IsPublicIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsValid() || !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

// Download fetches and buffers one response after URL, redirect, DNS, status,
// size, and MIME validation.
func (d *Downloader) Download(ctx context.Context, rawURL string, limits DownloadLimits) (DownloadResult, error) {
	if d == nil || d.client == nil || d.resolver == nil {
		return DownloadResult{}, errors.New("safe media downloader is not initialized")
	}
	if limits.MaxBytes <= 0 || limits.MaxBytes == maxInt64 {
		return DownloadResult{}, errors.New("remote media size limit is invalid")
	}
	allowed, err := normalizeAllowedMIMEs(limits.AllowedMIMETypes)
	if err != nil {
		return DownloadResult{}, err
	}
	parsed, err := ValidatePublicHTTPSURL(ctx, rawURL, d.resolver)
	if err != nil {
		return DownloadResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return DownloadResult{}, errors.New("create secure media request")
	}
	req.Header.Set("Accept", strings.Join(limits.AllowedMIMETypes, ", "))
	resp, err := d.client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			switch {
			case errors.Is(urlErr.Err, ErrUnsafeRemoteURL):
				return DownloadResult{}, fmt.Errorf("secure media download blocked: %w", ErrUnsafeRemoteURL)
			case errors.Is(urlErr.Err, context.Canceled):
				return DownloadResult{}, context.Canceled
			case errors.Is(urlErr.Err, context.DeadlineExceeded):
				return DownloadResult{}, context.DeadlineExceeded
			default:
				return DownloadResult{}, errors.New("secure media download failed")
			}
		}
		return DownloadResult{}, fmt.Errorf("secure media download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return DownloadResult{}, fmt.Errorf("remote media download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > limits.MaxBytes {
		return DownloadResult{}, ErrDownloadTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limits.MaxBytes+1))
	if err != nil {
		return DownloadResult{}, fmt.Errorf("read remote media response: %w", err)
	}
	if int64(len(data)) > limits.MaxBytes {
		return DownloadResult{}, ErrDownloadTooLarge
	}
	if len(data) == 0 {
		return DownloadResult{}, ErrEmptyDownload
	}
	responsePath := parsed.Path
	if resp.Request != nil && resp.Request.URL != nil {
		responsePath = resp.Request.URL.Path
	}
	mimeType, err := validateResponseMIME(resp.Header.Get("Content-Type"), responsePath, data, allowed)
	if err != nil {
		return DownloadResult{}, err
	}
	return DownloadResult{Data: data, MIMEType: mimeType, SizeBytes: int64(len(data))}, nil
}

// CloseIdleConnections releases pooled connections when the owning app shuts
// down. A Downloader remains safe for concurrent use before that point.
func (d *Downloader) CloseIdleConnections() {
	if d != nil && d.client != nil {
		d.client.CloseIdleConnections()
	}
}

func (d *Downloader) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > d.maxRedirects {
		return errors.New("remote media redirect limit exceeded")
	}
	req.Header.Del("Authorization")
	req.Header.Del("Cookie")
	req.Header.Del("Proxy-Authorization")
	req.Header.Del("Referer")
	_, err := ValidatePublicHTTPSURL(req.Context(), req.URL.String(), d.resolver)
	return err
}

type dialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

func safeDialContext(resolver Resolver, dial dialContextFunc) dialContextFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid remote media address: %w", err)
		}
		ips, err := resolvePublicIPs(ctx, host, resolver)
		if err != nil {
			return nil, err
		}
		var dialErrors []error
		for _, ip := range ips {
			if network == "tcp4" && ip.To4() == nil {
				continue
			}
			if network == "tcp6" && ip.To4() != nil {
				continue
			}
			conn, dialErr := dial(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			dialErrors = append(dialErrors, dialErr)
		}
		if len(dialErrors) == 0 {
			return nil, errors.New("remote media hostname has no address for the requested network")
		}
		return nil, fmt.Errorf("connect to validated remote media host: %w", errors.Join(dialErrors...))
	}
}

func resolvePublicIPs(ctx context.Context, host string, resolver Resolver) ([]net.IP, error) {
	if parsed := net.ParseIP(host); parsed != nil {
		if !IsPublicIP(parsed) {
			return nil, fmt.Errorf("%w: destination address is not public", ErrUnsafeRemoteURL)
		}
		return []net.IP{parsed}, nil
	}
	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve remote media host: %w", err)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("%w: hostname has no addresses", ErrUnsafeRemoteURL)
	}
	result := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		if address.Zone != "" || !IsPublicIP(address.IP) {
			return nil, fmt.Errorf("%w: hostname resolves to a non-public address", ErrUnsafeRemoteURL)
		}
		result = append(result, address.IP)
	}
	return result, nil
}

func normalizeAllowedMIMEs(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errors.New("at least one allowed remote media MIME type is required")
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		parts := strings.Split(value, "/")
		valid := false
		if strings.HasSuffix(value, "/*") {
			valid = len(parts) == 2 && parts[0] != "" && parts[1] == "*" && !strings.Contains(parts[0], "*")
		} else if mediaType, params, parseErr := mime.ParseMediaType(value); parseErr == nil {
			valid = mediaType == value && len(params) == 0 && len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.Contains(value, "*")
		}
		if !valid || value == "*/*" {
			return nil, fmt.Errorf("invalid allowed MIME type %q", value)
		}
		result = append(result, value)
	}
	return result, nil
}

func validateResponseMIME(contentType, responsePath string, data []byte, allowed []string) (string, error) {
	declared := ""
	if strings.TrimSpace(contentType) != "" {
		var err error
		declared, err = parseMIME(contentType)
		if err != nil {
			return "", fmt.Errorf("%w: response Content-Type is invalid", ErrUnsupportedMIME)
		}
	}
	sniffed := normalizedMIME(http.DetectContentType(data[:min(len(data), 512)]))
	extension := normalizedMIME(mime.TypeByExtension(strings.ToLower(filepath.Ext(responsePath))))

	if declared != "" && declared != "application/octet-stream" && !mimeAllowed(declared, allowed) {
		return "", fmt.Errorf("%w: response declared %s", ErrUnsupportedMIME, declared)
	}
	if sniffed != "" && sniffed != "application/octet-stream" && !mimeAllowed(sniffed, allowed) {
		return "", fmt.Errorf("%w: response content looks like %s", ErrUnsupportedMIME, sniffed)
	}
	for _, candidate := range []string{sniffed, declared, extension} {
		if candidate != "" && candidate != "application/octet-stream" && mimeAllowed(candidate, allowed) {
			return candidate, nil
		}
	}
	return "", ErrUnsupportedMIME
}

func normalizedMIME(value string) string {
	mediaType, err := parseMIME(value)
	if err != nil {
		return ""
	}
	return mediaType
}

func parseMIME(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return "", err
	}
	return strings.ToLower(mediaType), nil
}

func mimeAllowed(value string, allowed []string) bool {
	for _, candidate := range allowed {
		if candidate == value {
			return true
		}
		if strings.HasSuffix(candidate, "/*") && strings.HasPrefix(value, strings.TrimSuffix(candidate, "*")) {
			return true
		}
	}
	return false
}
