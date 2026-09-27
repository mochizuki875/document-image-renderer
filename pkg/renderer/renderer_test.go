package renderer

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRenderDocumentRendersEveryPDFPage(t *testing.T) {
	outputDirectory := t.TempDir()
	options := DefaultRenderOptions()
	options.DPI = 144

	result, err := RenderDocument(
		context.Background(),
		fixturePath("samplefile.pdf"),
		outputDirectory,
		&options,
	)
	if err != nil {
		t.Fatalf("render PDF: %v", err)
	}
	if result.PageCount() == 0 {
		t.Fatal("expected at least one rendered page")
	}
	for index, rendered := range result.Images {
		if rendered.PageNumber != index+1 {
			t.Fatalf("unexpected page number: %d", rendered.PageNumber)
		}
		expectedName := filepath.Join(outputDirectory, fmt.Sprintf("samplefile-page-%04d.png", index+1))
		if rendered.Path != expectedName {
			t.Fatalf("unexpected output path: %s", rendered.Path)
		}
		assertDecodableImage(t, rendered)
	}
}

func TestPageCountCountsPDFWithoutRendering(t *testing.T) {
	pageCount, err := PageCount(context.Background(), fixturePath("samplefile.pdf"))
	if err != nil {
		t.Fatalf("count PDF pages: %v", err)
	}
	if pageCount == 0 {
		t.Fatal("expected at least one page")
	}
}

func TestDocumentAPIsRejectPDFExceedingByteLimit(t *testing.T) {
	renderOptions := DefaultRenderOptions()
	renderOptions.MaxPDFBytes = 1
	extractOptions := DefaultExtractOptions()
	extractOptions.MaxPDFBytes = 1
	checks := []func() error{
		func() error {
			_, err := RenderDocument(context.Background(), fixturePath("samplefile.pdf"), t.TempDir(), &renderOptions)
			return err
		},
		func() error {
			_, err := PageCountWithOptions(context.Background(), fixturePath("samplefile.pdf"), &extractOptions)
			return err
		},
		func() error {
			_, err := ExtractDocumentWithOptions(context.Background(), fixturePath("samplefile.pdf"), &extractOptions)
			return err
		},
	}
	for _, check := range checks {
		err := check()
		var exceeded *PDFSizeLimitExceededError
		if !errors.As(err, &exceeded) || exceeded.MaxBytes != 1 || exceeded.Bytes <= exceeded.MaxBytes {
			t.Fatalf("expected PDFSizeLimitExceededError, got %v", err)
		}
	}
}

func TestDocumentAPIsReturnCanceledContextPromptly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	checks := []func() error{
		func() error { _, err := PageCount(ctx, fixturePath("samplefile.pdf")); return err },
		func() error {
			_, err := RenderDocument(ctx, fixturePath("samplefile.pdf"), t.TempDir(), nil)
			return err
		},
		func() error {
			_, err := ExtractDocumentWithOptions(ctx, fixturePath("samplefile.pdf"), nil)
			return err
		},
	}
	for _, check := range checks {
		if err := check(); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	}
}

func TestRenderDocumentAppliesRenderTimeout(t *testing.T) {
	renderOptions := DefaultRenderOptions()
	renderOptions.RenderTimeout = time.Nanosecond
	_, err := RenderDocument(context.Background(), fixturePath("samplefile.pdf"), t.TempDir(), &renderOptions)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestRenderDocumentCancellationInterruptsStuckPDFium(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString(bug451265PDF)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "bug_451265.pdf")
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, renderErr := RenderDocument(ctx, source, t.TempDir(), nil)
		done <- renderErr
	}()
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	<-timer.C
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("PDFium did not stop within five seconds")
	}
}

func TestRenderDocumentRejectsHugeMediaBoxBeforeRendering(t *testing.T) {
	source := filepath.Join(t.TempDir(), "huge.pdf")
	if err := os.WriteFile(source, []byte(hugeMediaBoxPDF), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		format      ImageFormat
		transparent bool
	}{
		{name: "opaque PNG", format: ImageFormatPNG},
		{name: "transparent PNG", format: ImageFormatPNG, transparent: true},
		{name: "JPEG", format: ImageFormatJPEG},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := DefaultRenderOptions()
			options.DPI = 72
			options.MaxPageWidth = 1_000
			options.ImageFormat = test.format
			options.TransparentBackground = test.transparent
			_, err := RenderDocument(context.Background(), source, t.TempDir(), &options)
			var exceeded *PageSizeLimitExceededError
			if !errors.As(err, &exceeded) {
				t.Fatalf("expected PageSizeLimitExceededError, got %v", err)
			}
		})
	}
}

func TestPageCountWrapsPDFErrors(t *testing.T) {
	source := filepath.Join(t.TempDir(), "invalid.pdf")
	if err := os.WriteFile(source, []byte("not a PDF"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := PageCount(context.Background(), source)
	var countError *DocumentPageCountError
	if !errors.As(err, &countError) {
		t.Fatalf("expected DocumentPageCountError, got %v", err)
	}
}

func TestRenderDocumentRejectsPDFExceedingMaxPages(t *testing.T) {
	options := DefaultRenderOptions()
	options.MaxPages = 1

	_, err := RenderDocument(
		context.Background(),
		fixturePath("samplefile.pdf"),
		t.TempDir(),
		&options,
	)
	var exceeded *PageLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected PageLimitExceededError, got %v", err)
	}
	if exceeded.MaxPages != options.MaxPages || exceeded.PageCount <= exceeded.MaxPages {
		t.Fatalf("unexpected page limit error: %+v", exceeded)
	}
}

func TestRenderDocumentRejectsDocumentPixelLimit(t *testing.T) {
	options := DefaultRenderOptions()
	options.DPI = 72
	options.MaxDocumentPixels = 1

	_, err := RenderDocument(
		context.Background(),
		fixturePath("samplefile.pdf"),
		t.TempDir(),
		&options,
	)
	var exceeded *DocumentPixelLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected DocumentPixelLimitExceededError, got %v", err)
	}
}

func TestRenderDocumentRejectsPagePixelLimit(t *testing.T) {
	options := DefaultRenderOptions()
	options.DPI = 72
	options.MaxPagePixels = 1

	_, err := RenderDocument(
		context.Background(),
		fixturePath("samplefile.pdf"),
		t.TempDir(),
		&options,
	)
	var exceeded *PageSizeLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected PageSizeLimitExceededError, got %v", err)
	}
}

func TestRenderDocumentRejectsPageWidthLimit(t *testing.T) {
	options := DefaultRenderOptions()
	options.DPI = 72
	options.MaxPageWidth = 1

	_, err := RenderDocument(
		context.Background(),
		fixturePath("samplefile.pdf"),
		t.TempDir(),
		&options,
	)
	var exceeded *PageSizeLimitExceededError
	if !errors.As(err, &exceeded) || exceeded.Dimension != "width" {
		t.Fatalf("expected width limit error, got %v", err)
	}
}

func TestRenderDocumentWrapsRenderErrors(t *testing.T) {
	source := filepath.Join(t.TempDir(), "invalid.pdf")
	if err := os.WriteFile(source, []byte("not a PDF"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := RenderDocument(context.Background(), source, t.TempDir(), nil)
	var renderError *DocumentRenderError
	if !errors.As(err, &renderError) {
		t.Fatalf("expected DocumentRenderError, got %v", err)
	}
}

func TestRenderDocumentWritesJPEGWithCustomPrefix(t *testing.T) {
	options := DefaultRenderOptions()
	options.ImageFormat = ImageFormatJPEG
	options.JPEGQuality = 80
	options.FilenamePrefix = "preview"

	result, err := RenderDocument(
		context.Background(),
		fixturePath("samplefile.pdf"),
		t.TempDir(),
		&options,
	)
	if err != nil {
		t.Fatalf("render JPEG: %v", err)
	}
	if filepath.Ext(result.Images[0].Path) != ".jpg" || filepath.Base(result.Images[0].Path) != "preview-page-0001.jpg" {
		t.Fatalf("unexpected JPEG path: %s", result.Images[0].Path)
	}
	assertDecodableImage(t, result.Images[0])
}

func TestRenderDocumentPreservesTransparentPNGBackground(t *testing.T) {
	options := DefaultRenderOptions()
	options.TransparentBackground = true

	result, err := RenderDocument(
		context.Background(),
		fixturePath("samplefile.pdf"),
		t.TempDir(),
		&options,
	)
	if err != nil {
		t.Fatalf("render transparent PNG: %v", err)
	}
	input, err := os.Open(result.Images[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	decoded, _, err := image.Decode(input)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, alpha := decoded.At(0, 0).RGBA()
	if alpha != 0 {
		t.Fatalf("expected transparent page background, got alpha %d", alpha)
	}
}

func TestRenderDocumentRejectsUnsupportedExtension(t *testing.T) {
	source := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(source, []byte("not a document"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := RenderDocument(context.Background(), source, t.TempDir(), nil)
	var unsupported *UnsupportedFormatError
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected UnsupportedFormatError, got %v", err)
	}
}

func TestSupportedExtensionsReturnsLexicalOrder(t *testing.T) {
	got := SupportedExtensions()
	want := []string{".doc", ".docx", ".pdf", ".ppt", ".pptx", ".xls", ".xlsm", ".xlsx"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SupportedExtensions() = %v, want %v", got, want)
	}
}

func TestValidateDocumentRejectsNilContext(t *testing.T) {
	_, _, err := validateDocument(nil, "input.pdf")
	if err == nil || !strings.Contains(err.Error(), "context must not be nil") {
		t.Fatalf("expected nil context error, got %v", err)
	}
}

func TestValidateDocumentRejectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := validateDocument(ctx, "input.pdf")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestValidateDocumentRejectsMissingFile(t *testing.T) {
	_, _, err := validateDocument(context.Background(), filepath.Join(t.TempDir(), "missing.pdf"))
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing file error, got %v", err)
	}
}

func TestValidateDocumentRejectsDirectory(t *testing.T) {
	_, _, err := validateDocument(context.Background(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected non-regular file error, got %v", err)
	}
}

func TestValidateDocumentRejectsUnsupportedExtension(t *testing.T) {
	source := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(source, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := validateDocument(context.Background(), source)
	var unsupported *UnsupportedFormatError
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected UnsupportedFormatError, got %v", err)
	}
}

func TestValidateDocumentAcceptsSupportedExtensionCaseInsensitively(t *testing.T) {
	source := filepath.Join(t.TempDir(), "input.PDF")
	if err := os.WriteFile(source, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, extension, err := validateDocument(context.Background(), source)
	if err != nil {
		t.Fatalf("validate document: %v", err)
	}
	if extension != ".pdf" {
		t.Fatalf("extension = %q, want .pdf", extension)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("path must be absolute: %s", path)
	}
}

func TestWithRenderTimeoutRejectsNilContext(t *testing.T) {
	_, _, err := withRenderTimeout(nil, 0)
	if err == nil || !strings.Contains(err.Error(), "context must not be nil") {
		t.Fatalf("expected nil context error, got %v", err)
	}
}

func TestWithRenderTimeoutAppliesDeadline(t *testing.T) {
	ctx, cancel, err := withRenderTimeout(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		t.Fatal("zero timeout must not set a deadline")
	}
}

func TestRenderDocumentRejectsNilContext(t *testing.T) {
	_, err := RenderDocument(nil, fixturePath("samplefile.pdf"), t.TempDir(), nil)
	if err == nil || !strings.Contains(err.Error(), "context must not be nil") {
		t.Fatalf("expected nil context error, got %v", err)
	}
}

func TestPageCountRejectsNilContext(t *testing.T) {
	_, err := PageCount(nil, fixturePath("samplefile.pdf"))
	if err == nil || !strings.Contains(err.Error(), "context must not be nil") {
		t.Fatalf("expected nil context error, got %v", err)
	}
}

func TestExtractDocumentRejectsNilContext(t *testing.T) {
	_, err := ExtractDocument(nil, fixturePath("samplefile.pdf"))
	if err == nil || !strings.Contains(err.Error(), "context must not be nil") {
		t.Fatalf("expected nil context error, got %v", err)
	}
}

func TestRenderDocumentRejectsMissingSource(t *testing.T) {
	_, err := RenderDocument(context.Background(), filepath.Join(t.TempDir(), "missing.pdf"), t.TempDir(), nil)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing file error, got %v", err)
	}
}

func TestRenderDocumentRejectsDirectorySource(t *testing.T) {
	_, err := RenderDocument(context.Background(), t.TempDir(), t.TempDir(), nil)
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected non-regular file error, got %v", err)
	}
}

func TestRenderDocumentCreatesOutputDirectory(t *testing.T) {
	outputDirectory := filepath.Join(t.TempDir(), "nested", "output")
	result, err := RenderDocument(context.Background(), fixturePath("samplefile.pdf"), outputDirectory, nil)
	if err != nil {
		t.Fatalf("render PDF: %v", err)
	}
	if result.PageCount() == 0 {
		t.Fatal("expected at least one rendered page")
	}
	if _, err := os.Stat(outputDirectory); err != nil {
		t.Fatalf("output directory was not created: %v", err)
	}
}

func TestRenderDocumentDefaultsPrefixToSourceName(t *testing.T) {
	outputDirectory := t.TempDir()
	result, err := RenderDocument(context.Background(), fixturePath("samplefile.pdf"), outputDirectory, nil)
	if err != nil {
		t.Fatalf("render PDF: %v", err)
	}
	if filepath.Base(result.Images[0].Path) != "samplefile-page-0001.png" {
		t.Fatalf("unexpected default prefix: %s", result.Images[0].Path)
	}
}

func fixturePath(name string) string {
	return filepath.Join("..", "..", "test", "documents", name)
}

func assertDecodableImage(t *testing.T, rendered RenderedImage) {
	t.Helper()
	input, err := os.Open(rendered.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	decoded, _, err := image.Decode(input)
	if err != nil {
		t.Fatalf("decode rendered image: %v", err)
	}
	if decoded.Bounds().Dx() != rendered.Width || decoded.Bounds().Dy() != rendered.Height {
		t.Fatalf("metadata dimensions do not match image: %+v", rendered)
	}
}

const bug451265PDF = "JVBERi0xLjcKJaDypPQKMSAwIG9iaiA8PAogIC9LaWRzIFszIDAgUl0KICAvVHlwZSAvUGFnZXMKICAvQ291bnQgMQo+PgplbmRvYmoKMiAwIG9iaiA8PAogIC9UeXBlIC9DYXRhbG9nCiAgL1BhZ2VzIDEgMCBSCj4+CmVuZG9iagozIDAgb2JqIDw8CiAgL1Jlc291cmNlcyAxMSAwIFIKICAvVHlwZSAvUGFnZQogIC9Db250ZW50cyAxMCAwIFIKICAvUGFyZW50IDEgMCBSCj4+CmVuZG9iagoxMCAwIG9iaiA8PAo+PgpzdHJlYW0KMSAwIDAgMSAzMTUuNzc5IDczMy4wMzkgY20KMSAwIDAgMSAyLjgzNSAtMTczLjYxNCBjbQoxLjA0NzA0IDAgMCAxLjA0NzA0IDAgMCBjbQovSW02IERvCmVuZHN0cmVhbQplbmRvYmoKMTEgMCBvYmogPDwKICAvWE9iamVjdCA8PAogICAgL0ltNiAxMiAwIFIKICA+Pgo+PgplbmRvYmoKMTIgMCBvYmogPDwKICAvU3VidHlwZSAvRm9ybQogIC9SZXNvdXJjZXMgPDwKICAgIC9YT2JqZWN0IDw8CiAgICAgIC94MTUgMTMgMCBSCiAgICA+PgogID4+Cj4+CnN0cmVhbQoveDE1IERvCmVuZHN0cmVhbQplbmRvYmoKMTMgMCBvYmogPDwKICAvU3VidHlwZSAvRm9ybQogIC9SZXNvdXJjZXMgPDwKICAgIC9QYXR0ZXJuIDw8CiAgICAgIC9wMzEgMTQgMCBSCiAgICA+PgogID4+Cj4+CnN0cmVhbQpxIC9QYXR0ZXJuIGNzIC9wMzEgc2NuIC9hMCBncwowIDAgMjI0LjcyMDAwMSAxNjAuMzk5OTk0IHJlIGYKUQplbmRzdHJlYW0KZW5kb2JqCjE0IDAgb2JqIDw8CiAgL1BhdHRlcm5UeXBlIDEKICAvQkJveCBbMCAwIDIyNSAxNjFdCiAgL1Jlc291cmNlcyA8PAogICAgL1hPYmplY3QgPDwKICAgICAgL3g0NyAxMiAwIFIKICAgID4+CiAgPj4KPj4Kc3RyZWFtCi94NDcgRG8KZW5kc3RyZWFtCmVuZG9iagp4cmVmCjAgMTUKMDAwMDAwMDAwMCA2NTUzNSBmIAowMDAwMDAwMDE1IDAwMDAwIG4gCjAwMDAwMDAwNzggMDAwMDAgbiAKMDAwMDAwMDEzMSAwMDAwMCBuIAowMDAwMDAwMDAwIDY1NTM1IGYgCjAwMDAwMDAwMDAgNjU1MzUgZiAKMDAwMDAwMDAwMCA2NTUzNSBmIAowMDAwMDAwMDAgNjU1MzUgZiAKMDAwMDAwMDAwMCA2NTUzNSBmIAowMDAwMDAwMDAgNjU1MzUgZiAKMDAwMDAwMDIyMSAwMDAwMCBuIAowMDAwMDAwMzQ4IDAwMDAwIG4gCjAwMDAwMDQwNSAwMDAwMCBuIAowMDAwMDA1MzEgMDAwMDAgbiAKMDAwMDAwMDcxMiAwMDAwMCBuIAp0cmFpbGVyIDw8CiAgL1Jvb3QgMiAwIFIKICAvU2l6ZSAxNQo+PgpzdGFydHhyZWYKNzg0CiUlRU9GCg=="

const hugeMediaBoxPDF = "%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 60000 60000]>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n"
