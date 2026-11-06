package renderer

import "strings"

// TextPart contains text extracted from one document unit. A unit is a page
// for PDFs, a slide for presentations, or a worksheet for workbooks.
type TextPart struct {
	PartNumber int
	Text       string
}

// ExtractResult contains text extracted parts in source order.
type ExtractResult struct {
	Source string
	Parts  []TextPart
}

// PartCount returns the number of extracted document units.
func (result ExtractResult) PartCount() int {
	return len(result.Parts)
}

// Part returns the extracted text part with the requested one-based number.
// The bool is false when no part has that number.
func (result ExtractResult) Part(number int) (TextPart, bool) {
	for _, part := range result.Parts {
		if part.PartNumber == number {
			return part, true
		}
	}
	return TextPart{}, false
}

// Text joins extracted parts with a blank line.
func (result ExtractResult) Text() string {
	texts := make([]string, 0, len(result.Parts))
	for _, part := range result.Parts {
		texts = append(texts, part.Text)
	}
	return strings.Join(texts, "\n\n")
}
