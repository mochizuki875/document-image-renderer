# document-image-renderer

A Go library and command-line tool that renders PDF and Microsoft Office documents as PNG or JPEG images.

PDF rendering uses PDFium compiled to WebAssembly through `go-pdfium`. It does not require CGO, a PDF renderer executable, or an operating-system PDF package. Office documents are converted to temporary PDFs with LibreOffice.

## Supported formats

| Input | Output unit | Conversion path |
|---|---|---|
| PDF | Page | PDFium directly |
| DOC, DOCX | Page | LibreOffice Writer to PDF |
| PPT, PPTX | Slide | LibreOffice Impress to PDF |
| XLS, XLSX, XLSM | Worksheet | LibreOffice Calc to PDF |

XLS uses LibreOffice's single-page-sheet export filter. XLSX and XLSM worksheets are adjusted on temporary copies to fit one landscape page per sheet. The source document is never modified.

## Requirements

- Go 1.27 or later
- LibreOffice for Office input only
- Fonts used by the source documents for stable Office layout

## Library usage

```go
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
		"test/documents/samplefile.docx",
		"example/output",
		&options,
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("rendered %d pages\n", result.PageCount())
}
```

Example:

```bash
go run example/example.go
```

Pass `nil` options to use the defaults. Use `DefaultRenderOptions` before overriding individual fields.

## Options

| Field | Default | Description |
|---|---:|---|
| `DPI` | `200` | Resolution from 1 to 1200 DPI |
| `ImageFormat` | `png` | `png` or `jpeg` |
| `JPEGQuality` | `90` | JPEG quality from 1 to 100 |
| `TransparentBackground` | `false` | Preserve a transparent PDF page background in PNG |
| `FilenamePrefix` | source stem | Output filename prefix |
| `LibreOfficeTimeout` | `120s` | Office conversion timeout |
| `LibreOfficeExecutable` | auto-detected | Explicit LibreOffice executable path |

Output names use `<prefix>-page-0001.png` or `.jpg`. Existing files with the same names are replaced; unrelated output files remain untouched.

Rendering is page-oriented rather than transactional. If a later page fails, images already written for earlier pages remain in the output directory. The returned `RenderResult.Source` is the absolute input path.

## Errors and cancellation

`RenderDocument` accepts a `context.Context`. Cancellation stops an active LibreOffice conversion and is observed between PDF pages. Callers can use `errors.As` with these public error types:

- `UnsupportedFormatError`
- `DependencyNotFoundError`
- `DocumentConversionError`
- `DocumentRenderError`

`DocumentConversionError` retains LibreOffice standard output and standard error for diagnostics. Input-validation and filesystem errors may be returned directly.

## CLI

```bash
go run ./cmd/document-image-renderer [options] SOURCE OUTPUT_DIRECTORY
```

Example:

```bash
go run ./cmd/document-image-renderer \
  --dpi 200 \
  --format png \
  test/documents/samplefile.docx \
  example/output
```

Use `--help` for all options. Generated image paths are printed to standard output in page order.

## Development

```bash
make verify
make test-integration
make build
```

Integration tests discover every file directly under `test/documents`. They require `RUN_INTEGRATION_TESTS=1`; Office cases are skipped when LibreOffice is unavailable.

The dev container supplies Go 1.27, LibreOffice Writer/Calc/Impress, and fonts used by common Office documents.

See [DESIGN.md](DESIGN.md) for architecture, conversion behavior, and operational limits.
