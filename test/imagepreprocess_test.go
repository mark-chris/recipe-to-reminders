package test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"recipe-to-reminders/internal/parser"
)

// helper: create a small test PNG image
func createTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// helper: create a small test JPEG image
func createTestJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecodeBase64Image_ValidPNG(t *testing.T) {
	raw := createTestPNG(t, 100, 100)
	b64 := base64.StdEncoding.EncodeToString(raw)
	imgBytes, format, err := parser.DecodeBase64Image(b64, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if format != "png" {
		t.Errorf("format = %q, want png", format)
	}
	if len(imgBytes) == 0 {
		t.Error("expected non-empty image bytes")
	}
}

func TestDecodeBase64Image_ValidJPEG(t *testing.T) {
	raw := createTestJPEG(t, 100, 100)
	b64 := base64.StdEncoding.EncodeToString(raw)
	_, format, err := parser.DecodeBase64Image(b64, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("format = %q, want jpeg", format)
	}
}

func TestDecodeBase64Image_TooLarge(t *testing.T) {
	// Create a base64 string that exceeds maxSizeMB=1
	large := make([]byte, 2*1024*1024)
	b64 := base64.StdEncoding.EncodeToString(large)
	_, _, err := parser.DecodeBase64Image(b64, 1)
	if err == nil {
		t.Fatal("expected error for oversized image")
	}
}

func TestDecodeBase64Image_InvalidBase64(t *testing.T) {
	_, _, err := parser.DecodeBase64Image("not-valid-base64!!!", 7)
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestDecodeBase64Image_UnsupportedFormat(t *testing.T) {
	// GIF header bytes — valid base64 but unsupported format
	gifHeader := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\xff\xff\xff\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")
	b64 := base64.StdEncoding.EncodeToString(gifHeader)
	_, _, err := parser.DecodeBase64Image(b64, 7)
	if err == nil {
		t.Fatal("expected error for unsupported format (GIF)")
	}
}

func TestValidateImageDimensions_WithinLimits(t *testing.T) {
	raw := createTestPNG(t, 2000, 1500)
	err := parser.ValidateImageDimensions(raw, 3000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateImageDimensions_TooWide(t *testing.T) {
	raw := createTestPNG(t, 4000, 100)
	err := parser.ValidateImageDimensions(raw, 3000)
	if err == nil {
		t.Fatal("expected error for oversized dimensions")
	}
}

func TestValidateImageDimensions_TooTall(t *testing.T) {
	raw := createTestPNG(t, 100, 4000)
	err := parser.ValidateImageDimensions(raw, 3000)
	if err == nil {
		t.Fatal("expected error for oversized dimensions")
	}
}

func TestValidateImageDimensions_ExactlyAtLimit(t *testing.T) {
	raw := createTestPNG(t, 3000, 3000)
	err := parser.ValidateImageDimensions(raw, 3000)
	if err != nil {
		t.Fatalf("unexpected error for dimensions exactly at limit: %v", err)
	}
}

func TestPrepareForOCR_ConvertsToGrayscale(t *testing.T) {
	raw := createTestPNG(t, 200, 200)
	prepared, err := parser.PrepareForOCR(raw, 3000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Decode result and verify it's grayscale
	img, err := png.Decode(bytes.NewReader(prepared))
	if err != nil {
		t.Fatalf("failed to decode prepared image: %v", err)
	}
	// Check a pixel — red input should become gray
	r, g, b, _ := img.At(100, 100).RGBA()
	if r != g || g != b {
		t.Errorf("expected grayscale pixel, got R=%d G=%d B=%d", r>>8, g>>8, b>>8)
	}
}

func TestPrepareForOCR_ResizesOversized(t *testing.T) {
	raw := createTestPNG(t, 500, 400)
	prepared, err := parser.PrepareForOCR(raw, 300) // maxDim=300
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(prepared))
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() > 300 || bounds.Dy() > 300 {
		t.Errorf("expected resized to <=300, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestPrepareForOCR_MaintainsAspectRatio(t *testing.T) {
	// 600x300 image with maxDim=200 should resize to 200x100
	raw := createTestPNG(t, 600, 300)
	prepared, err := parser.PrepareForOCR(raw, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(prepared))
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	bounds := img.Bounds()
	// Width should be 200 (the limiting dimension), height should be 100
	if bounds.Dx() != 200 {
		t.Errorf("width = %d, want 200", bounds.Dx())
	}
	if bounds.Dy() != 100 {
		t.Errorf("height = %d, want 100", bounds.Dy())
	}
}

func TestPrepareForOCR_NoResizeWhenWithinLimits(t *testing.T) {
	raw := createTestPNG(t, 100, 80)
	prepared, err := parser.PrepareForOCR(raw, 3000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(prepared))
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 80 {
		t.Errorf("expected 100x80 (no resize), got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestPrepareForOCR_AcceptsJPEG(t *testing.T) {
	raw := createTestJPEG(t, 150, 150)
	prepared, err := parser.PrepareForOCR(raw, 3000)
	if err != nil {
		t.Fatalf("unexpected error for JPEG input: %v", err)
	}
	// Output should be a valid PNG (grayscale)
	img, err := png.Decode(bytes.NewReader(prepared))
	if err != nil {
		t.Fatalf("expected PNG output, got decode error: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 150 || bounds.Dy() != 150 {
		t.Errorf("expected 150x150, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}
