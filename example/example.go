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

	extracted, err := renderer.ExtractDocument(ctx, source)
	if err != nil {
		log.Fatal(err)
	}
	prefix := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	textPath := filepath.Join(outputDirectory, prefix+".txt")
	if err := os.WriteFile(textPath, []byte(extracted.Text()), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Println(textPath)
}
