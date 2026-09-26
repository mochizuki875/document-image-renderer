// Package cli implements the document-image-renderer command.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mochizuki875/document-image-renderer/pkg/renderer"
)

// Run executes the command and returns its process exit code.
// Exit codes: 0 success, 1 runtime error, 2 usage error.
func Run(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	defaults := renderer.DefaultRenderOptions()
	flags := flag.NewFlagSet("document-image-renderer", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dpi := flags.Int("dpi", defaults.DPI, "rendering resolution")
	maxPages := flags.Int("max-pages", defaults.MaxPages, "maximum pages to render (0 for unlimited)")
	maxCharacters := flags.Int("max-characters", 0, "maximum characters to extract (0 for unlimited)")
	imageFormat := flags.String("format", string(defaults.ImageFormat), "output format: png or jpeg")
	jpegQuality := flags.Int("jpeg-quality", defaults.JPEGQuality, "JPEG quality from 1 to 100")
	prefix := flags.String("prefix", "", "output file name prefix")
	timeout := flags.Float64("timeout", defaults.LibreOfficeTimeout.Seconds(), "LibreOffice timeout in seconds")
	transparent := flags.Bool("transparent", false, "use a transparent PNG background")
	libreOffice := flags.String("libreoffice", "", "path to the LibreOffice executable")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: document-image-renderer [options] SOURCE OUTPUT_DIRECTORY")
		flags.PrintDefaults()
	}

	// Help is not an error; invalid flags and wrong argument counts are usage errors.
	if err := flags.Parse(arguments); errors.Is(err, flag.ErrHelp) {
		return 0
	} else if err != nil {
		return 2
	}
	if flags.NArg() != 2 {
		flags.Usage()
		return 2
	}

	options := renderer.RenderOptions{
		DPI:                   *dpi,
		MaxPages:              *maxPages,
		ImageFormat:           renderer.ImageFormat(*imageFormat),
		JPEGQuality:           *jpegQuality,
		TransparentBackground: *transparent,
		FilenamePrefix:        *prefix,
		LibreOfficeTimeout:    time.Duration(*timeout * float64(time.Second)),
		LibreOfficeExecutable: *libreOffice,
	}
	extractOptions := renderer.ExtractOptions{
		MaxCharacters:         *maxCharacters,
		LibreOfficeTimeout:    options.LibreOfficeTimeout,
		LibreOfficeExecutable: options.LibreOfficeExecutable,
	}
	if err := extractOptions.Validate(); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	outputDirectory, err := filepath.Abs(flags.Arg(1))
	if err != nil {
		fmt.Fprintf(stderr, "error: resolve output directory: %v\n", err)
		return 1
	}
	result, err := renderer.RenderDocument(ctx, flags.Arg(0), outputDirectory, &options)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	extracted, err := renderer.ExtractDocumentWithOptions(ctx, flags.Arg(0), &extractOptions)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	// Write one .txt file next to each rendered image, matching page numbers.
	textPaths := make([]string, 0, len(result.Images))
	for _, image := range result.Images {
		text := ""
		if part, found := extracted.Part(image.PageNumber); found {
			text = part.Text
		}
		textPath := strings.TrimSuffix(image.Path, filepath.Ext(image.Path)) + ".txt"
		if err := os.WriteFile(textPath, []byte(text), 0o644); err != nil {
			fmt.Fprintf(stderr, "error: write extracted text: %v\n", err)
			return 1
		}
		textPaths = append(textPaths, textPath)
	}
	// Print image and text paths in pairs, one page per line.
	for index, image := range result.Images {
		fmt.Fprintln(stdout, image.Path)
		fmt.Fprintln(stdout, textPaths[index])
	}
	return 0
}
