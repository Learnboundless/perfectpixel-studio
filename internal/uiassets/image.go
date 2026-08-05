package uiassets

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"

	_ "golang.org/x/image/webp"
)

func Decode(r io.Reader, limits DecodeLimits) (*image.NRGBA, ImageInfo, error) {
	limits = normalizeDecodeLimits(limits)
	raw, err := io.ReadAll(io.LimitReader(r, limits.MaxBytes+1))
	if err != nil {
		return nil, ImageInfo{}, fmt.Errorf("read image: %w", err)
	}
	return DecodeBytes(raw, limits)
}

func DecodeBytes(raw []byte, limits DecodeLimits) (*image.NRGBA, ImageInfo, error) {
	limits = normalizeDecodeLimits(limits)
	if int64(len(raw)) > limits.MaxBytes {
		return nil, ImageInfo{}, fmt.Errorf("%w: got %d bytes, limit %d", ErrImageTooLarge, len(raw), limits.MaxBytes)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, ImageInfo{}, fmt.Errorf("decode image header: %w", err)
	}
	format = canonicalImageFormat(format)
	if !supportedFormat(format) {
		return nil, ImageInfo{}, fmt.Errorf("%w: %s", ErrUnsupportedImage, format)
	}
	if err := validateDimensions(config.Width, config.Height, limits.MaxPixels); err != nil {
		return nil, ImageInfo{}, err
	}
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, ImageInfo{}, fmt.Errorf("decode image: %w", err)
	}
	decodedFormat = canonicalImageFormat(decodedFormat)
	if decodedFormat != format {
		return nil, ImageInfo{}, fmt.Errorf("decode image: header format %q changed to %q", format, decodedFormat)
	}
	bounds := decoded.Bounds()
	if err := validateDimensions(bounds.Dx(), bounds.Dy(), limits.MaxPixels); err != nil {
		return nil, ImageInfo{}, err
	}
	img := ToNRGBA(decoded)
	return img, ImageInfo{
		Format:    format,
		MIMEType:  mimeForFormat(format),
		Width:     img.Bounds().Dx(),
		Height:    img.Bounds().Dy(),
		SizeBytes: int64(len(raw)),
	}, nil
}

func canonicalImageFormat(format string) string {
	if format == "apng" {
		return "png"
	}
	return format
}

func ToNRGBA(src image.Image) *image.NRGBA {
	if src == nil {
		return nil
	}
	bounds := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Src)
	return dst
}

func normalizeDecodeLimits(limits DecodeLimits) DecodeLimits {
	if limits.MaxBytes <= 0 {
		limits.MaxBytes = DefaultMaxImageBytes
	}
	if limits.MaxPixels <= 0 {
		limits.MaxPixels = DefaultMaxImagePixels
	}
	return limits
}

func validateDimensions(width, height int, maxPixels int64) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("%w: %dx%d", ErrInvalidImageSize, width, height)
	}
	if int64(width) > maxPixels/int64(height) {
		return fmt.Errorf("%w: %dx%d exceeds %d pixels", ErrImageTooManyPixels, width, height, maxPixels)
	}
	return nil
}

func supportedFormat(format string) bool {
	switch format {
	case "png", "jpeg", "gif", "webp":
		return true
	default:
		return false
	}
}

func mimeForFormat(format string) string {
	switch format {
	case "png":
		return "image/png"
	case "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}
