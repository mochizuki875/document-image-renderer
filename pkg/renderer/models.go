package renderer

import (
	"fmt"
	"path/filepath"
	"time"
)

// ImageFormat identifies an output image encoding.
type ImageFormat string

const (
	ImageFormatPNG  ImageFormat = "png"
	ImageFormatJPEG ImageFormat = "jpeg"
)

// RenderOptions controls page rasterization and LibreOffice execution.
type RenderOptions struct {
	DPI                   int
	ImageFormat           ImageFormat
	JPEGQuality           int
	TransparentBackground bool
	FilenamePrefix        string
	LibreOfficeTimeout    time.Duration
	LibreOfficeExecutable string
}

// DefaultRenderOptions returns the default rendering configuration.
func DefaultRenderOptions() RenderOptions {
	return RenderOptions{
		DPI:                200,
		ImageFormat:        ImageFormatPNG,
		JPEGQuality:        90,
		LibreOfficeTimeout: 120 * time.Second,
	}
}

// Validate checks whether all option values can be rendered safely.
func (options RenderOptions) Validate() error {
	if options.DPI < 1 || options.DPI > 1200 {
		return fmt.Errorf("dpi must be between 1 and 1200")
	}
	if options.ImageFormat != ImageFormatPNG && options.ImageFormat != ImageFormatJPEG {
		return fmt.Errorf("image format must be %q or %q", ImageFormatPNG, ImageFormatJPEG)
	}
	if options.JPEGQuality < 1 || options.JPEGQuality > 100 {
		return fmt.Errorf("jpeg quality must be between 1 and 100")
	}
	if options.ImageFormat == ImageFormatJPEG && options.TransparentBackground {
		return fmt.Errorf("JPEG does not support a transparent background")
	}
	if options.LibreOfficeTimeout <= 0 {
		return fmt.Errorf("libreoffice timeout must be greater than zero")
	}
	if options.FilenamePrefix != "" && filepath.Base(options.FilenamePrefix) != options.FilenamePrefix {
		return fmt.Errorf("filename prefix must be a file name component")
	}
	return nil
}

// RenderedImage describes one rendered document page.
type RenderedImage struct {
	PageNumber int
	Path       string
	Width      int
	Height     int
}

// RenderResult contains all images generated from a document.
type RenderResult struct {
	Source string
	Images []RenderedImage
}

// PageCount returns the number of rendered pages.
func (result RenderResult) PageCount() int {
	return len(result.Images)
}
