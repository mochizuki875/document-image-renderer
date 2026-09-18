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
	return []TextPart{{PartNumber: 1}}, nil
}

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
	return strings.Join(parts, "\n"), nil
}

func numberedPart(name, prefix string) int {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	number, _ := strconv.Atoi(strings.TrimPrefix(base, prefix))
	return number
}
