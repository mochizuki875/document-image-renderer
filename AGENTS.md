# AGENTS.md

## Project Overview

`document-image-renderer` is a Go library and command-line tool that renders PDF and Microsoft Office documents (DOC, DOCX, PPT, PPTX, XLS, XLSX, XLSM) as PNG or JPEG images, and extracts text in document order.

- PDF rendering uses PDFium compiled to WebAssembly via `go-pdfium` (running on wazero). No CGO, no OS PDF package, no external PDF renderer process.
- Office documents are converted to temporary PDFs with LibreOffice (system dependency, not a Go module dependency).
- The public API lives in `pkg/renderer`; CLI-specific logic is isolated in `internal/cli`.

## Commands

```bash
make build            # Build the CLI binary to bin/document-image-renderer
make fmt              # go fmt ./...
make vet              # go vet ./...
make test             # go test ./...
make test-integration # RUN_INTEGRATION_TESTS=1 go test -v ./test/integration
make verify           # fmt + vet + test (default target)
```

- Integration tests require `RUN_INTEGRATION_TESTS=1`; Office cases are skipped when LibreOffice is unavailable.
- Go 1.27+ is required.

## Architecture

```text
cmd/document-image-renderer/main.go   Process startup, signal handling, exit code
internal/cli/cli.go                   Flag parsing, standard I/O, exit codes (0/1/2)
pkg/renderer/
  doc.go                              Public package documentation
  models.go                           Options and result models
  errors.go                           Public error types
  renderer.go                         Shared validation and rendering pipeline control
  extract.go                          Extraction API and format routing
  office.go                           LibreOffice execution and temporary workspace
  ooxml.go                            PPTX/XLSX/XLSM preparation
  ooxml_text.go                       DOCX/PPTX text extraction
  workbook_text.go                    XLSX/XLSM cell extraction
  pdf.go                              Shared PDFium lifecycle, rendering, and extraction
test/documents/                       Document fixtures
test/integration/                     Integration tests using real tools
example/example.go                    Public API example
```

Pipeline: validate input/options → (Office only) convert to temporary PDF with LibreOffice → render every PDF page sequentially with PDFium → encode PNG/JPEG.

## Key Design Constraints

- **PDF is the intermediate representation** for all Office formats. Do not reimplement Office page layout in Go.
- **Source documents are never modified.** All OOXML preparation happens on temporary copies in a per-conversion workspace under the OS temp directory.
- **Each call owns an isolated LibreOffice profile and PDFium pool.** Calls can run concurrently with different output destinations; concurrent writes to the same filename are not coordinated.
- **Output is not transactional.** Images written before a later failure remain in the output directory. No `RenderResult` is returned on error.
- **Limits are enforced with overflow-safe arithmetic** before rendering/encoding (page dimensions, per-page pixels, document pixels, PDF bytes, OOXML member count/bytes, character count).
- **PDF input is held in memory** as a contiguous byte slice (go-pdfium requirement); `MaxPDFBytes` bounds the allocation.
- **LibreOffice is invoked without a shell** (argument array), with `--headless`, `--nologo`, `--nodefault`, `--nolockcheck`, `--nofirststartwizard`, and an isolated profile with macro security set to Very High.
- **XLSX/XLSM worksheets are adjusted on temporary copies** (`fitToPage=1`, `fitToWidth=1`, `fitToHeight=1`, landscape, remove `scale`) so each worksheet produces exactly one PDF page. XLS uses the `SinglePageSheets` export filter.
- **PPTX normalization** applies only to preset `line` shapes with negative extents (add extent to offset, take absolute value).
- **DOC/DOCX text extraction returns the document body as one part** (OOXML has no rendered page boundaries); extra rendered pages get empty text files.
- **PPTX slide order** follows `presentation.xml` relationships (`p:sldIdLst`), not slide filenames. Hidden slides are included; orphan slide parts are excluded.

## Conventions

- Public API entry points accept paths, not streams; input and returned artifact paths are absolute.
- Use `DefaultRenderOptions()` / `DefaultExtractOptions()` before overriding fields. Passing `nil` selects defaults; a non-nil options value is validated as supplied (not merged with defaults).
- Public error types are defined in `pkg/renderer/errors.go` and must be usable with `errors.As`/`errors.Is`. Wrapping errors (`DocumentConversionError`, `DocumentPageCountError`, `DocumentRenderError`, `DocumentExtractionError`) implement `Unwrap`.
- Invalid options, nil context, missing/non-regular input, path resolution failures, and output-directory creation failures are ordinary errors with operation context — not the public document-processing error types.
- All entry points take a `context.Context`; cancellation must stop LibreOffice and interrupt in-flight PDFium execution.
- Output filenames: `<prefix>-page-<NNNN>.png|.jpg`, zero-padded to at least four digits (width expands for 10,000+ pages to keep lexical order aligned with page order).
- CLI exit codes: `0` success/help, `1` runtime error, `2` usage error.
- Keep `README.md` and `DESIGN.md` in sync with behavior changes; they are the primary documentation.

## Testing

- Unit tests in `pkg/renderer` cover rendering order, options, error types, LibreOffice discovery/profile/filter behavior, OOXML rewriting, extraction order, and CLI exit codes.
- LibreOffice discovery/execution is replaceable at small function boundaries so unit tests avoid external processes.
- Integration tests dynamically collect every file directly under `test/documents` and verify: at least one image and text part, decodable images with positive dimensions matching metadata, and unchanged source SHA-256.
- Pixel-level regressions require an environment with fixed PDFium, LibreOffice, fonts, and locale; reference images should be updated only after visual review.

## Dependencies

- Direct: `github.com/klippa-app/go-pdfium` (PDF rendering/extraction), `github.com/beevik/etree` (XML editing), `github.com/xuri/excelize/v2` (workbook text extraction).
- Indirect: `github.com/tetratelabs/wazero` (PDFium WASM runtime).
- System: LibreOffice (Writer/Calc/Impress) and fonts used by source documents for stable Office layout.
