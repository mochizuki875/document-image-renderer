package renderer

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// extractDOCXText extracts the text of the main document body from a DOCX archive.
// A DOCX stores its content in a single part (word/document.xml), so the result
// always contains exactly one TextPart.
func extractDOCXText(source string) ([]TextPart, error) {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		text, err := extractTextNodes(file)
		if err != nil {
			return nil, err
		}
		return []TextPart{{PartNumber: 1, Text: text}}, nil
	}
	// An empty document may have no document.xml part at all.
	return []TextPart{{PartNumber: 1}}, nil
}

// extractPPTXText extracts the text of every slide from a PPTX archive.
// Slides are collected from the ppt/slides/ directory and sorted by their
// numeric suffix so that PartNumber matches the presentation order.
func extractPPTXText(source string) ([]TextPart, error) {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	var slides []*zip.File
	for _, file := range archive.File {
		if strings.HasPrefix(file.Name, "ppt/slides/slide") && strings.HasSuffix(file.Name, ".xml") {
			slides = append(slides, file)
		}
	}
	// Archive order is not guaranteed, so sort by the slide number in the name.
	sort.Slice(slides, func(left, right int) bool {
		return numberedPart(slides[left].Name, "slide") < numberedPart(slides[right].Name, "slide")
	})
	parts := make([]TextPart, 0, len(slides))
	for index, slide := range slides {
		text, err := extractTextNodes(slide)
		if err != nil {
			return nil, err
		}
		parts = append(parts, TextPart{PartNumber: index + 1, Text: text})
	}
	return parts, nil
}

// extractTextNodes reads all text runs from an OOXML part.
// Both Word (<w:t>) and PowerPoint (<a:t>) text runs use the local element
// name "t", so a single pass over the XML tokens covers both formats.
func extractTextNodes(file *zip.File) (string, error) {
	input, err := file.Open()
	if err != nil {
		return "", err
	}
	defer input.Close()
	decoder := xml.NewDecoder(input)
	var parts []string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "t" {
			continue
		}
		var text string
		if err := decoder.DecodeElement(&text, &start); err != nil {
			return "", err
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	// One run per line keeps paragraphs readable in the extracted output.
	return strings.Join(parts, "\n"), nil
}

// numberedPart extracts the numeric suffix from an OOXML part name such as
// "ppt/slides/slide3.xml" (prefix "slide") so parts can be ordered numerically.
func numberedPart(name, prefix string) int {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	number, _ := strconv.Atoi(strings.TrimPrefix(base, prefix))
	return number
}
