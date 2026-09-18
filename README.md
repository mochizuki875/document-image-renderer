# document-image-renderer

A Go library and command-line tool that renders PDF and Microsoft Office documents as PNG or JPEG images. The library also extracts text in document order.

PDF rendering uses PDFium compiled to WebAssembly through `go-pdfium`. It does not require CGO, a PDF renderer executable, or an operating-system PDF package. Office documents are converted to temporary PDFs with LibreOffice.

## Supported formats

| Input | Render unit | Text extraction |
|---|---|---|
| PDF | Page | Page with PDFium |
| DOC | Page | Document body after temporary DOCX conversion |
| DOCX | Page | Document body |
| PPT | Slide | Slide after temporary PPTX conversion |
| PPTX | Slide | Slide |
| XLS | Worksheet | Worksheet cell values after temporary XLSX conversion |
| XLSX, XLSM | Worksheet | Worksheet cell values |

Legacy DOC, PPT, and XLS files are converted to temporary OOXML files before text extraction. XLS uses LibreOffice's single-page-sheet export filter for rendering. XLSX and XLSM worksheets are adjusted on temporary copies to fit one landscape page per sheet. The source document is never modified.

## Requirements

- Go 1.27 or later
- LibreOffice for Office rendering and legacy DOC, PPT, or XLS extraction
- Fonts used by the source documents for stable Office layout

## Library usage

```go
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
```

Example:

```bash
go run example/example.go
```

Pass `nil` render options to use the defaults. Use `DefaultRenderOptions` before overriding individual fields. `ExtractDocument` uses default dependency settings; use `ExtractDocumentWithOptions` and `DefaultExtractOptions` to set the LibreOffice timeout or executable used for legacy Office extraction.

## Render options

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

## Extract options

| Field | Default | Description |
|---|---:|---|
| `LibreOfficeTimeout` | `120s` | Legacy Office conversion timeout |
| `LibreOfficeExecutable` | auto-detected | Explicit LibreOffice executable path |

`ExtractResult.Parts` preserves document order. `ExtractResult.Text()` joins all parts with a blank line without writing a file.

## Errors and cancellation

`RenderDocument` and `ExtractDocument` accept a `context.Context`. Cancellation stops an active LibreOffice conversion and is observed between PDF pages. Callers can use `errors.As` with these public error types:

- `UnsupportedFormatError`
- `DependencyNotFoundError`
- `DocumentConversionError`
- `DocumentRenderError`
- `DocumentExtractionError`

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

Use `--help` for all options. Generated image paths are printed to standard output in page order, followed by the extracted text path. All extracted parts are joined with a blank line and written to `<prefix>.txt`, or to `<source-stem>.txt` when `--prefix` is omitted.

## Development

```bash
make verify
make test-integration
make build
```

Integration tests discover every file directly under `test/documents`. They require `RUN_INTEGRATION_TESTS=1`; Office cases are skipped when LibreOffice is unavailable.

The dev container supplies Go 1.27, LibreOffice Writer/Calc/Impress, and fonts used by common Office documents.

See [DESIGN.md](DESIGN.md) for architecture, conversion behavior, and operational limits.
