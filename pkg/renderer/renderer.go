package renderer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var supportedExtensions = map[string]struct{}{
	".doc":  {},
	".docx": {},
	".pdf":  {},
	".ppt":  {},
	".pptx": {},
	".xls":  {},
	".xlsm": {},
	".xlsx": {},
}

// PageCount returns the rendered page count of a PDF or Office document.
func PageCount(ctx context.Context, source string) (int, error) {
	return PageCountWithOptions(ctx, source, nil)
}

// PageCountWithOptions returns the rendered page count using the supplied dependency options.
// Office documents are converted to a temporary PDF before their pages are counted.
func PageCountWithOptions(ctx context.Context, source string, options *ExtractOptions) (int, error) {
	extractOptions := DefaultExtractOptions()
	if options != nil {
		extractOptions = *options
	}
	if err := extractOptions.Validate(); err != nil {
		return 0, err
	}

	sourcePath, extension, err := validateDocument(ctx, source)
	if err != nil {
		return 0, err
	}
	pdfPath := sourcePath
	cleanup := func() {}
	if extension != ".pdf" {
		pdfPath, cleanup, err = convertOfficeToPDF(ctx, sourcePath, extension, RenderOptions{
			LibreOfficeTimeout:    extractOptions.LibreOfficeTimeout,
			LibreOfficeExecutable: extractOptions.LibreOfficeExecutable,
		})
		if err != nil {
			return 0, err
		}
		defer cleanup()
	}

	pageCount, err := pdfPageCount(ctx, pdfPath)
	if err != nil {
		return 0, &DocumentPageCountError{Path: sourcePath, Err: err}
	}
	return pageCount, nil
}

// RenderDocument renders every page in a PDF or Office document to an image.
func RenderDocument(
	ctx context.Context,
	source string,
	outputDirectory string,
	options *RenderOptions,
) (*RenderResult, error) {
	renderOptions := DefaultRenderOptions()
	if options != nil {
		renderOptions = *options
	}
	if err := renderOptions.Validate(); err != nil {
		return nil, err
	}

	sourcePath, extension, err := validateDocument(ctx, source)
	if err != nil {
		return nil, err
	}
	outputPath, err := filepath.Abs(outputDirectory)
	if err != nil {
		return nil, fmt.Errorf("resolve output directory: %w", err)
	}
	if err := os.MkdirAll(outputPath, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	prefix := renderOptions.FilenamePrefix
	if prefix == "" {
		prefix = strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	}

	pdfPath := sourcePath
	cleanup := func() {}
	if extension != ".pdf" {
		pdfPath, cleanup, err = convertOfficeToPDF(ctx, sourcePath, extension, renderOptions)
		if err != nil {
			return nil, err
		}
		defer cleanup()
	}

	images, err := renderPDF(ctx, pdfPath, outputPath, prefix, renderOptions)
	if err != nil {
		return nil, err
	}
	return &RenderResult{Source: sourcePath, Images: images}, nil
}

func validateDocument(ctx context.Context, source string) (string, string, error) {
	if ctx == nil {
		return "", "", fmt.Errorf("context must not be nil")
	}
	sourcePath, err := filepath.Abs(source)
	if err != nil {
		return "", "", fmt.Errorf("resolve input document: %w", err)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", "", fmt.Errorf("input document does not exist %q: %w", sourcePath, err)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("input document is not a regular file: %s", sourcePath)
	}
	extension := strings.ToLower(filepath.Ext(sourcePath))
	if _, supported := supportedExtensions[extension]; !supported {
		return "", "", &UnsupportedFormatError{Extension: extension, Path: sourcePath}
	}
	return sourcePath, extension, nil
}

// SupportedExtensions returns the accepted input extensions in lexical order.
func SupportedExtensions() []string {
	extensions := make([]string, 0, len(supportedExtensions))
	for extension := range supportedExtensions {
		extensions = append(extensions, extension)
	}
	sort.Strings(extensions)
	return extensions
}
