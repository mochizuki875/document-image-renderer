package renderer

import (
	"errors"
	"strings"
	"testing"
)

func TestPageLimitExceededErrorMessage(t *testing.T) {
	err := &PageLimitExceededError{PageCount: 5, MaxPages: 3}
	if got := err.Error(); got != "document has 5 pages, exceeding the limit of 3" {
		t.Fatalf("unexpected message: %q", got)
	}
}

func TestPageSizeLimitExceededErrorMessage(t *testing.T) {
	err := &PageSizeLimitExceededError{PageNumber: 2, Width: 100, Height: 200, Pixels: 20_000, Limit: 10_000, Dimension: "pixel count"}
	if got := err.Error(); got != "page 2 is 100x200 pixels, exceeding the pixel count limit of 10000" {
		t.Fatalf("unexpected message: %q", got)
	}
}

func TestDocumentPixelLimitExceededErrorMessage(t *testing.T) {
	err := &DocumentPixelLimitExceededError{Pixels: 1_000_000, MaxPixels: 500_000}
	if got := err.Error(); got != "document requires 1000000 pixels, exceeding the limit of 500000" {
		t.Fatalf("unexpected message: %q", got)
	}
}

func TestPDFSizeLimitExceededErrorMessage(t *testing.T) {
	err := &PDFSizeLimitExceededError{Bytes: 2048, MaxBytes: 1024}
	if got := err.Error(); got != "PDF has 2048 bytes, exceeding the limit of 1024" {
		t.Fatalf("unexpected message: %q", got)
	}
}

func TestCharacterLimitExceededErrorMessage(t *testing.T) {
	err := &CharacterLimitExceededError{CharacterCount: 100, MaxCharacters: 50}
	if got := err.Error(); got != "extracted text has 100 characters, exceeding the limit of 50" {
		t.Fatalf("unexpected message: %q", got)
	}
}

func TestOOXMLLimitExceededErrorMessage(t *testing.T) {
	memberErr := &OOXMLLimitExceededError{LimitType: "uncompressed bytes", Member: "word/document.xml", Actual: 100, Limit: 50}
	if got := memberErr.Error(); got != `OOXML member "word/document.xml" has 100 uncompressed bytes, exceeding the limit of 50` {
		t.Fatalf("unexpected member message: %q", got)
	}
	archiveErr := &OOXMLLimitExceededError{LimitType: "members", Actual: 10, Limit: 5}
	if got := archiveErr.Error(); got != "OOXML archive has 10 members, exceeding the limit of 5" {
		t.Fatalf("unexpected archive message: %q", got)
	}
}

func TestUnsupportedFormatErrorMessage(t *testing.T) {
	err := &UnsupportedFormatError{Extension: ".txt", Path: "/tmp/input.txt"}
	if got := err.Error(); got != `unsupported document format ".txt": /tmp/input.txt` {
		t.Fatalf("unexpected message: %q", got)
	}
	noExtension := &UnsupportedFormatError{Path: "/tmp/input"}
	if got := noExtension.Error(); got != `unsupported document format "<none>": /tmp/input` {
		t.Fatalf("unexpected message without extension: %q", got)
	}
}

func TestDependencyNotFoundErrorMessage(t *testing.T) {
	err := &DependencyNotFoundError{Dependency: "LibreOffice", Operation: "convert .doc documents"}
	if got := err.Error(); got != "LibreOffice is required to convert .doc documents" {
		t.Fatalf("unexpected message: %q", got)
	}
}

func TestDocumentConversionErrorWrapsCause(t *testing.T) {
	cause := errors.New("exit status 1")
	err := &DocumentConversionError{Path: "/tmp/input.doc", Stdout: "out", Stderr: "err", Err: cause}
	if got := err.Error(); got != `could not convert Office document "/tmp/input.doc": exit status 1` {
		t.Fatalf("unexpected message: %q", got)
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is must find the wrapped cause")
	}
}

func TestDocumentPageCountErrorWrapsCause(t *testing.T) {
	cause := errors.New("bad pdf")
	err := &DocumentPageCountError{Path: "/tmp/input.pdf", Err: cause}
	if got := err.Error(); got != `could not count pages in document "/tmp/input.pdf": bad pdf` {
		t.Fatalf("unexpected message: %q", got)
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is must find the wrapped cause")
	}
}

func TestDocumentRenderErrorWrapsCause(t *testing.T) {
	cause := errors.New("rasterize failed")
	err := &DocumentRenderError{Path: "/tmp/input.pdf", Err: cause}
	if got := err.Error(); got != `could not render PDF "/tmp/input.pdf": rasterize failed` {
		t.Fatalf("unexpected message: %q", got)
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is must find the wrapped cause")
	}
}

func TestDocumentExtractionErrorWrapsCause(t *testing.T) {
	cause := errors.New("extract failed")
	err := &DocumentExtractionError{Path: "/tmp/input.pdf", Err: cause}
	if got := err.Error(); got != `could not extract document text "/tmp/input.pdf": extract failed` {
		t.Fatalf("unexpected message: %q", got)
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is must find the wrapped cause")
	}
}

func TestDocumentErrorsAreDistinctTypes(t *testing.T) {
	// The public error types must be distinguishable with errors.As.
	cause := errors.New("cause")
	checks := []struct {
		err    error
		target any
	}{
		{&DocumentConversionError{Err: cause}, &DocumentConversionError{}},
		{&DocumentPageCountError{Err: cause}, &DocumentPageCountError{}},
		{&DocumentRenderError{Err: cause}, &DocumentRenderError{}},
		{&DocumentExtractionError{Err: cause}, &DocumentExtractionError{}},
	}
	for _, check := range checks {
		if !errors.As(check.err, &check.target) {
			t.Fatalf("errors.As failed for %T", check.err)
		}
	}
}

func TestErrorMessagesContainDiagnostics(t *testing.T) {
	conversion := &DocumentConversionError{Path: "p", Stdout: "stdout", Stderr: "stderr", Err: errors.New("boom")}
	if !strings.Contains(conversion.Error(), "boom") {
		t.Fatalf("conversion error must include the cause: %q", conversion.Error())
	}
}
