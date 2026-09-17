// Package cli implements the document-image-renderer command.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/mochizuki875/document-image-renderer/pkg/renderer"
)

// Run executes the command and returns its process exit code.
func Run(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("document-image-renderer", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dpi := flags.Int("dpi", 200, "rendering resolution")
	imageFormat := flags.String("format", "png", "output format: png or jpeg")
	jpegQuality := flags.Int("jpeg-quality", 90, "JPEG quality from 1 to 100")
	prefix := flags.String("prefix", "", "output file name prefix")
	timeout := flags.Float64("timeout", 120, "LibreOffice timeout in seconds")
	transparent := flags.Bool("transparent", false, "use a transparent PNG background")
	libreOffice := flags.String("libreoffice", "", "path to the LibreOffice executable")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: document-image-renderer [options] SOURCE OUTPUT_DIRECTORY")
		flags.PrintDefaults()
	}

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
		ImageFormat:           renderer.ImageFormat(*imageFormat),
		JPEGQuality:           *jpegQuality,
		TransparentBackground: *transparent,
		FilenamePrefix:        *prefix,
		LibreOfficeTimeout:    time.Duration(*timeout * float64(time.Second)),
		LibreOfficeExecutable: *libreOffice,
	}
	result, err := renderer.RenderDocument(ctx, flags.Arg(0), flags.Arg(1), &options)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	for _, image := range result.Images {
		fmt.Fprintln(stdout, image.Path)
	}
	return 0
}
