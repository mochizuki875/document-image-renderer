package renderer

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
)

func renderPDF(
	ctx context.Context,
	pdfPath string,
	outputDirectory string,
	prefix string,
	options RenderOptions,
) ([]RenderedImage, error) {
	images, err := withPDFDocument(ctx, pdfPath, func(instance pdfium.Pdfium, document references.FPDF_DOCUMENT, pageCount int) ([]RenderedImage, error) {
		if options.MaxPages > 0 && pageCount > options.MaxPages {
			return nil, &PageLimitExceededError{PageCount: pageCount, MaxPages: options.MaxPages}
		}
		pageDigits := max(4, len(fmt.Sprintf("%d", pageCount)))
		images := make([]RenderedImage, 0, pageCount)
		for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			imagePath := filepath.Join(
				outputDirectory,
				fmt.Sprintf("%s-page-%0*d.%s", prefix, pageDigits, pageIndex+1, imageExtension(options.ImageFormat)),
			)
			rendered, err := renderPage(instance, document, pageIndex, imagePath, options)
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

func extractPDFText(ctx context.Context, source string) ([]TextPart, error) {
	parts, err := withPDFDocument(ctx, source, func(instance pdfium.Pdfium, document references.FPDF_DOCUMENT, pageCount int) ([]TextPart, error) {
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
			parts = append(parts, TextPart{PartNumber: pageIndex + 1, Text: pageText.Text})
		}
		return parts, nil
	})
	if err != nil {
		return nil, &DocumentExtractionError{Path: source, Err: err}
	}
	return parts, nil
}

func withPDFDocument[T any](
	ctx context.Context,
	path string,
	use func(pdfium.Pdfium, references.FPDF_DOCUMENT, int) (T, error),
) (T, error) {
	var zero T
	data, err := os.ReadFile(path)
	if err != nil {
		return zero, err
	}
	pool, err := webassembly.Init(webassembly.Config{MinIdle: 1, MaxIdle: 1, MaxTotal: 1})
	if err != nil {
		return zero, fmt.Errorf("initialize PDFium: %w", err)
	}
	defer pool.Close()
	instance, err := pool.GetInstanceWithContext(ctx)
	if err != nil {
		return zero, fmt.Errorf("acquire PDFium instance: %w", err)
	}
	defer instance.Close()
	document, err := instance.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return zero, fmt.Errorf("open document: %w", err)
	}
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: document.Document})
	pageCount, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: document.Document})
	if err != nil {
		return zero, fmt.Errorf("get page count: %w", err)
	}
	return use(instance, document.Document, pageCount.PageCount)
}

func renderPage(
	instance pdfium.Pdfium,
	document references.FPDF_DOCUMENT,
	pageIndex int,
	imagePath string,
	options RenderOptions,
) (RenderedImage, error) {
	var renderedImage image.Image
	cleanup := func() {}
	if options.TransparentBackground {
		var err error
		renderedImage, err = renderTransparentPage(instance, document, pageIndex, options.DPI)
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

	if err := saveImage(imagePath, renderedImage, options); err != nil {
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

func renderTransparentPage(
	instance pdfium.Pdfium,
	document references.FPDF_DOCUMENT,
	pageIndex int,
	dpi int,
) (*image.RGBA, error) {
	page := requests.Page{ByIndex: &requests.PageByIndex{Document: document, Index: pageIndex}}
	size, err := instance.GetPageSizeInPixels(&requests.GetPageSizeInPixels{Page: page, DPI: dpi})
	if err != nil {
		return nil, err
	}
	bitmap, err := instance.FPDFBitmap_Create(&requests.FPDFBitmap_Create{
		Width: size.Width, Height: size.Height, Alpha: 1,
	})
	if err != nil {
		return nil, err
	}
	defer instance.FPDFBitmap_Destroy(&requests.FPDFBitmap_Destroy{Bitmap: bitmap.Bitmap})
	if _, err := instance.FPDFBitmap_FillRect(&requests.FPDFBitmap_FillRect{
		Bitmap: bitmap.Bitmap, Width: size.Width, Height: size.Height, Color: 0x00000000,
	}); err != nil {
		return nil, err
	}
	if _, err := instance.FPDF_RenderPageBitmap(&requests.FPDF_RenderPageBitmap{
		Bitmap: bitmap.Bitmap,
		Page:   page,
		SizeX:  size.Width,
		SizeY:  size.Height,
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
	if len(pixels) < stride.Stride*size.Height {
		return nil, fmt.Errorf("PDFium returned an incomplete bitmap")
	}
	return &image.RGBA{
		Pix: pixels, Stride: stride.Stride, Rect: image.Rect(0, 0, size.Width, size.Height),
	}, nil
}

func compositeOnWhite(source image.Image) image.Image {
	bounds := source.Bounds()
	output := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(output, output.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(output, output.Bounds(), source, bounds.Min, draw.Over)
	return output
}

func saveImage(path string, source image.Image, options RenderOptions) (returnErr error) {
	output, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if err := output.Close(); returnErr == nil && err != nil {
			returnErr = err
		}
	}()

	if options.ImageFormat == ImageFormatJPEG {
		return jpeg.Encode(output, source, &jpeg.Options{Quality: options.JPEGQuality})
	}
	return png.Encode(output, source)
}
