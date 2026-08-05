package uiassets

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestDecodeBytesValidatesEncodedSizeAndPixels(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 20, 10))
	source.SetNRGBA(3, 4, color.NRGBA{R: 12, G: 34, B: 56, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}

	decoded, info, err := DecodeBytes(encoded.Bytes(), DefaultDecodeLimits())
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != "png" || info.MIMEType != "image/png" || info.Width != 20 || info.Height != 10 {
		t.Fatalf("unexpected info: %+v", info)
	}
	if got := decoded.NRGBAAt(3, 4); got != (color.NRGBA{R: 12, G: 34, B: 56, A: 255}) {
		t.Fatalf("decoded pixel = %+v", got)
	}

	_, _, err = DecodeBytes(encoded.Bytes(), DecodeLimits{MaxBytes: int64(encoded.Len() - 1), MaxPixels: 1_000})
	if !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("byte limit error = %v", err)
	}
	_, _, err = DecodeBytes(encoded.Bytes(), DecodeLimits{MaxBytes: int64(encoded.Len()), MaxPixels: 199})
	if !errors.Is(err, ErrImageTooManyPixels) {
		t.Fatalf("pixel limit error = %v", err)
	}
}

func TestDecodeReaderStopsAtByteLimit(t *testing.T) {
	data := bytes.Repeat([]byte{0xff}, 64)
	_, _, err := Decode(bytes.NewReader(data), DecodeLimits{MaxBytes: 16, MaxPixels: 100})
	if !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("error = %v", err)
	}
}

func TestToNRGBANormalizesNonZeroOrigin(t *testing.T) {
	source := image.NewNRGBA(image.Rect(10, 20, 12, 23))
	source.SetNRGBA(11, 22, color.NRGBA{R: 90, A: 255})
	got := ToNRGBA(source)
	if got.Bounds() != image.Rect(0, 0, 2, 3) {
		t.Fatalf("bounds = %v", got.Bounds())
	}
	if got.NRGBAAt(1, 2).R != 90 {
		t.Fatalf("pixel was not translated: %+v", got.NRGBAAt(1, 2))
	}
}
