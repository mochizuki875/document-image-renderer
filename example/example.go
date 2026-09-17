package main

import (
	"context"
	"fmt"
	"log"

	"github.com/mochizuki875/document-image-renderer/pkg/renderer"
)

func main() {
	options := renderer.DefaultRenderOptions()
	options.DPI = 200
	options.ImageFormat = renderer.ImageFormatPNG

	result, err := renderer.RenderDocument(
		context.Background(),
		"test/documents/samplefile.pptx",
		"example/output",
		&options,
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("rendered %d pages\n", result.PageCount())
}
