package renderer

import "testing"

func TestDefaultRenderOptionsAreValid(t *testing.T) {
	options := DefaultRenderOptions()
	if err := options.Validate(); err != nil {
		t.Fatalf("default options must be valid: %v", err)
	}
	if options.DPI != 300 || options.ImageFormat != ImageFormatPNG || options.JPEGQuality != 100 {
		t.Fatalf("unexpected defaults: %+v", options)
	}
	if options.MaxPDFBytes == 0 {
		t.Fatal("default PDF byte limit must be finite")
	}
	if options.TargetPagePixels != 0 {
		t.Fatalf("target page pixels must default to disabled: %d", options.TargetPagePixels)
	}
}

func TestDefaultRenderTimeout(t *testing.T) {
	if timeout := DefaultRenderOptions().RenderTimeout; timeout.Seconds() != 120 {
		t.Fatalf("unexpected render timeout: %s", timeout)
	}
}

func TestRenderOptionsRejectInvalidValues(t *testing.T) {
	tests := map[string]func(*RenderOptions){
		"dpi": func(options *RenderOptions) { options.DPI = 0 },
		"render timeout": func(options *RenderOptions) {
			options.RenderTimeout = -1
		},
		"max pages":    func(options *RenderOptions) { options.MaxPages = -1 },
		"image format": func(options *RenderOptions) { options.ImageFormat = "gif" },
		"jpeg quality": func(options *RenderOptions) { options.JPEGQuality = 101 },
		"jpeg transparency": func(options *RenderOptions) {
			options.ImageFormat, options.TransparentBackground = ImageFormatJPEG, true
		},
		"timeout":            func(options *RenderOptions) { options.LibreOfficeTimeout = -1 },
		"filename traversal": func(options *RenderOptions) { options.FilenamePrefix = "../outside" },
		"max page width":     func(options *RenderOptions) { options.MaxPageWidth = -1 },
		"max page height":    func(options *RenderOptions) { options.MaxPageHeight = -1 },
		"max ooxml members":  func(options *RenderOptions) { options.MaxOOXMLMembers = -1 },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			options := DefaultRenderOptions()
			mutate(&options)
			if err := options.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestRenderOptionsAllowUnlimitedTimeouts(t *testing.T) {
	options := DefaultRenderOptions()
	options.RenderTimeout = 0
	options.LibreOfficeTimeout = 0
	if err := options.Validate(); err != nil {
		t.Fatalf("render options must allow unlimited timeouts: %v", err)
	}
}
