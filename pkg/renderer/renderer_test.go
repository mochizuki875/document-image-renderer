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
	"testing"
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
