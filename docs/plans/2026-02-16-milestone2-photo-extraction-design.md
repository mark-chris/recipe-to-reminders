# Milestone 2: Photo Extraction — Design

## Goal

Add photo-based ingredient extraction to the `/extract` endpoint. Users send a base64-encoded recipe photo; the system returns structured ingredients using Tesseract OCR with a heuristic parser as primary, falling back to Claude Vision API when confidence is low.

## Decisions

- **Tesseract Lambda layer deferred** to Milestone 4 (AWS Deployment). Code assumes Tesseract is on PATH.
- **Tests mock Tesseract** via an `OCREngine` interface. One skip-guarded integration test validates the real subprocess. CI has no Tesseract dependency.
- **Image preprocessing uses Go stdlib only** (`image/png`, `image/jpeg`). No new image library dependencies.
- **Anthropic Go SDK** (`anthropics/anthropic-sdk-go`) for Claude Vision fallback.

## Architecture

The photo path plugs into the existing `Extractor` as a parallel pipeline. When `image_base64` is provided instead of `url`, the handler decodes the image, runs it through Tesseract OCR + the ingredient parser, checks confidence, optionally falls back to Claude, then feeds results through the same normalize/categorize/dedup pipeline.

### New Files

| File | Purpose |
|------|---------|
| `internal/parser/imagepreprocess.go` | Base64 decode, format validation (JPEG/PNG), dimension check (3000x3000 max), resize, grayscale conversion |
| `internal/parser/tesseract.go` | `OCREngine` interface + `TesseractEngine` implementation. `os/exec` wrapper with stdin piping, 20s timeout, parameter validation at startup |
| `internal/parser/ingredientparser.go` | Line classification (ingredient vs header vs noise), extraction of raw ingredient strings from OCR text |
| `internal/parser/confidence.go` | 4-factor scoring: ingredient line ratio (0.4), parse success rate (0.3), OCR char confidence (0.2), ingredient count plausibility (0.1) |
| `internal/parser/claude.go` | `ImageExtractor` interface + `ClaudeExtractor` implementation. Anthropic SDK, system prompt, strict output validation, rate limiting |

### Modified Files

| File | Change |
|------|--------|
| `internal/parser/parser.go` | Add `ExtractImage(ctx, imageBytes)` method to `Extractor`. Wire OCREngine + ImageExtractor. |
| `internal/handler/extract.go` | Accept `image_base64`, decode, validate size, call `ExtractImage` |
| `go.mod` | Add `anthropics/anthropic-sdk-go` |

## Data Flow

```
image_base64 in request
  -> base64 decode -> []byte (reject if >7MB before decode)
  -> imagepreprocess.Validate: decode header, check JPEG/PNG, check <=3000x3000
  -> imagepreprocess.Prepare: resize if oversized, convert to grayscale PNG
  -> OCREngine.Run(ctx, grayscaleBytes) -> (rawText, charConfidence)
  -> ingredientparser.Parse(rawText) -> []ParsedLine
  -> confidence.Score(parsedLines, charConfidence) -> float64
  -> if score >= threshold: build RawRecipe{method:"tesseract", confidence:score}
  -> if score < threshold AND ANTHROPIC_API_KEY set: claude.Extract(ctx, originalImage)
  -> if score < threshold AND no key: return tesseract results with low confidence
  -> processRaw(): normalize -> categorize -> dedup -> ExtractResponse
```

The ingredient parser (`ingredientparser.go`) classifies OCR lines and extracts raw ingredient strings. The existing `normalize.go` then parses those strings into structured quantity/unit/name fields. No duplication of regex logic.

## Interfaces

```go
// OCREngine abstracts Tesseract for testing.
type OCREngine interface {
    Run(ctx context.Context, image []byte) (text string, charConfidence float64, err error)
}

// ImageExtractor abstracts Claude fallback for testing.
type ImageExtractor interface {
    Extract(ctx context.Context, image []byte) (*models.RawRecipe, error)
}
```

Both injected into `Extractor`. Production wires real implementations; tests wire mocks.

## Confidence Scoring

Four weighted factors (per CLAUDE.md):

| Factor | Weight | Description |
|--------|--------|-------------|
| Ingredient line ratio | 0.4 | Classified ingredient lines / total non-blank lines. Recipe lists should be >60%. |
| Parse success rate | 0.3 | Lines with all 3 fields (qty, unit, name) / total ingredient lines. |
| OCR char confidence | 0.2 | Tesseract per-character confidence (0-100 mapped to 0.0-1.0). |
| Count plausibility | 0.1 | Penalty curve outside 5-25 ingredient range. |

Default threshold: 0.65 (configurable via `CONFIDENCE_THRESHOLD`).

## Claude Vision Fallback

- Uses Anthropic Go SDK with Haiku 4.5 model
- Sends original (un-preprocessed) image with CLAUDE.md system prompt
- Output validation: strict Go struct unmarshal (only `title` + `ingredients`), also unmarshal to `map[string]any` to reject unexpected keys
- Field sanitization: same limits as user-supplied input (name 200 chars, raw 500 chars)
- Caps: max_tokens 4096, reject >50KB response, reject >100 ingredients
- Rate limit: token bucket, max `CLAUDE_FALLBACK_MAX_PER_MIN` (default 10) per minute
- Graceful degradation: if no API key, return Tesseract results with low-confidence flag

## Security

- Image data piped to Tesseract via stdin only (never temp files)
- `TESSERACT_PSM` validated as integer 0-13, `TESSERACT_LANG` validated as `^[a-z]{3}$`, both at startup
- Tesseract subprocess killed after 20s context timeout
- Claude response treated as untrusted input (images may contain prompt injection text)
- Base64 payload size checked before decoding (7MB limit prevents memory bombs)
- Decoded image dimensions checked before processing (3000x3000 max)
- OCR output capped at 200 lines, 500 chars per line

## Testing

| Test file | What it covers |
|-----------|---------------|
| `test/ingredientparser_test.go` | Line classification, qty/unit/name extraction from OCR text, section headers, noise, edge cases |
| `test/confidence_test.go` | High/low/boundary scoring, weight verification |
| `test/parser_test.go` (extended) | Full photo pipeline with mock OCREngine, mock Claude, fallback triggering, strategy overrides |
| `test/security_test.go` (extended) | Image size limits, dimension limits, Tesseract param validation |

Tesseract is mocked via `OCREngine` interface in all unit tests. One integration test (skip-guarded with `exec.LookPath`) validates the real subprocess against a fixture image.

## Environment Variables (new)

| Variable | Default | Description |
|----------|---------|-------------|
| `ANTHROPIC_API_KEY` | (none) | Claude API key. Omit to disable fallback. |
| `CONFIDENCE_THRESHOLD` | 0.65 | Min score to accept Tesseract results |
| `PHOTO_STRATEGY` | tesseract-first | `tesseract-first`, `claude-only`, `tesseract-only` |
| `TESSERACT_PSM` | 6 | Page segmentation mode (0-13) |
| `TESSERACT_LANG` | eng | Language pack (`^[a-z]{3}$`) |
| `MAX_IMAGE_SIZE_MB` | 7 | Max image size before base64 encoding |
| `CLAUDE_FALLBACK_MAX_PER_MIN` | 10 | Claude API rate limit |
