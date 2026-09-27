package renderer

import (
	"context"
	"errors"
	"fmt"
)

// ExtractDocument extracts text from each document unit in source order.
func ExtractDocument(ctx context.Context, source string) (*ExtractResult, error) {
	return ExtractDocumentWithOptions(ctx, source, nil)
}

// ExtractDocumentWithOptions extracts text using the supplied dependency options.
// Each format is handled by a dedicated extractor; legacy binary formats are
// converted to OOXML first.
func ExtractDocumentWithOptions(ctx context.Context, source string, options *ExtractOptions) (*ExtractResult, error) {
	extractOptions := DefaultExtractOptions()
	if options != nil {
		extractOptions = *options
	}
	if err := extractOptions.Validate(); err != nil {
		return nil, err
	}
	sourcePath, extension, err := validateDocument(ctx, source)
	if err != nil {
		return nil, err
	}
	var parts []TextPart
	budget := &characterBudget{max: extractOptions.MaxCharacters}
	switch extension {
	case ".pdf":
		parts, err = extractPDFText(ctx, sourcePath, extractOptions.MaxPDFBytes, budget)
	case ".docx":
		parts, err = extractDOCXText(sourcePath, extractOptions, budget)
	case ".pptx":
		parts, err = extractPPTXText(sourcePath, extractOptions, budget)
	case ".xlsx", ".xlsm":
		parts, err = extractWorkbookText(sourcePath, extractOptions, budget)
	case ".doc", ".ppt", ".xls":
		parts, err = extractLegacyOfficeText(ctx, sourcePath, extension, extractOptions, budget)
	default:
		return &ExtractResult{Source: sourcePath}, nil
	}
	if err != nil {
		var extractionError *DocumentExtractionError
		if errors.As(err, &extractionError) {
			return nil, err
		}
		return nil, &DocumentExtractionError{Path: sourcePath, Err: err}
	}
	return &ExtractResult{Source: sourcePath, Parts: parts}, nil
}

// extractLegacyOfficeText converts a legacy binary Office document to OOXML
// and then reuses the corresponding OOXML extractor on the converted file.
func extractLegacyOfficeText(ctx context.Context, source, extension string, options ExtractOptions, budget *characterBudget) ([]TextPart, error) {
	converted, cleanup, err := convertLegacyOfficeToOOXML(ctx, source, extension, libreOfficeConfig{
		timeout: options.LibreOfficeTimeout, executable: options.LibreOfficeExecutable,
	})
	if err != nil {
		return nil, err
	}
	defer cleanup()
	switch extension {
	case ".doc":
		return extractDOCXText(converted, options, budget)
	case ".ppt":
		return extractPPTXText(converted, options, budget)
	case ".xls":
		return extractWorkbookText(converted, options, budget)
	default:
		return nil, fmt.Errorf("unsupported legacy Office format: %s", extension)
	}
}
