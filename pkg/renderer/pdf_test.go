package renderer

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

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

func TestReadPDFRejectsMissingFile(t *testing.T) {
	if _, err := readPDF(context.Background(), filepath.Join(t.TempDir(), "missing.pdf"), 0); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadPDFRejectsExceedingLimitFromStat(t *testing.T) {
	source := filepath.Join(t.TempDir(), "input.pdf")
	if err := os.WriteFile(source, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readPDF(context.Background(), source, 4)
	var exceeded *PDFSizeLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected PDFSizeLimitExceededError, got %v", err)
	}
}

func TestReadPDFRejectsExceedingLimitFromRead(t *testing.T) {
	source := filepath.Join(t.TempDir(), "input.pdf")
	if err := os.WriteFile(source, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readPDF(context.Background(), source, 4)
	var exceeded *PDFSizeLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected PDFSizeLimitExceededError, got %v", err)
	}
}

func TestReadPDFStopsOnCancellation(t *testing.T) {
	source := filepath.Join(t.TempDir(), "input.pdf")
	if err := os.WriteFile(source, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := readPDF(ctx, source, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestContextReaderPropagatesReadError(t *testing.T) {
	reader := &contextReader{ctx: context.Background(), reader: &failingReader{}}
	if _, err := reader.Read(make([]byte, 10)); err == nil {
		t.Fatal("expected read error")
	}
}

func TestContextWriterStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	writer := &contextWriter{ctx: ctx, writer: &bytes.Buffer{}}
	cancel()
	if _, err := writer.Write([]byte("data")); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestContextWriterPropagatesWriteError(t *testing.T) {
	writer := &contextWriter{ctx: context.Background(), writer: &failingWriter{}}
	if _, err := writer.Write([]byte("data")); err == nil {
		t.Fatal("expected write error")
	}
}

func TestValidatePageSizeRejectsHeightLimit(t *testing.T) {
	options := DefaultRenderOptions()
	options.MaxPageHeight = 100
	_, err := validatePageSize(1, 10, 101, options)
	var exceeded *PageSizeLimitExceededError
	if !errors.As(err, &exceeded) || exceeded.Dimension != "height" {
		t.Fatalf("expected height limit error, got %v", err)
	}
}

func TestValidatePageSizeRejectsPixelLimit(t *testing.T) {
	options := DefaultRenderOptions()
	options.MaxPagePixels = 100
	_, err := validatePageSize(1, 20, 20, options)
	var exceeded *PageSizeLimitExceededError
	if !errors.As(err, &exceeded) || exceeded.Dimension != "pixel count" {
		t.Fatalf("expected pixel limit error, got %v", err)
	}
}

func TestValidatePageSizeRejectsOverflowingPixels(t *testing.T) {
	options := DefaultRenderOptions()
	options.MaxPageWidth = 0
	options.MaxPageHeight = 0
	options.MaxPagePixels = 0
	_, err := validatePageSize(1, math.MaxInt, 3, options)
	var exceeded *PageSizeLimitExceededError
	if !errors.As(err, &exceeded) || exceeded.Dimension != "pixel count" {
		t.Fatalf("expected pixel overflow error, got %v", err)
	}
}

func TestImageExtension(t *testing.T) {
	if got := imageExtension(ImageFormatJPEG); got != "jpg" {
		t.Fatalf("imageExtension(JPEG) = %q", got)
	}
	if got := imageExtension(ImageFormatPNG); got != "png" {
		t.Fatalf("imageExtension(PNG) = %q", got)
	}
}

func TestCompositeOnWhiteUsesWhiteBackground(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 2))
	// Leave the source fully transparent.
	output := compositeOnWhite(source)
	r, g, b, a := output.At(0, 0).RGBA()
	if r != 0xffff || g != 0xffff || b != 0xffff || a != 0xffff {
		t.Fatalf("expected white opaque background, got %d %d %d %d", r, g, b, a)
	}
}

func TestSaveImageRejectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	options := DefaultRenderOptions()
	err := saveImage(ctx, filepath.Join(t.TempDir(), "out.png"), image.NewRGBA(image.Rect(0, 0, 1, 1)), options)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestSaveImagePublishesCompleteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.png")
	options := DefaultRenderOptions()
	if err := saveImage(context.Background(), path, image.NewRGBA(image.Rect(0, 0, 4, 4)), options); err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	decoded, _, err := image.Decode(input)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 4 || decoded.Bounds().Dy() != 4 {
		t.Fatalf("unexpected decoded size: %v", decoded.Bounds())
	}
}

func TestSaveImageJPEGUsesConfiguredQuality(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jpg")
	options := DefaultRenderOptions()
	options.ImageFormat = ImageFormatJPEG
	options.JPEGQuality = 50
	if err := saveImage(context.Background(), path, image.NewRGBA(image.Rect(0, 0, 4, 4)), options); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
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
