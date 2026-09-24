package renderer

import "testing"

func TestDefaultRenderOptionsAreValid(t *testing.T) {
	options := DefaultRenderOptions()
	if err := options.Validate(); err != nil {
		t.Fatalf("default options must be valid: %v", err)
	}
	if options.DPI != 300 || options.ImageFormat != ImageFormatPNG || options.JPEGQuality != 90 {
		t.Fatalf("unexpected defaults: %+v", options)
	}
}

func TestDefaultExtractOptionsAreValid(t *testing.T) {
	options := DefaultExtractOptions()
	if err := options.Validate(); err != nil {
		t.Fatalf("default options must be valid: %v", err)
	}
	if options.LibreOfficeTimeout <= 0 {
		t.Fatalf("unexpected defaults: %+v", options)
	}
}

func TestRenderOptionsRejectInvalidValues(t *testing.T) {
	tests := map[string]func(*RenderOptions){
		"dpi":          func(options *RenderOptions) { options.DPI = 0 },
		"image format": func(options *RenderOptions) { options.ImageFormat = "gif" },
		"jpeg quality": func(options *RenderOptions) { options.JPEGQuality = 101 },
		"jpeg transparency": func(options *RenderOptions) {
			options.ImageFormat, options.TransparentBackground = ImageFormatJPEG, true
		},
		"timeout":            func(options *RenderOptions) { options.LibreOfficeTimeout = 0 },
		"filename traversal": func(options *RenderOptions) { options.FilenamePrefix = "../outside" },
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

func TestExtractResultTextJoinsParts(t *testing.T) {
	result := ExtractResult{Parts: []TextPart{{Text: "first"}, {Text: "second"}}}
	if text := result.Text(); text != "first\n\nsecond" {
		t.Fatalf("Text() = %q", text)
	}
}

func TestExtractResultPartFindsNumberedPart(t *testing.T) {
	result := ExtractResult{Parts: []TextPart{{PartNumber: 2, Text: "second"}}}
	part, found := result.Part(2)
	if !found || part.Text != "second" {
		t.Fatalf("Part(2) = %#v, %t", part, found)
	}
	if _, found := result.Part(1); found {
		t.Fatal("Part(1) unexpectedly found")
	}
}
