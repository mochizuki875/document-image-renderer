package renderer

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
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

func TestReadPDFAcceptsExactByteLimit(t *testing.T) {
	source := filepath.Join(t.TempDir(), "input.pdf")
	content := []byte("1234")
	if err := os.WriteFile(source, content, 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := readPDF(context.Background(), source, uint64(len(content)))
	if err != nil || string(data) != string(content) {
		t.Fatalf("data=%q error=%v", data, err)
	}
}

func TestContextReaderStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := &contextReader{ctx: ctx, reader: bytes.NewReader([]byte("PDF data"))}
	buffer := make([]byte, 3)
	if _, err := reader.Read(buffer); err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := reader.Read(buffer); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestSaveImageCancellationDoesNotPublishPartialFile(t *testing.T) {
	for _, format := range []ImageFormat{ImageFormatPNG, ImageFormatJPEG} {
		t.Run(string(format), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			path := filepath.Join(t.TempDir(), "output."+imageExtension(format))
			original := []byte("complete previous output")
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			options := DefaultRenderOptions()
			options.ImageFormat = format
			source := cancelingImage{Image: image.NewRGBA(image.Rect(0, 0, 512, 512)), cancel: cancel}
			err := saveImage(ctx, path, source, options)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("expected context.Canceled, got %v", err)
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil || string(data) != string(original) {
				t.Fatalf("previous output changed: data=%q error=%v", data, readErr)
			}
			matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".document-image-renderer-*"))
			if err != nil || len(matches) != 0 {
				t.Fatalf("temporary outputs remain: %v, error=%v", matches, err)
			}
		})
	}
}

type cancelingImage struct {
	image.Image
	cancel context.CancelFunc
}

func (source cancelingImage) At(x, y int) color.Color {
	source.cancel()
	return source.Image.At(x, y)
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

func TestValidatePageSizeBoundariesAndOverflow(t *testing.T) {
	options := DefaultRenderOptions()
	options.MaxPageWidth = 100
	options.MaxPageHeight = 200
	options.MaxPagePixels = 20_000
	if pixels, err := validatePageSize(1, 100, 200, options); err != nil || pixels != 20_000 {
		t.Fatalf("boundary size rejected: pixels=%d error=%v", pixels, err)
	}
	if _, err := validatePageSize(1, 101, 200, options); err == nil {
		t.Fatal("width above limit was accepted")
	}
	options.MaxPageWidth, options.MaxPageHeight, options.MaxPagePixels = 0, 0, 0
	if _, err := validatePageSize(1, math.MaxInt, 3, options); err == nil {
		t.Fatal("overflowing pixel count was accepted")
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

func TestExtractDocumentExtractsEveryPDFPage(t *testing.T) {
	result, err := ExtractDocument(context.Background(), fixturePath("samplefile.pdf"))
	if err != nil {
		t.Fatalf("extract PDF text: %v", err)
	}
	if result.PartCount() == 0 {
		t.Fatal("expected at least one extracted page")
	}
	for index, part := range result.Parts {
		if part.PartNumber != index+1 {
			t.Fatalf("unexpected part number: %d", part.PartNumber)
		}
	}
}

func TestExtractDocumentRejectsTextExceedingMaxCharacters(t *testing.T) {
	options := DefaultExtractOptions()
	options.MaxCharacters = 1

	_, err := ExtractDocumentWithOptions(context.Background(), fixturePath("samplefile.pdf"), &options)
	var exceeded *CharacterLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected CharacterLimitExceededError, got %v", err)
	}
	if exceeded.MaxCharacters != options.MaxCharacters || exceeded.CharacterCount <= exceeded.MaxCharacters {
		t.Fatalf("unexpected character limit error: %+v", exceeded)
	}
}

func TestExtractDocumentExtractsModernOfficeText(t *testing.T) {
	for _, name := range []string{"samplefile.docx", "samplefile.pptx", "samplefile.xlsx", "samplefile.xlsm"} {
		t.Run(name, func(t *testing.T) {
			result, err := ExtractDocument(context.Background(), fixturePath(name))
			if err != nil {
				t.Fatalf("extract Office text: %v", err)
			}
			if result.PartCount() == 0 {
				t.Fatal("expected at least one extracted part")
			}
			for index, part := range result.Parts {
				if part.PartNumber != index+1 {
					t.Fatalf("unexpected part number: %d", part.PartNumber)
				}
				if strings.TrimSpace(part.Text) == "" {
					t.Fatalf("part %d contains no text", part.PartNumber)
				}
			}
		})
	}
}

func TestExtractDocumentExtractsLegacyOfficeText(t *testing.T) {
	replaceLibreOfficeFunctions(t)
	findExecutable = func(string) (string, error) {
		t.Fatal("explicit LibreOffice executable was not used")
		return "", nil
	}
	executeLibreOffice = func(_ context.Context, executable string, arguments []string) (string, string, error) {
		if executable != "/custom/libreoffice" {
			t.Fatalf("unexpected executable: %s", executable)
		}
		source := arguments[len(arguments)-1]
		targets := map[string]string{".doc": ".docx", ".ppt": ".pptx", ".xls": ".xlsx"}
		targetExtension := targets[filepath.Ext(source)]
		if filter := argumentAfter(t, arguments, "--convert-to"); filter != strings.TrimPrefix(targetExtension, ".") {
			t.Fatalf("unexpected conversion filter: %s", filter)
		}
		outputDirectory := argumentAfter(t, arguments, "--outdir")
		outputName := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source)) + targetExtension
		copyFixture(t, fixturePath("samplefile"+targetExtension), filepath.Join(outputDirectory, outputName))
		return "", "", nil
	}
	for _, name := range []string{"samplefile.doc", "samplefile.ppt", "samplefile.xls"} {
		t.Run(name, func(t *testing.T) {
			result, err := ExtractDocumentWithOptions(context.Background(), fixturePath(name), &ExtractOptions{
				LibreOfficeTimeout: 5 * time.Second, LibreOfficeExecutable: "/custom/libreoffice",
			})
			if err != nil {
				t.Fatalf("extract legacy Office text: %v", err)
			}
			if result.PartCount() == 0 {
				t.Fatal("expected at least one extracted part")
			}
			for _, part := range result.Parts {
				if strings.TrimSpace(part.Text) == "" {
					t.Fatalf("part %d contains no text", part.PartNumber)
				}
			}
		})
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
