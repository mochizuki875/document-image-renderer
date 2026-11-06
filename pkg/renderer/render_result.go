package renderer

// RenderedImage describes one rendered document page.
// PageNumber is one-based and follows the document page order.
type RenderedImage struct {
	PageNumber      int
	Path            string
	RequestedWidth  int
	RequestedHeight int
	Width           int
	Height          int
	Scaled          bool
}

// RenderResult contains all images generated from a document.
type RenderResult struct {
	Source string
	Images []RenderedImage
}

// PageCount returns the number of rendered pages.
func (result RenderResult) PageCount() int {
	return len(result.Images)
}
