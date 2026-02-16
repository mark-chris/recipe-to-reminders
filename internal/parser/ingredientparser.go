package parser

import (
	"regexp"
	"strings"
)

const (
	maxOCRLines   = 200
	maxOCRLineLen = 500
)

// OCRParseResult holds the parsed output of OCR text analysis.
type OCRParseResult struct {
	IngredientLines []string // cleaned ingredient strings
	HeaderLines     int      // count of section headers detected
	NoiseLines      int      // count of discarded lines
	TotalLines      int      // total non-blank lines processed (capped at maxOCRLines)
}

// Matches lines starting with a digit, fraction, or Unicode fraction character.
var ingredientLinePattern = regexp.MustCompile(
	`(?i)^[\d½¼¾⅓⅔⅛]`)

// Matches quantity words at start of line.
var quantityWordPattern = regexp.MustCompile(
	`(?i)^(a\s+(?:pinch|dash|handful)\b|one|two|three|four|five|six|seven|eight|nine|ten|dozen)\b`)

// Matches section headers like "For the sauce:", "Marinade:", "Crust:"
var sectionHeaderPattern = regexp.MustCompile(
	`(?i)^(?:for\s+(?:the\s+)?)?[\w\s]+(:|—|-)\s*$`)

// Matches noise lines: page numbers, URLs, serving info, recipe metadata.
var noisePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^page\s+\d+`),
	regexp.MustCompile(`(?i)^https?://|^www\.`),
	regexp.MustCompile(`(?i)^serves?\s+\d|^prep\s+time|^cook\s+time|^total\s+time`),
	regexp.MustCompile(`(?i)^instructions?$|^directions?$|^method$|^steps?$`),
	regexp.MustCompile(`(?i)^ingredients?$`),
	regexp.MustCompile(`^\d+$`), // bare numbers (page numbers)
}

// ParseOCRText classifies lines from OCR output and extracts ingredient strings.
func ParseOCRText(text string) OCRParseResult {
	lines := strings.Split(text, "\n")
	result := OCRParseResult{}

	count := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		count++
		if count > maxOCRLines {
			break
		}

		// Cap line length
		if len(line) > maxOCRLineLen {
			line = line[:maxOCRLineLen]
		}

		// Check noise patterns first
		if isNoiseLine(line) {
			result.NoiseLines++
			continue
		}

		// Check section headers
		if sectionHeaderPattern.MatchString(line) {
			result.HeaderLines++
			continue
		}

		// Check if it looks like an ingredient
		if isIngredientLine(line) {
			result.IngredientLines = append(result.IngredientLines, line)
			continue
		}

		// Default: noise
		result.NoiseLines++
	}

	result.TotalLines = min(count, maxOCRLines)
	return result
}

func isNoiseLine(line string) bool {
	for _, p := range noisePatterns {
		if p.MatchString(line) {
			return true
		}
	}
	return false
}

func isIngredientLine(line string) bool {
	return ingredientLinePattern.MatchString(line) || quantityWordPattern.MatchString(line)
}
