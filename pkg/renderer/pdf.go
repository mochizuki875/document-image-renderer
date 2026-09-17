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
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, &DocumentRenderError{Path: pdfPath, Err: err}
	}
	pool, err := webassembly.Init(webassembly.Config{
		MinIdle:  1,
		MaxIdle:  1,
		MaxTotal: 1,
	})
	if err != nil {
		return nil, &DocumentRenderError{Path: pdfPath, Err: fmt.Errorf("initialize PDFium: %w", err)}
	}
	defer pool.Close()
	instance, err := pool.GetInstanceWithContext(ctx)
	if err != nil {
		return nil, &DocumentRenderError{Path: pdfPath, Err: fmt.Errorf("acquire PDFium instance: %w", err)}
	}
	defer instance.Close()
	document, err := instance.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return nil, &DocumentRenderError{Path: pdfPath, Err: fmt.Errorf("open document: %w", err)}
	}
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: document.Document})
	pageCountResponse, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: document.Document})
	if err != nil {
		return nil, &DocumentRenderError{Path: pdfPath, Err: fmt.Errorf("get page count: %w", err)}
	}

	pageCount := pageCountResponse.PageCount
	pageDigits := max(4, len(fmt.Sprintf("%d", pageCount)))
	images := make([]RenderedImage, 0, pageCount)
	for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
		if err := ctx.Err(); err != nil {
			return nil, &DocumentRenderError{Path: pdfPath, Err: err}
		}
		var renderedImage image.Image
		cleanup := func() {}
		if options.TransparentBackground {
			renderedImage, err = renderTransparentPage(instance, document.Document, pageIndex, options.DPI)
		} else {
			page, renderErr := instance.RenderPageInDPI(&requests.RenderPageInDPI{
				DPI: options.DPI,
				Page: requests.Page{ByIndex: &requests.PageByIndex{
					Document: document.Document,
					Index:    pageIndex,
				}},
				Document:   &document.Document,
				RenderForm: true,
			})
			err = renderErr
			if page != nil {
				cleanup = page.Cleanup
				renderedImage = page.Result.Image
			}
		}
		if err != nil {
			return nil, &DocumentRenderError{Path: pdfPath, Err: fmt.Errorf("render page %d: %w", pageIndex+1, err)}
		}
		if !options.TransparentBackground {
			renderedImage = compositeOnWhite(renderedImage)
		}

		extension := "png"
		if options.ImageFormat == ImageFormatJPEG {
			extension = "jpg"
		}
		imagePath := filepath.Join(
			outputDirectory,
			fmt.Sprintf("%s-page-%0*d.%s", prefix, pageDigits, pageIndex+1, extension),
		)
		if err := saveImage(imagePath, renderedImage, options); err != nil {
			cleanup()
			return nil, &DocumentRenderError{Path: pdfPath, Err: fmt.Errorf("save page %d: %w", pageIndex+1, err)}
		}
		images = append(images, RenderedImage{
			PageNumber: pageIndex + 1,
			Path:       imagePath,
			Width:      renderedImage.Bounds().Dx(),
			Height:     renderedImage.Bounds().Dy(),
		})
		cleanup()
	}
	return images, nil
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
