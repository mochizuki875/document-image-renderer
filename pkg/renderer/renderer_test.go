package renderer

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
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
