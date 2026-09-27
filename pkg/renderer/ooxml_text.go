package renderer

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
)

const (
	wordprocessingNamespace = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	drawingNamespace        = "http://schemas.openxmlformats.org/drawingml/2006/main"
)

func extractDOCXText(source string, options ExtractOptions, budget *characterBudget) ([]TextPart, error) {
	limits := extractOOXMLLimits(options)
	archive, err := openAndCheckOOXMLHeaders(source, limits)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	var total uint64
	for _, file := range archive.File {
		if file.Name == "word/document.xml" {
			input, err := openLimitedOOXMLMember(file, &total, limits)
			if err != nil {
				return nil, err
			}
			text, extractErr := extractParagraphText(input, wordprocessingNamespace, "p", budget)
			closeErr := input.Close()
			if extractErr != nil {
				return nil, extractErr
			}
			if closeErr != nil {
				return nil, closeErr
			}
			return []TextPart{{PartNumber: 1, Text: text}}, nil
		}
	}
	return []TextPart{{PartNumber: 1}}, nil
}

func extractPPTXText(source string, options ExtractOptions, budget *characterBudget) ([]TextPart, error) {
	limits := extractOOXMLLimits(options)
	archive, err := openAndCheckOOXMLHeaders(source, limits)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	files := make(map[string]*zip.File, len(archive.File))
	for _, file := range archive.File {
		files[file.Name] = file
	}
	var total uint64
	presentation, err := requiredOOXMLMember(files, "ppt/presentation.xml", &total, limits)
	if err != nil {
		return nil, err
	}
	relationships, err := requiredOOXMLMember(files, "ppt/_rels/presentation.xml.rels", &total, limits)
	if err != nil {
		return nil, err
	}
	slideNames, err := orderedSlideNames(presentation, relationships)
	if err != nil {
		return nil, err
	}
	parts := make([]TextPart, 0, len(slideNames))
	for index, name := range slideNames {
		file := files[name]
		if file == nil {
			return nil, fmt.Errorf("required OOXML part %q is missing", name)
		}
		slide, err := openLimitedOOXMLMember(file, &total, limits)
		if err != nil {
			return nil, err
		}
		text, extractErr := extractParagraphText(slide, drawingNamespace, "p", budget)
		closeErr := slide.Close()
		if extractErr != nil {
			return nil, extractErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		parts = append(parts, TextPart{PartNumber: index + 1, Text: text})
	}
	return parts, nil
}

func requiredOOXMLMember(files map[string]*zip.File, name string, total *uint64, limits ooxmlLimits) ([]byte, error) {
	file := files[name]
	if file == nil {
		return nil, fmt.Errorf("required OOXML part %q is missing", name)
	}
	return readLimitedOOXMLMember(file, total, limits)
}

func orderedSlideNames(presentation, relationships []byte) ([]string, error) {
	var order struct {
		Slides []struct {
			RelationshipID string `xml:"id,attr"`
		} `xml:"sldIdLst>sldId"`
	}
	if err := xml.Unmarshal(presentation, &order); err != nil {
		return nil, fmt.Errorf("parse presentation order: %w", err)
	}
	var relationList struct {
		Relationships []struct {
			ID         string `xml:"Id,attr"`
			Target     string `xml:"Target,attr"`
			TargetMode string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(relationships, &relationList); err != nil {
		return nil, fmt.Errorf("parse presentation relationships: %w", err)
	}
	targets := make(map[string]string, len(relationList.Relationships))
	for _, relationship := range relationList.Relationships {
		if relationship.TargetMode != "" && !strings.EqualFold(relationship.TargetMode, "Internal") {
			continue
		}
		targets[relationship.ID] = relationship.Target
	}
	names := make([]string, 0, len(order.Slides))
	for _, slide := range order.Slides {
		target, found := targets[slide.RelationshipID]
		if !found || target == "" {
			return nil, fmt.Errorf("slide relationship %q is missing", slide.RelationshipID)
		}
		parsedTarget, err := url.PathUnescape(target)
		if err != nil || strings.Contains(parsedTarget, "\\") || path.IsAbs(parsedTarget) || strings.ContainsAny(parsedTarget, "?#") {
			return nil, fmt.Errorf("slide relationship %q has invalid target %q", slide.RelationshipID, target)
		}
		name := path.Clean(path.Join("ppt", parsedTarget))
		if !strings.HasPrefix(name, "ppt/slides/") || name == "ppt/slides" {
			return nil, fmt.Errorf("slide relationship %q has invalid target %q", slide.RelationshipID, target)
		}
		names = append(names, name)
	}
	return names, nil
}

func extractParagraphText(input io.Reader, namespace, paragraphName string, budget *characterBudget) (string, error) {
	decoder := xml.NewDecoder(input)
	var output strings.Builder
	var paragraph strings.Builder
	paragraphCharacters := 0
	inParagraph := false
	inText := false
	paragraphs := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Space == namespace && value.Name.Local == paragraphName {
				paragraph.Reset()
				paragraphCharacters = 0
				inParagraph = true
				continue
			}
			if !inParagraph || value.Name.Space != namespace {
				continue
			}
			var structuralText string
			switch value.Name.Local {
			case "t":
				inText = true
			case "br", "cr":
				structuralText = "\n"
			case "tab":
				structuralText = "\t"
			}
			if structuralText != "" {
				max, limited := paragraphLimit(budget, paragraphs)
				if err := appendBounded(&paragraph, &paragraphCharacters, structuralText, max, budget.max, limited); err != nil {
					return "", err
				}
			}
		case xml.CharData:
			if inParagraph && inText {
				max, limited := paragraphLimit(budget, paragraphs)
				if err := appendBounded(&paragraph, &paragraphCharacters, string(value), max, budget.max, limited); err != nil {
					return "", err
				}
			}
		case xml.EndElement:
			if value.Name.Space == namespace && value.Name.Local == "t" {
				inText = false
			}
			if value.Name.Space == namespace && value.Name.Local == paragraphName {
				text := paragraph.String()
				if strings.Trim(text, "\n\t") != "" {
					if paragraphs > 0 {
						if err := budget.append(&output, "\n"); err != nil {
							return "", err
						}
					}
					if err := budget.append(&output, text); err != nil {
						return "", err
					}
					paragraphs++
				}
				inParagraph = false
				inText = false
			}
		}
	}
	return output.String(), nil
}

func paragraphLimit(budget *characterBudget, existingParagraphs int) (int, bool) {
	if budget.max == 0 {
		return 0, false
	}
	remaining := budget.max - budget.count
	if existingParagraphs > 0 {
		remaining--
	}
	if remaining < 0 {
		remaining = 0
	}
	return remaining, true
}
