package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/mochizuki875/document-image-renderer/pkg/renderer"
)

func main() {
	ctx := context.Background()
	source := "test/documents/samplefile.docx"
	outputDirectory := "example/output"
	options := renderer.DefaultRenderOptions()
	options.DPI = 200
	options.ImageFormat = renderer.ImageFormatPNG
	options.MaxPages = 3

	// Render the first three pages of the sample document to images.
	result, err := renderer.RenderDocument(
		ctx,
		source,
		outputDirectory,
		&options,
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("rendered %d pages\n", result.PageCount())

	// Extract the matching page text and save it next to each image.
	extracted, err := renderer.ExtractDocument(ctx, source)
	if err != nil {
		log.Fatal(err)
	}
	for _, image := range result.Images {
		text := ""
		if part, found := extracted.Part(image.PageNumber); found {
			text = part.Text
		}
		textPath := strings.TrimSuffix(image.Path, filepath.Ext(image.Path)) + ".txt"
		if err := os.WriteFile(textPath, []byte(text), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println(textPath)
	}
}
