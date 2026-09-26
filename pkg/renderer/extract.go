package renderer

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"
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
	switch extension {
	case ".pdf":
		parts, err = extractPDFText(ctx, sourcePath)
	case ".docx":
		parts, err = extractDOCXText(sourcePath)
	case ".pptx":
		parts, err = extractPPTXText(sourcePath)
	case ".xlsx", ".xlsm":
		parts, err = extractWorkbookText(sourcePath)
	case ".doc", ".ppt", ".xls":
		parts, err = extractLegacyOfficeText(ctx, sourcePath, extension, extractOptions)
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
	if err := validateCharacterLimit(parts, extractOptions.MaxCharacters); err != nil {
		return nil, &DocumentExtractionError{Path: sourcePath, Err: err}
	}
	return &ExtractResult{Source: sourcePath, Parts: parts}, nil
}

// validateCharacterLimit enforces the configured maximum on the total number
// of extracted characters. A limit of zero means no limit.
func validateCharacterLimit(parts []TextPart, maxCharacters int) error {
	if maxCharacters == 0 {
		return nil
	}
	characterCount := 0
	for _, part := range parts {
		characterCount += utf8.RuneCountInString(part.Text)
	}
	if characterCount > maxCharacters {
		return &CharacterLimitExceededError{CharacterCount: characterCount, MaxCharacters: maxCharacters}
	}
	return nil
}

// extractLegacyOfficeText converts a legacy binary Office document to OOXML
// and then reuses the corresponding OOXML extractor on the converted file.
func extractLegacyOfficeText(ctx context.Context, source, extension string, options ExtractOptions) ([]TextPart, error) {
	converted, cleanup, err := convertLegacyOfficeToOOXML(ctx, source, extension, libreOfficeConfig{
		timeout: options.LibreOfficeTimeout, executable: options.LibreOfficeExecutable,
	})
	if err != nil {
		return nil, err
	}
	defer cleanup()
	switch extension {
	case ".doc":
		return extractDOCXText(converted)
	case ".ppt":
		return extractPPTXText(converted)
	case ".xls":
		return extractWorkbookText(converted)
	default:
		return nil, fmt.Errorf("unsupported legacy Office format: %s", extension)
	}
}
