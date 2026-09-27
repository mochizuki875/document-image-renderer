package renderer

import "fmt"

// PageLimitExceededError reports a document that exceeds the configured render page limit.
type PageLimitExceededError struct {
	PageCount int
	MaxPages  int
}

func (err *PageLimitExceededError) Error() string {
	return fmt.Sprintf("document has %d pages, exceeding the limit of %d", err.PageCount, err.MaxPages)
}

// PageSizeLimitExceededError reports a page whose raster dimensions exceed a configured limit.
type PageSizeLimitExceededError struct {
	PageNumber int
	Width      int
	Height     int
	Pixels     uint64
	Limit      uint64
	Dimension  string
}

func (err *PageSizeLimitExceededError) Error() string {
	return fmt.Sprintf("page %d is %dx%d pixels, exceeding the %s limit of %d", err.PageNumber, err.Width, err.Height, err.Dimension, err.Limit)
}

// DocumentPixelLimitExceededError reports a document whose rendered pages exceed the total pixel limit.
type DocumentPixelLimitExceededError struct {
	Pixels    uint64
	MaxPixels uint64
}

func (err *DocumentPixelLimitExceededError) Error() string {
	return fmt.Sprintf("document requires %d pixels, exceeding the limit of %d", err.Pixels, err.MaxPixels)
}

// PDFSizeLimitExceededError reports a PDF input that exceeds the configured byte limit.
type PDFSizeLimitExceededError struct {
	Bytes    uint64
	MaxBytes uint64
}

func (err *PDFSizeLimitExceededError) Error() string {
	return fmt.Sprintf("PDF has %d bytes, exceeding the limit of %d", err.Bytes, err.MaxBytes)
}

// CharacterLimitExceededError reports text extraction that exceeds the configured character limit.
type CharacterLimitExceededError struct {
	CharacterCount int
	MaxCharacters  int
}

func (err *CharacterLimitExceededError) Error() string {
	return fmt.Sprintf("extracted text has %d characters, exceeding the limit of %d", err.CharacterCount, err.MaxCharacters)
}

// OOXMLLimitExceededError reports an OOXML archive that exceeds a configured resource limit.
type OOXMLLimitExceededError struct {
	LimitType string
	Member    string
	Actual    uint64
	Limit     uint64
}

func (err *OOXMLLimitExceededError) Error() string {
	if err.Member != "" {
		return fmt.Sprintf("OOXML member %q has %d %s, exceeding the limit of %d", err.Member, err.Actual, err.LimitType, err.Limit)
	}
	return fmt.Sprintf("OOXML archive has %d %s, exceeding the limit of %d", err.Actual, err.LimitType, err.Limit)
}

// UnsupportedFormatError reports an unsupported input extension.
type UnsupportedFormatError struct {
	Extension string
	Path      string
}

func (err *UnsupportedFormatError) Error() string {
	extension := err.Extension
	if extension == "" {
		extension = "<none>"
	}
	return fmt.Sprintf("unsupported document format %q: %s", extension, err.Path)
}

// DependencyNotFoundError reports a missing external dependency.
type DependencyNotFoundError struct {
	Dependency string
	Operation  string
}

func (err *DependencyNotFoundError) Error() string {
	return fmt.Sprintf("%s is required to %s", err.Dependency, err.Operation)
}

// DocumentConversionError reports an Office document conversion failure.
type DocumentConversionError struct {
	Path   string
	Stdout string
	Stderr string
	Err    error
}

func (err *DocumentConversionError) Error() string {
	return fmt.Sprintf("could not convert Office document %q: %v", err.Path, err.Err)
}

func (err *DocumentConversionError) Unwrap() error { return err.Err }

// DocumentPageCountError reports a failure while counting a document's rendered pages.
type DocumentPageCountError struct {
	Path string
	Err  error
}

func (err *DocumentPageCountError) Error() string {
	return fmt.Sprintf("could not count pages in document %q: %v", err.Path, err.Err)
}

func (err *DocumentPageCountError) Unwrap() error { return err.Err }

// DocumentRenderError reports a PDF rasterization failure.
type DocumentRenderError struct {
	Path string
	Err  error
}

func (err *DocumentRenderError) Error() string {
	return fmt.Sprintf("could not render PDF %q: %v", err.Path, err.Err)
}

func (err *DocumentRenderError) Unwrap() error { return err.Err }

// DocumentExtractionError reports a document text extraction failure.
type DocumentExtractionError struct {
	Path string
	Err  error
}

func (err *DocumentExtractionError) Error() string {
	return fmt.Sprintf("could not extract document text %q: %v", err.Path, err.Err)
}

func (err *DocumentExtractionError) Unwrap() error { return err.Err }
