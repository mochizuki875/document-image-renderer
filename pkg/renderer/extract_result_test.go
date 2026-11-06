package renderer

import "testing"

func TestExtractResultTextJoinsParts(t *testing.T) {
	result := ExtractResult{Parts: []TextPart{{Text: "first"}, {Text: "second"}}}
	if text := result.Text(); text != "first\n\nsecond" {
		t.Fatalf("Text() = %q", text)
	}
}

func TestExtractResultPartFindsNumberedPart(t *testing.T) {
	result := ExtractResult{Parts: []TextPart{{PartNumber: 2, Text: "second"}}}
	part, found := result.Part(2)
	if !found || part.Text != "second" {
		t.Fatalf("Part(2) = %#v, %t", part, found)
	}
	if _, found := result.Part(1); found {
		t.Fatal("Part(1) unexpectedly found")
	}
}
