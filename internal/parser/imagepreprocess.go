package parser

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"

	// Register JPEG decoder so image.Decode/image.DecodeConfig can handle JPEG input.
	_ "image/jpeg"
)

// DecodeBase64Image decodes a base64 string, validates size, and returns raw bytes + format.
// maxSizeMB is the maximum allowed size of the decoded bytes in megabytes.
// Returns an error if the data exceeds the size limit, is not valid base64, or is not PNG/JPEG.
func DecodeBase64Image(b64 string, maxSizeMB int) ([]byte, string, error) {
	maxBytes := int64(maxSizeMB) * 1024 * 1024

	// Check base64 length before decoding (base64 inflates by ~33%)
	estimatedSize := int64(len(b64)) * 3 / 4
	if estimatedSize > maxBytes {
		return nil, "", fmt.Errorf("image exceeds maximum size of %dMB", maxSizeMB)
	}

	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, "", fmt.Errorf("invalid base64 image data")
	}

	if int64(len(data)) > maxBytes {
		return nil, "", fmt.Errorf("image exceeds maximum size of %dMB", maxSizeMB)
	}

	// Detect format via image.DecodeConfig (reads header only)
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("unsupported image format")
	}
	if format != "png" && format != "jpeg" {
		return nil, "", fmt.Errorf("unsupported image format: %s (must be PNG or JPEG)", format)
	}

	return data, format, nil
}

// ValidateImageDimensions checks that the image does not exceed maxDim x maxDim pixels.
// Returns an error if either dimension exceeds maxDim.
func ValidateImageDimensions(data []byte, maxDim int) error {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to read image dimensions")
	}
	if cfg.Width > maxDim || cfg.Height > maxDim {
		return fmt.Errorf("image dimensions %dx%d exceed maximum %dx%d", cfg.Width, cfg.Height, maxDim, maxDim)
	}
	return nil
}

// PrepareForOCR converts an image to grayscale PNG, resizing if either dimension exceeds maxDim.
// Aspect ratio is preserved when resizing. The output is always a PNG-encoded grayscale image.
func PrepareForOCR(data []byte, maxDim int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image")
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Resize if needed (maintain aspect ratio, scale to fit within maxDim x maxDim)
	if w > maxDim || h > maxDim {
		scale := float64(maxDim) / float64(w)
		if scaleH := float64(maxDim) / float64(h); scaleH < scale {
			scale = scaleH
		}
		newW := int(float64(w) * scale)
		newH := int(float64(h) * scale)
		img = resizeNearest(img, newW, newH)
		bounds = img.Bounds()
	}

	// Convert to grayscale
	gray := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray.Set(x, y, color.GrayModel.Convert(img.At(x, y)))
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, gray); err != nil {
		return nil, fmt.Errorf("failed to encode grayscale image")
	}
	return buf.Bytes(), nil
}

// resizeNearest performs nearest-neighbor resizing of an image.
func resizeNearest(src image.Image, newW, newH int) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	for y := range newH {
		for x := range newW {
			srcX := bounds.Min.X + x*bounds.Dx()/newW
			srcY := bounds.Min.Y + y*bounds.Dy()/newH
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}
