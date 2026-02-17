package parser

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

const tesseractTimeout = 20 * time.Second

var langPattern = regexp.MustCompile(`^[a-z]{3}$`)

// OCREngine abstracts Tesseract for testing.
type OCREngine interface {
	Run(ctx context.Context, image []byte) (text string, charConfidence float64, err error)
}

// TesseractEngine calls Tesseract via os/exec with stdin/stdout piping.
type TesseractEngine struct {
	psm  string
	lang string
}

// NewTesseractEngine creates a TesseractEngine after validating parameters.
// Empty psm defaults to "6", empty lang defaults to "eng".
func NewTesseractEngine(psm, lang string) (*TesseractEngine, error) {
	if psm == "" {
		psm = "6"
	}
	if lang == "" {
		lang = "eng"
	}

	// Validate PSM: integer 0-13
	n, err := strconv.Atoi(psm)
	if err != nil || n < 0 || n > 13 {
		return nil, fmt.Errorf("invalid TESSERACT_PSM %q: must be integer 0-13", psm)
	}

	// Validate lang: exactly 3 lowercase letters
	if !langPattern.MatchString(lang) {
		return nil, fmt.Errorf("invalid TESSERACT_LANG %q: must match ^[a-z]{3}$", lang)
	}

	return &TesseractEngine{psm: psm, lang: lang}, nil
}

// Run pipes image bytes to Tesseract via stdin and returns the extracted text.
// charConfidence is Tesseract's average character confidence (0-100 mapped to 0.0-1.0).
func (t *TesseractEngine) Run(ctx context.Context, image []byte) (string, float64, error) {
	ctx, cancel := context.WithTimeout(ctx, tesseractTimeout)
	defer cancel()

	// Run Tesseract: read from stdin, write text to stdout
	cmd := exec.CommandContext(ctx, "tesseract", "stdin", "stdout", // #nosec G204 -- psm/lang validated in NewTesseractEngine
		"-l", t.lang, "--psm", t.psm)
	cmd.Stdin = bytes.NewReader(image)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", 0, fmt.Errorf("tesseract failed: %v", err)
	}

	text := stdout.String()

	// Get confidence via a second call with TSV output
	confidence := t.getConfidence(ctx, image)

	return text, confidence, nil
}

// getConfidence runs Tesseract with TSV output to extract average character confidence.
func (t *TesseractEngine) getConfidence(ctx context.Context, image []byte) float64 {
	cmd := exec.CommandContext(ctx, "tesseract", "stdin", "stdout", // #nosec G204 -- psm/lang validated in NewTesseractEngine
		"-l", t.lang, "--psm", t.psm, "tsv")
	cmd.Stdin = bytes.NewReader(image)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return 0
	}

	// Parse TSV: last column is confidence (0-100), skip header and -1 values
	var total, count float64
	for line := range bytes.SplitSeq(stdout.Bytes(), []byte("\n")) {
		fields := bytes.Split(line, []byte("\t"))
		if len(fields) < 12 {
			continue
		}
		conf, err := strconv.ParseFloat(string(fields[11]), 64)
		if err != nil || conf < 0 {
			continue
		}
		total += conf
		count++
	}

	if count == 0 {
		return 0
	}
	return (total / count) / 100.0 // Map 0-100 to 0.0-1.0
}
