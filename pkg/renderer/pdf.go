package renderer

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
	wazeroapi "github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

// renderPDF rasterizes every page of a PDF into the output directory and
// returns the generated images in page order.
func renderPDF(
	ctx context.Context,
	pdfPath string,
	outputDirectory string,
	prefix string,
	options RenderOptions,
) ([]RenderedImage, error) {
	images, err := withPDFDocument(ctx, pdfPath, options.MaxPDFBytes, func(instance pdfium.Pdfium, document references.FPDF_DOCUMENT, pageCount int) ([]RenderedImage, error) {
		if options.MaxPages > 0 && pageCount > options.MaxPages {
			return nil, &PageLimitExceededError{PageCount: pageCount, MaxPages: options.MaxPages}
		}
		// Pad page numbers to at least four digits so file names sort correctly.
		pageDigits := max(4, len(fmt.Sprintf("%d", pageCount)))
		images := make([]RenderedImage, 0, pageCount)
		var documentPixels uint64
		for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			imagePath := filepath.Join(
				outputDirectory,
				fmt.Sprintf("%s-page-%0*d.%s", prefix, pageDigits, pageIndex+1, imageExtension(options.ImageFormat)),
			)
			size, pixels, err := checkedPageSize(instance, document, pageIndex, options)
			if err != nil {
				return nil, err
			}
			if documentPixels > math.MaxUint64-pixels {
				return nil, &DocumentPixelLimitExceededError{Pixels: math.MaxUint64, MaxPixels: options.MaxDocumentPixels}
			}
			if options.MaxDocumentPixels > 0 && (documentPixels > options.MaxDocumentPixels || pixels > options.MaxDocumentPixels-documentPixels) {
				return nil, &DocumentPixelLimitExceededError{Pixels: documentPixels + pixels, MaxPixels: options.MaxDocumentPixels}
			}
			documentPixels += pixels
			rendered, err := renderPage(ctx, instance, document, pageIndex, imagePath, size.Width, size.Height, options)
			if err != nil {
				return nil, fmt.Errorf("page %d: %w", pageIndex+1, err)
			}
			images = append(images, rendered)
		}
		return images, nil
	})
	if err != nil {
		return nil, &DocumentRenderError{Path: pdfPath, Err: err}
	}
	return images, nil
}

func pdfPageCount(ctx context.Context, source string, maxPDFBytes uint64) (int, error) {
	return withPDFDocument(ctx, source, maxPDFBytes, func(_ pdfium.Pdfium, _ references.FPDF_DOCUMENT, pageCount int) (int, error) {
		return pageCount, nil
	})
}

// extractPDFText extracts the text of every PDF page, one TextPart per page.
func extractPDFText(ctx context.Context, source string, maxPDFBytes uint64, budget *characterBudget) ([]TextPart, error) {
	parts, err := withPDFDocument(ctx, source, maxPDFBytes, func(instance pdfium.Pdfium, document references.FPDF_DOCUMENT, pageCount int) ([]TextPart, error) {
		parts := make([]TextPart, 0, pageCount)
		for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			pageText, err := instance.GetPageText(&requests.GetPageText{Page: requests.Page{ByIndex: &requests.PageByIndex{
				Document: document, Index: pageIndex,
			}}})
			if err != nil {
				return nil, fmt.Errorf("page %d: %w", pageIndex+1, err)
			}
			var text strings.Builder
			if err := budget.append(&text, pageText.Text); err != nil {
				return nil, err
			}
			parts = append(parts, TextPart{PartNumber: pageIndex + 1, Text: text.String()})
		}
		return parts, nil
	})
	if err != nil {
		return nil, &DocumentExtractionError{Path: source, Err: err}
	}
	return parts, nil
}

// withPDFDocument opens a PDF with PDFium, runs use with the document handle
// and page count, and guarantees that all PDFium resources are released.
func withPDFDocument[T any](
	ctx context.Context,
	path string,
	maxPDFBytes uint64,
	use func(pdfium.Pdfium, references.FPDF_DOCUMENT, int) (T, error),
) (T, error) {
	var zero T
	data, err := readPDF(ctx, path, maxPDFBytes)
	if err != nil {
		return zero, err
	}
	// A single-instance pool keeps memory usage low; PDFium is used serially.
	features := wazeroapi.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling
	pool, err := webassembly.Init(webassembly.Config{
		Context: ctx, MinIdle: 1, MaxIdle: 1, MaxTotal: 1,
		RuntimeConfig: wazero.NewRuntimeConfig().WithCoreFeatures(features).WithCloseOnContextDone(true),
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return zero, ctxErr
		}
		return zero, fmt.Errorf("initialize PDFium: %w", err)
	}
	defer pool.Close()
	instance, err := pool.GetInstanceWithContext(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return zero, ctxErr
		}
		return zero, fmt.Errorf("acquire PDFium instance: %w", err)
	}
	operationDone := make(chan struct{})
	watchDone := make(chan struct{})
	var killed atomic.Bool
	go func() {
		defer close(watchDone)
		select {
		case <-ctx.Done():
			killed.Store(true)
			_ = instance.Kill()
		case <-operationDone:
		}
	}()
	defer func() {
		close(operationDone)
		<-watchDone
		if !killed.Load() {
			_ = instance.Close()
		}
	}()
	document, err := instance.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return zero, ctxErr
		}
		return zero, fmt.Errorf("open document: %w", err)
	}
	defer func() {
		if !killed.Load() {
			_, _ = instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: document.Document})
		}
	}()
	pageCount, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: document.Document})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return zero, ctxErr
		}
		return zero, fmt.Errorf("get page count: %w", err)
	}
	result, err := use(instance, document.Document, pageCount.PageCount)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return zero, ctxErr
	}
	return result, err
}

func readPDF(ctx context.Context, path string, maxBytes uint64) ([]byte, error) {
	input, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && info.Size() >= 0 && uint64(info.Size()) > maxBytes {
		return nil, &PDFSizeLimitExceededError{Bytes: uint64(info.Size()), MaxBytes: maxBytes}
	}
	reader := io.Reader(&contextReader{ctx: ctx, reader: input})
	if maxBytes > 0 {
		limit := maxBytes + 1
		if limit == 0 || limit > math.MaxInt64 {
			limit = math.MaxInt64
		}
		reader = io.LimitReader(reader, int64(limit))
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && uint64(len(data)) > maxBytes {
		return nil, &PDFSizeLimitExceededError{Bytes: uint64(len(data)), MaxBytes: maxBytes}
	}
	return data, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	count, err := reader.reader.Read(buffer)
	if ctxErr := reader.ctx.Err(); ctxErr != nil {
		return count, ctxErr
	}
	return count, err
}

func checkedPageSize(instance pdfium.Pdfium, document references.FPDF_DOCUMENT, pageIndex int, options RenderOptions) (*responses.GetPageSizeInPixels, uint64, error) {
	page := requests.Page{ByIndex: &requests.PageByIndex{Document: document, Index: pageIndex}}
	size, err := instance.GetPageSizeInPixels(&requests.GetPageSizeInPixels{Page: page, DPI: options.DPI})
	if err != nil {
		return nil, 0, err
	}
	if size.Width <= 0 || size.Height <= 0 {
		return nil, 0, fmt.Errorf("invalid page dimensions %dx%d", size.Width, size.Height)
	}
	pixels, err := validatePageSize(pageIndex+1, size.Width, size.Height, options)
	if err != nil {
		return nil, 0, err
	}
	return size, pixels, nil
}

func validatePageSize(pageNumber, width, height int, options RenderOptions) (uint64, error) {
	if options.MaxPageWidth > 0 && width > options.MaxPageWidth {
		return 0, &PageSizeLimitExceededError{PageNumber: pageNumber, Width: width, Height: height, Limit: uint64(options.MaxPageWidth), Dimension: "width"}
	}
	if options.MaxPageHeight > 0 && height > options.MaxPageHeight {
		return 0, &PageSizeLimitExceededError{PageNumber: pageNumber, Width: width, Height: height, Limit: uint64(options.MaxPageHeight), Dimension: "height"}
	}
	if uint64(width) > math.MaxUint64/uint64(height) {
		return 0, &PageSizeLimitExceededError{PageNumber: pageNumber, Width: width, Height: height, Pixels: math.MaxUint64, Limit: options.MaxPagePixels, Dimension: "pixel count"}
	}
	pixels := uint64(width) * uint64(height)
	if options.MaxPagePixels > 0 && pixels > options.MaxPagePixels {
		return 0, &PageSizeLimitExceededError{PageNumber: pageNumber, Width: width, Height: height, Pixels: pixels, Limit: options.MaxPagePixels, Dimension: "pixel count"}
	}
	return pixels, nil
}

// renderPage rasterizes a single PDF page to imagePath and returns its metadata.
// With TransparentBackground the page is rendered onto an alpha bitmap;
// otherwise the page is composited onto a white background.
func renderPage(
	ctx context.Context,
	instance pdfium.Pdfium,
	document references.FPDF_DOCUMENT,
	pageIndex int,
	imagePath string,
	width int,
	height int,
	options RenderOptions,
) (RenderedImage, error) {
	var renderedImage image.Image
	cleanup := func() {}
	if options.TransparentBackground {
		var err error
		renderedImage, err = renderTransparentPage(instance, document, pageIndex, width, height)
		if err != nil {
			return RenderedImage{}, fmt.Errorf("render: %w", err)
		}
	} else {
		page, err := instance.RenderPageInDPI(&requests.RenderPageInDPI{
			DPI: options.DPI,
			Page: requests.Page{ByIndex: &requests.PageByIndex{
				Document: document,
				Index:    pageIndex,
			}},
			Document:   &document,
			RenderForm: true,
		})
		if page != nil {
			cleanup = page.Cleanup
		}
		defer cleanup()
		if err != nil {
			return RenderedImage{}, fmt.Errorf("render: %w", err)
		}
		if page == nil || page.Result.Image == nil {
			return RenderedImage{}, fmt.Errorf("render: PDFium returned no image")
		}
		renderedImage = compositeOnWhite(page.Result.Image)
	}

	if err := saveImage(ctx, imagePath, renderedImage, options); err != nil {
		return RenderedImage{}, fmt.Errorf("save: %w", err)
	}
	return RenderedImage{
		PageNumber: pageIndex + 1,
		Path:       imagePath,
		Width:      renderedImage.Bounds().Dx(),
		Height:     renderedImage.Bounds().Dy(),
	}, nil
}

func imageExtension(format ImageFormat) string {
	if format == ImageFormatJPEG {
		return "jpg"
	}
	return "png"
}

// renderTransparentPage renders a page into an RGBA bitmap with a fully
// transparent background. The bitmap buffer is copied because PDFium reuses
// the underlying memory after the bitmap is destroyed.
func renderTransparentPage(
	instance pdfium.Pdfium,
	document references.FPDF_DOCUMENT,
	pageIndex int,
	width int,
	height int,
) (*image.RGBA, error) {
	page := requests.Page{ByIndex: &requests.PageByIndex{Document: document, Index: pageIndex}}
	bitmap, err := instance.FPDFBitmap_Create(&requests.FPDFBitmap_Create{
		Width: width, Height: height, Alpha: 1,
	})
	if err != nil {
		return nil, err
	}
	defer instance.FPDFBitmap_Destroy(&requests.FPDFBitmap_Destroy{Bitmap: bitmap.Bitmap})
	// Fill with transparent black before rendering so untouched pixels stay clear.
	if _, err := instance.FPDFBitmap_FillRect(&requests.FPDFBitmap_FillRect{
		Bitmap: bitmap.Bitmap, Width: width, Height: height, Color: 0x00000000,
	}); err != nil {
		return nil, err
	}
	if _, err := instance.FPDF_RenderPageBitmap(&requests.FPDF_RenderPageBitmap{
		Bitmap: bitmap.Bitmap,
		Page:   page,
		SizeX:  width,
		SizeY:  height,
		Flags:  enums.FPDF_RENDER_FLAG_REVERSE_BYTE_ORDER | enums.FPDF_RENDER_FLAG_ANNOT,
	}); err != nil {
		return nil, err
	}
	buffer, err := instance.FPDFBitmap_GetBuffer(&requests.FPDFBitmap_GetBuffer{Bitmap: bitmap.Bitmap})
	if err != nil {
		return nil, err
	}
	stride, err := instance.FPDFBitmap_GetStride(&requests.FPDFBitmap_GetStride{Bitmap: bitmap.Bitmap})
	if err != nil {
		return nil, err
	}
	pixels := append([]byte(nil), buffer.Buffer...)
	if stride.Stride < 0 || uint64(stride.Stride)*uint64(height) > uint64(len(pixels)) {
		return nil, fmt.Errorf("PDFium returned an incomplete bitmap")
	}
	return &image.RGBA{
		Pix: pixels, Stride: stride.Stride, Rect: image.Rect(0, 0, width, height),
	}, nil
}

// compositeOnWhite draws the rendered page over a white background so that
// transparent areas of the PDF appear white in the output image.
func compositeOnWhite(source image.Image) image.Image {
	bounds := source.Bounds()
	output := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(output, output.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(output, output.Bounds(), source, bounds.Min, draw.Over)
	return output
}

// saveImage encodes to a temporary file and publishes it only after a complete,
// uncanceled encode.
func saveImage(ctx context.Context, path string, source image.Image, options RenderOptions) (returnErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	output, err := os.CreateTemp(filepath.Dir(path), ".document-image-renderer-*")
	if err != nil {
		return err
	}
	temporaryPath := output.Name()
	committed := false
	defer func() {
		_ = output.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()

	writer := &contextWriter{ctx: ctx, writer: output}
	if options.ImageFormat == ImageFormatJPEG {
		err = jpeg.Encode(writer, source, &jpeg.Options{Quality: options.JPEGQuality})
	} else {
		err = png.Encode(writer, source)
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}

type contextWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (writer *contextWriter) Write(data []byte) (int, error) {
	if err := writer.ctx.Err(); err != nil {
		return 0, err
	}
	count, err := writer.writer.Write(data)
	if ctxErr := writer.ctx.Err(); ctxErr != nil {
		return count, ctxErr
	}
	return count, err
}
