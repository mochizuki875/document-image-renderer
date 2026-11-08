# document-image-renderer Design
`document-image-renderer` is a Go library and command-line tool that renders PDF and Microsoft Office documents as page-oriented PNG or JPEG images, and extracts their text in source order. The public API is `pkg/renderer`; the CLI is isolated in `internal/cli`. Rendering uses PDFium compiled to WebAssembly, while Office documents are converted with LibreOffice.

## Scope

| Input family | Extensions | Render unit | Text extraction unit |
|---|---|---|---|
| PDF | `.pdf` | Page | Page |
| Word | `.doc`, `.docx` | Page | Document body |
| PowerPoint | `.ppt`, `.pptx` | Slide | Slide |
| Excel | `.xls`, `.xlsx`, `.xlsm` | Printed worksheet page | Worksheet |

The output formats are lossless PNG and quality-configurable JPEG. XLS, XLSX, and XLSM use LibreOffice's `SinglePageSheets` PDF export option so that each worksheet produces one PDF page and therefore one image. For XLSX and XLSM, matching page settings are applied to a temporary copy. The intermediate PDF fixes the actual output order and unit; other print settings, such as hidden worksheets, print areas, and margins, follow LibreOffice's behavior.

The following features are out of scope:

- Password entry and decryption for encrypted documents
- Repair of corrupted documents
- Execution of Office macros
- Document editing or structural analysis beyond ordered text extraction
- Pixel-for-pixel equivalence with Microsoft Office in arbitrary environments

## Reproducibility

Reproducible output requires fixed PDFium and LibreOffice versions, fonts, locale, and rendering options. LibreOffice, not Microsoft Office, determines Office layout; differences in rendering engines, font substitution, and LibreOffice versions can change pagination and pixels.

## Architecture

```mermaid
flowchart TD
  caller[Go API / CLI] --> validate[Validate input and options]
  validate --> render[Render document]
  validate --> extract[Extract text]
  render --> renderRoute{PDF or Office?}
  renderRoute -->|PDF| pagePlan[Build page render plan]
  renderRoute -->|Office| officePDF[Prepare when needed and convert with LibreOffice]
  officePDF --> pagePlan
  pagePlan --> pdfium[PDFium]
  pdfium --> image[Sequential PNG or JPEG images]
  extract --> extractRoute{Format}
  extractRoute -->|PDF| pdfText[PDFium page text]
  extractRoute -->|DOCX / PPTX| xmlText[OOXML text nodes]
  extractRoute -->|XLSX / XLSM| workbookText[Worksheet cell values]
  extractRoute -->|DOC / PPT / XLS| legacy[Convert to OOXML, then extract]
  legacy --> xmlText
  legacy --> workbookText
  pdfText --> textParts[Sequential text parts]
  xmlText --> textParts
  workbookText --> textParts
```

For rendering, Office input is converted to a temporary PDF and PDF input is used directly. PDFium then processes pages sequentially and writes one image per page. Text extraction uses the source format where possible; legacy binary Office files are first converted to OOXML. PDF is the rendering intermediate representation, so the library does not reimplement Office layout.

## Packages

| Package | Responsibility |
| --- | --- |
| `cmd/document-image-renderer` | Process startup, signal handling, and exit status |
| `internal/cli` | Flag parsing, standard I/O, and CLI exit codes |
| `pkg/renderer` | Public API, validation, Office conversion, PDF rendering, and text extraction |
| `test/integration` | End-to-end tests using document fixtures and installed dependencies |

## Public API

The public Go API is provided by `pkg/renderer`. It renders supported documents, extracts their text, and counts rendered pages. All entry points accept paths rather than streams; input paths and returned artifact paths are absolute.

The primary rendering API is used as follows:

```go
options := renderer.DefaultRenderOptions()
options.DPI = 300
options.TargetPagePixels = 80_000_000
options.ImageFormat = renderer.ImageFormatPNG

result, err := renderer.RenderDocument(
	context.Background(),
	"samplefile.docx",
	"output",
	&options,
)
if err != nil {
	return err
}
for _, image := range result.Images {
	fmt.Println(image.PageNumber, image.Path, image.Width, image.Height)
}
```

Use `DefaultRenderOptions` or `DefaultExtractOptions` before overriding individual fields. Passing `nil` selects the corresponding defaults. A non-`nil` options value is validated as supplied and is not merged with defaults.

## API Reference

### Supported formats

`SupportedExtensions() []string` returns the accepted lowercase extensions in lexical order: `.doc`, `.docx`, `.pdf`, `.ppt`, `.pptx`, `.xls`, `.xlsm`, and `.xlsx`. All document entry points match extensions case-insensitively.

### Render document

```go
func RenderDocument(ctx context.Context, source, outputDirectory string, options *RenderOptions) (*RenderResult, error)
```

`RenderDocument` creates `outputDirectory` when needed and renders each page in order. Office input is converted to a temporary PDF first. It returns no result on failure, but images written before a later failure remain in the output directory.

### Page count
```go
func PageCount(ctx context.Context, source string) (int, error)
func PageCountWithOptions(ctx context.Context, source string, options *ExtractOptions) (int, error)
```

`PageCount` returns the number of pages that `RenderDocument` would render without writing images. `PageCountWithOptions` controls the LibreOffice executable and timeout; `PageCount` uses `DefaultExtractOptions`.

### Text extraction

```go
func ExtractDocument(ctx context.Context, source string) (*ExtractResult, error)
func ExtractDocumentWithOptions(ctx context.Context, source string, options *ExtractOptions) (*ExtractResult, error)
```

These functions extract ordered text without writing files. PDF parts correspond to pages, presentations to slides, and workbooks to worksheets. DOC and DOCX return one document-body part. Legacy DOC, PPT, and XLS files are converted to temporary OOXML before extraction. `ExtractDocument` uses `DefaultExtractOptions`.

### Options

```go
func DefaultRenderOptions() RenderOptions
func (RenderOptions) Validate() error
func DefaultExtractOptions() ExtractOptions
func (ExtractOptions) Validate() error
```

| `RenderOptions` field | Default | Constraint and meaning |
|---|---:|---|
| `DPI` | `300` | Rendering resolution from 1 through 1200 DPI |
| `RenderTimeout` | `120s` | Timeout for the complete render operation; `0` permits no timeout |
| `LibreOfficeTimeout` | `120s` | Timeout for one Office conversion; `0` permits no timeout |
| `MaxPages` | `0` | Maximum pages to render; `0` permits unlimited pages |
| `MaxPDFBytes` | `134217728` | Maximum PDF input bytes; `0` permits unlimited bytes |
| `MaxPageWidth` | `20000` | Hard limit on planned and rendered page width after target scaling; `0` permits unlimited width |
| `MaxPageHeight` | `20000` | Hard limit on planned and rendered page height after target scaling; `0` permits unlimited height |
| `MaxPagePixels` | `200000000` | Hard limit on planned and rendered pixels per page after target scaling; `0` permits unlimited pixels |
| `TargetPagePixels` | `0` | Target pixels per page; larger pages are scaled down before bitmap creation, smaller pages are not enlarged, and `0` disables scaling |
| `MaxDocumentPixels` | `1000000000` | Hard limit on the accumulated rendered pixels across the document; `0` permits unlimited pixels |
| `MaxOOXMLMembers` | `10000` | Maximum OOXML ZIP members; `0` permits unlimited members |
| `MaxOOXMLMemberBytes` | `268435456` | Maximum uncompressed bytes per member; `0` permits unlimited bytes |
| `MaxOOXMLTotalBytes` | `1073741824` | Maximum total uncompressed bytes; `0` permits unlimited bytes |
| `ImageFormat` | `ImageFormatPNG` | PNG or JPEG output encoding |
| `JPEGQuality` | `100` | Value from 1 through 100; validated even for PNG output |
| `TransparentBackground` | `false` | Preserve PDF page alpha for PNG output |
| `FilenamePrefix` | Source basename without extension | Empty selects the source stem; a non-empty value must be one filename component |
| `LibreOfficeExecutable` | Auto-detected | Explicit LibreOffice executable when provided |

`ImageFormat` is a string type with the only valid values `ImageFormatPNG` (`"png"`) and `ImageFormatJPEG` (`"jpeg"`).

JPEG has no alpha channel, so combining `ImageFormatJPEG` with `TransparentBackground=true` is invalid.

`TargetPagePixels` is a scaling target, while `MaxPageWidth`, `MaxPageHeight`, and `MaxPagePixels` are rejection limits. The target is applied first and the hard limits are applied to the planned output. Pre-scaling dimensions derived from `DPI` may be larger than the corresponding hard-limit values; rendering is allowed only when the planned output after target scaling satisfies every enabled hard limit. `TargetPagePixels` and `MaxPagePixels` are independent; options validation does not require the target to be less than the hard limit, and rendering fails if the planned output still exceeds an enabled hard limit.

| `ExtractOptions` field | Default | Constraint and meaning |
|---|---:|---|
| `MaxCharacters` | `0` | Maximum Unicode characters across all extracted parts; `0` permits unlimited text |
| `MaxPDFBytes` | `134217728` | Maximum PDF bytes for page counting or extraction; `0` permits unlimited bytes |
| `MaxOOXMLMembers` | `10000` | Maximum OOXML ZIP members; `0` permits unlimited members |
| `MaxOOXMLMemberBytes` | `268435456` | Maximum uncompressed bytes per member; `0` permits unlimited bytes |
| `MaxOOXMLTotalBytes` | `1073741824` | Maximum total uncompressed bytes; `0` permits unlimited bytes |
| `LibreOfficeTimeout` | `120s` | Timeout for a legacy Office conversion; `0` permits no timeout |
| `LibreOfficeExecutable` | Auto-detected | Explicit LibreOffice executable when provided |

Modern OOXML extraction does not invoke LibreOffice, though options are validated before routing.

### Results

```go
type RenderedImage struct {
  PageNumber      int
  Path            string
  RequestedWidth  int
  RequestedHeight int
  Width           int
  Height          int
  Scaled          bool
}

type RenderResult struct {
  Source string
  Images []RenderedImage
}

func (RenderResult) PageCount() int

type TextPart struct {
  PartNumber int
  Text       string
}

type ExtractResult struct {
  Source string
  Parts  []TextPart
}

func (ExtractResult) PartCount() int
func (ExtractResult) Part(number int) (TextPart, bool)
func (ExtractResult) Text() string
```

Image and text parts are ordered and numbered from one. `RequestedWidth` and `RequestedHeight` are the dimensions requested by `DPI` before target scaling; `Width` and `Height` are the saved image dimensions. `Scaled` is fixed from the page render plan and reports whether `TargetPagePixels` reduced either planned dimension. PDFium returning a smaller image within an unscaled plan does not set `Scaled`. `RenderResult.PageCount` returns `len(Images)`; `ExtractResult.Part` looks up a part by number; `Text` joins parts with one blank line.


### Cancellation, Concurrency, and Resource Management

`RenderTimeout` limits the complete render operation; an earlier caller deadline wins. On Unix systems, each LibreOffice conversion owns a process group. Context cancellation sends `SIGKILL` to that group. If a descendant keeps an output pipe open after the direct process exits, `exec.Cmd.WaitDelay` closes the pipe after two seconds, and the resulting `exec.ErrWaitDelay` causes the same process group to be terminated. Process groups isolate concurrent conversions; descendants that explicitly leave their group are outside this guarantee. Calls also own independent LibreOffice profiles and PDFium pools, and may run concurrently unless they write the same output name. PDFs and live page bitmaps are held in memory; services processing untrusted input should also enforce process-level CPU, memory, input-size, storage, and concurrency limits.

DOCX and PPTX extraction preserves paragraph boundaries, explicit breaks, and tabs. PPTX slide order follows presentation relationships; hidden slides are included and orphan slide parts are ignored.

### Errors

The error types correspond to processing boundaries and can be inspected with `errors.As` and `errors.Is`.

| Error type | Condition |
|---|---|
| `UnsupportedFormatError` | Unsupported input extension |
| `DependencyNotFoundError` | LibreOffice cannot be resolved for an operation that requires it |
| `PageLimitExceededError` | The document page count exceeds `RenderOptions.MaxPages` |
| `PageSizeLimitExceededError` | A rendered page exceeds a dimension or per-page pixel limit |
| `DocumentPixelLimitExceededError` | Rendered pages exceed the total pixel limit |
| `PDFSizeLimitExceededError` | PDF input exceeds the configured byte limit |
| `CharacterLimitExceededError` | Extracted text exceeds `ExtractOptions.MaxCharacters` |
| `OOXMLLimitExceededError` | OOXML member count or expanded byte limits are exceeded |
| `DocumentConversionError` | Office conversion or temporary conversion setup fails; includes `Path`, `Stdout`, `Stderr`, and an unwrapped cause |
| `DocumentPageCountError` | PDF page counting fails; includes `Path` and an unwrapped cause |
| `DocumentRenderError` | PDF rasterization or image writing fails; includes `Path` and an unwrapped cause |
| `DocumentExtractionError` | Text extraction or text-limit validation fails; includes `Path` and an unwrapped cause |

Invalid options, a `nil` context, invalid input paths, and output-directory creation failures are ordinary contextual errors. Formats are selected by lowercased filename extension; file content is not inspected.


## Office Conversion

### Workspace and invocation

`LibreOfficeExecutable` is used directly when set; otherwise the library searches `PATH` for `libreoffice`, then `soffice`. Each conversion owns a temporary workspace, isolated LibreOffice profile, and prepared OOXML copy when needed. LibreOffice runs without a shell with macro security set to Very High, and the source is never modified. On Unix systems, `Setpgid` creates a conversion-specific process group and `Cmd.Cancel` terminates that group. Conversion fails unless the expected regular output exists; standard output and standard error are retained for diagnostics.

### Format-specific preparation

| Format | Preparation | Rationale |
|---|---|---|
| DOC, DOCX | None | Pass directly to LibreOffice without introducing an unnecessary intermediate format |
| PPT | None | Avoid additional presentation layout changes |
| PPTX | Normalize negative width and height values for line shapes | Reduce line reversal caused by differences in how PowerPoint and LibreOffice interpret negative extents |
| XLS | Set `SinglePageSheets=true` in the Calc PDF export filter | Fit each sheet to one page without rewriting the binary format |
| XLSX, XLSM | Apply the same `SinglePageSheets` filter; also set `fitToPage=1`, `fitToWidth=1`, `fitToHeight=1`, and landscape orientation on every worksheet, and remove `scale` | Ensure each worksheet produces exactly one PDF page while keeping explicit page settings in the temporary OOXML copy |

PPTX normalization preserves line endpoints; when no shape changes, the original source is used. XLSX and XLSM settings are applied to `xl/worksheets/sheet*.xml`. OOXML member and expanded-byte limits are enforced both from ZIP metadata and while reading members, and rewriting always targets a temporary file.

## PDF Rendering

PDF rendering uses `go-pdfium` on wazero, requiring neither CGO nor an external renderer. Each call owns a single-instance pool and holds the PDF in memory, bounded by `MaxPDFBytes`. Pages are planned, rendered, and saved sequentially. Cancellation kills the active PDFium instance and waits for it to terminate.

### DPI and target relationship

`DPI` determines the requested base resolution from the physical PDF page size. `TargetPagePixels` does not replace `DPI`; it caps the pixel count produced from that requested resolution. When the requested pixel count is within the target, the output dimensions remain those derived from `DPI`. When it exceeds the target, both dimensions are reduced by the same scale before rendering. Pages below the target are never enlarged.

Requested width and height are approximately proportional to `DPI`, so the requested pixel count is approximately proportional to the square of `DPI`. Increasing `DPI` increases output dimensions until `TargetPagePixels` is reached; after that point, the target keeps the output pixel count capped. For a scaled page, the conceptual effective resolution is `DPI * scale`, but the renderer passes the calculated pixel dimensions directly to PDFium instead of rounding this value to an integer DPI.

### Page render planning

`GetPageSizeInPixels` calculates `RequestedWidth` and `RequestedHeight` from the physical PDF page and `DPI`. A pure calculation then produces one immutable plan for the page:

```go
type pageRenderPlan struct {
  RequestedWidth  int
  RequestedHeight int
  OutputWidth     int
  OutputHeight    int
  OutputPixels    uint64
  Scaled          bool
}
```

When `TargetPagePixels` is `0`, or the requested pixel count is already within the target, the output dimensions equal the requested dimensions. The renderer never enlarges a page to approach the target. Otherwise it applies the same conceptual scale to both dimensions:

```text
pixelScale  = sqrt(TargetPagePixels / RequestedWidth / RequestedHeight)
scale       = min(1, pixelScale)
OutputWidth = max(1, floor(RequestedWidth * scale))
OutputHeight = max(1, floor(RequestedHeight * scale))
```

The implementation divides before multiplying so an overflowing requested pixel product does not prevent deriving a safe target size. It also checks the integer result with `OutputWidth > TargetPagePixels / OutputHeight` rather than multiplying first. If floating-point error leaves the integer dimensions above the target, `OutputWidth` is clamped to the maximum width allowed by `OutputHeight`. Every enabled target can produce at least `1 x 1`; the calculation performs a fixed number of operations and does not search iteratively.

### Limit ordering and document accounting

The renderer applies dimensions and limits in this order:

1. Calculate requested dimensions from `DPI`.
2. Apply `TargetPagePixels` to obtain planned output dimensions.
3. Apply `MaxPageWidth`, `MaxPageHeight`, and `MaxPagePixels` to the planned output.
4. Before rendering, compare saved-page pixels plus the next page's planned pixels with `MaxDocumentPixels`.
5. After a successful save, add the rendered image's actual pixels to the saved-page total.

All pixel multiplication and accumulation checks are overflow-safe. The pre-render document check is conservative because rendered dimensions must not exceed the plan. Failed rendering or saving does not add that page to the accumulated total. These limits describe allocated and saved output dimensions; a separate option would be required if requested dimensions before target scaling also needed a hard limit.

### PDFium rendering paths

- An opaque page with `Scaled=false` uses `RenderPageInDPI` to preserve existing output behavior.
- An opaque page with `Scaled=true` uses `RenderPageInPixels`, passing `OutputWidth` and `OutputHeight` as maximum dimensions.
- A transparent page uses `FPDFBitmap_Create` and `FPDF_RenderPageBitmap` with `OutputWidth` and `OutputHeight`, so it never allocates the requested-size bitmap before scaling.

Both opaque paths preserve form rendering. Opaque output is composited onto white; transparent output remains PNG-only and uses an alpha bitmap.

### Rendered-size validation

Before encoding, image bounds are the source of truth for saved `Width` and `Height`. The renderer requires both dimensions to be positive and no greater than the planned dimensions, then reapplies the enabled page hard limits and `TargetPagePixels` to the actual pixel count. A result outside the plan is an internal rendering mismatch rather than an input limit violation; it is not resized or retried and is returned through `DocumentRenderError`. The image is published only after this validation and a complete encode.

### Image publication

Images are written to a temporary destination file and renamed after a complete encode, so failed or canceled encodes do not publish a partial file.

## Output Contract

The output directory is created with mode `0755` when necessary. Output names are:

```text
<prefix>-page-<page-number>.png
<prefix>-page-<page-number>.jpg
```

Page numbers are padded to at least four digits and expand for 10,000 or more pages. Existing generated names are replaced, but unrelated and stale files are retained. Output is not transactional: earlier images remain after a later failure. Excel output has one image per exported worksheet in workbook order.

## Security

- **No shell invocation**: LibreOffice runs via `exec.CommandContext` with an argument array, so paths and arguments cannot inject shell commands.
- **Isolated LibreOffice profile**: each conversion uses a fresh temporary profile (`-env:UserInstallation`) with `--headless` and related flags; macro security is Very High (`MacroSecurityLevel=3`), so XLSM macros are not executed.
- **Source documents are never modified**: OOXML preparation runs on temporary copies in the per-conversion workspace; the original file is only read.
- **Temporary workspace permissions**: workspace and output directories are `0700`, the profile file is `0600`, and the workspace is removed after each conversion.
- **Resource limits with overflow-safe arithmetic**: `MaxPDFBytes`, `MaxPages`, `TargetPagePixels`, page dimension/pixel hard limits, `MaxDocumentPixels`, `MaxOOXML*`, and `MaxCharacters` bound memory, rendering, and text. Target scaling occurs before bitmap allocation and hard-limit evaluation. OOXML limits are enforced from ZIP metadata and while streaming members, mitigating zip-bomb-style archives.
- **Cancellation and timeouts**: on Unix systems, context cancellation terminates the conversion-specific LibreOffice process group, and a two-second `WaitDelay` bounds inherited output-pipe waiting; cancellation also interrupts PDFium. `RenderTimeout` and `LibreOfficeTimeout` bound operation duration.
- **Atomic image publication**: images are encoded to a temporary file and renamed only after a complete encode, so failed or canceled encodes never publish a partial image.
- **Filename prefix validation**: `FilenamePrefix` must be a single path component, preventing path traversal through generated output names.
- **The library is not a sandbox**: PDFium (WASM) and LibreOffice run on the host. Malicious documents can exploit vulnerabilities in PDFium, wazero, LibreOffice, or the OOXML libraries; keep dependencies current and run in an isolated host environment.
- **Enforce process-level limits**: services processing untrusted input should also enforce CPU, memory, input-size, storage, and concurrency limits; the library's limits are defense in depth, not a complete boundary.
- **Formats are selected by extension only**: file content is not inspected, so a mislabeled file is processed according to its extension.
- **Output is not transactional**: images written before a later failure remain in the output directory; callers needing all-or-nothing output must stage and publish themselves.
- **Existing output files are replaced**: generated names are overwritten, but unrelated and stale files are retained; use a dedicated output directory.
- **Concurrent writes to the same output name are not coordinated**: callers must avoid concurrent writes to the same filename.
- **Reproducibility depends on the environment**: LibreOffice, not Microsoft Office, determines Office layout; fixed PDFium/LibreOffice versions, fonts, and locale are required for reproducible output.

## CLI Design

`cmd/document-image-renderer` creates a context that handles operating-system signals and delegates to `internal/cli.Run`. The CLI exposes the library options as flags and accepts `SOURCE OUTPUT_DIRECTORY` as positional arguments. `--target-page-pixels` scales larger pages before rendering and defaults to `0`, which disables scaling. `--render-timeout` limits the complete render operation, while `--timeout` retains its existing meaning as the limit for each LibreOffice conversion; both default to 120 seconds.

On success, each generated image path is followed by the path of its UTF-8 text file. A text file uses the same stem as its image, replacing the image extension with `.txt`. The CLI matches `RenderedImage.PageNumber` to `TextPart.PartNumber`; when no corresponding part exists, it writes an empty text file. This occurs for additional DOC or DOCX pages because those formats expose one document-body text part rather than rendered page boundaries. Paths are emitted only after rendering, extraction, and all text writes succeed. Diagnostics and usage information are written to standard error. Exit codes have the following meanings:

| Exit code | Meaning |
|---:|---|
| `0` | Conversion succeeded, or help was displayed |
| `1` | Rendering, extraction, or text output failed |
| `2` | Flag parsing failed or positional arguments were invalid |

The CLI contains no document conversion or extraction logic. It delegates to `renderer.RenderDocument` and `renderer.ExtractDocumentWithOptions`, then writes one text artifact for each rendered image.
