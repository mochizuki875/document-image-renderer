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

// RenderOptions controls rendering limits, page rasterization, and LibreOffice execution.
// The zero value is not usable; call DefaultRenderOptions and override fields.
type RenderOptions struct {
	DPI                   int
	RenderTimeout         time.Duration
	MaxPages              int
	MaxPDFBytes           uint64
	MaxPageWidth          int
	MaxPageHeight         int
	MaxPagePixels         uint64
	TargetPagePixels      uint64
	MaxDocumentPixels     uint64
	MaxOOXMLMembers       int
	MaxOOXMLMemberBytes   uint64
	MaxOOXMLTotalBytes    uint64
	ImageFormat           ImageFormat
	JPEGQuality           int
	TransparentBackground bool
	FilenamePrefix        string
	LibreOfficeTimeout    time.Duration
	LibreOfficeExecutable string
}

// DefaultRenderOptions returns the default rendering configuration.
// MaxPages is zero, meaning no page limit, and the background is opaque.
func DefaultRenderOptions() RenderOptions {
	return RenderOptions{
		DPI:                 300,
		RenderTimeout:       120 * time.Second,
		MaxPDFBytes:         128 << 20,
		MaxPageWidth:        20_000,
		MaxPageHeight:       20_000,
		MaxPagePixels:       200_000_000,
		MaxDocumentPixels:   1_000_000_000,
		MaxOOXMLMembers:     10_000,
		MaxOOXMLMemberBytes: 256 << 20,
		MaxOOXMLTotalBytes:  1 << 30,
		ImageFormat:         ImageFormatPNG,
		JPEGQuality:         100,
		LibreOfficeTimeout:  120 * time.Second,
	}
}

// Validate checks whether all option values can be rendered safely.
func (options RenderOptions) Validate() error {
	if options.DPI < 1 || options.DPI > 1200 {
		return fmt.Errorf("dpi must be between 1 and 1200")
	}
	if options.RenderTimeout < 0 {
		return fmt.Errorf("render timeout must not be negative")
	}
	if options.MaxPages < 0 {
		return fmt.Errorf("max pages must not be negative")
	}
	if options.MaxPageWidth < 0 || options.MaxPageHeight < 0 {
		return fmt.Errorf("maximum page dimensions must not be negative")
	}
	if options.MaxOOXMLMembers < 0 {
		return fmt.Errorf("max OOXML members must not be negative")
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
	if options.LibreOfficeTimeout < 0 {
		return fmt.Errorf("libreoffice timeout must not be negative")
	}
	if options.FilenamePrefix != "" && filepath.Base(options.FilenamePrefix) != options.FilenamePrefix {
		return fmt.Errorf("filename prefix must be a file name component")
	}
	return nil
}
