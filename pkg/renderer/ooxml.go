package renderer

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// prepareOfficeSource rewrites an OOXML document before conversion so that
// LibreOffice produces predictable output. PPTX shapes with negative extents
// are normalized and worksheets are forced to fit a single page. On any error
// the original source is returned so conversion can still proceed.
func prepareOfficeSource(source, extension, workingDirectory string) string {
	var (
		prepared string
		err      error
	)
	switch extension {
	case ".pptx":
		prepared, err = rewriteOOXML(source, workingDirectory, true, normalizePPTXPart)
	case ".xlsx", ".xlsm":
		prepared, err = rewriteOOXML(source, workingDirectory, false, fitWorksheetPart)
	default:
		return source
	}
	if err != nil {
		return source
	}
	return prepared
}

// ooxmlTransform rewrites a single archive member. The bool reports whether
// the member content changed so callers can detect a no-op rewrite.
type ooxmlTransform func(name string, data []byte) ([]byte, bool, error)

// rewriteOOXML copies an OOXML archive into workingDirectory, applying
// transform to every member. When onlyWhenChanged is set and no member was
// modified, the copy is discarded and the original source path is returned.
func rewriteOOXML(
	source string,
	workingDirectory string,
	onlyWhenChanged bool,
	transform ooxmlTransform,
) (string, error) {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return "", err
	}
	defer archive.Close()

	preparedPath := filepath.Join(workingDirectory, filepath.Base(source))
	output, err := os.Create(preparedPath)
	if err != nil {
		return "", err
	}
	writer := zip.NewWriter(output)
	// fail cleans up the partially written archive before returning the error.
	fail := func(err error) (string, error) {
		_ = writer.Close()
		_ = output.Close()
		_ = os.Remove(preparedPath)
		return "", err
	}

	changed := false
	for _, member := range archive.File {
		input, err := member.Open()
		if err != nil {
			return fail(err)
		}
		data, err := io.ReadAll(input)
		closeErr := input.Close()
		if err != nil {
			return fail(err)
		}
		if closeErr != nil {
			return fail(closeErr)
		}
		data, memberChanged, err := transform(member.Name, data)
		if err != nil {
			return fail(err)
		}
		changed = changed || memberChanged

		// Preserve the original member metadata (name, timestamps, flags).
		header := member.FileHeader
		entry, err := writer.CreateHeader(&header)
		if err != nil {
			return fail(err)
		}
		if _, err := entry.Write(data); err != nil {
			return fail(err)
		}
	}
	if err := writer.Close(); err != nil {
		_ = output.Close()
		_ = os.Remove(preparedPath)
		return "", err
	}
	if err := output.Close(); err != nil {
		_ = os.Remove(preparedPath)
		return "", err
	}
	if onlyWhenChanged && !changed {
		_ = os.Remove(preparedPath)
		return source, nil
	}
	return preparedPath, nil
}

// normalizePPTXPart fixes line shapes that carry a negative width or height.
// LibreOffice renders such shapes incorrectly, so the offset is shifted by the
// negative extent and the extent is made positive (mirroring the geometry).
func normalizePPTXPart(name string, data []byte) ([]byte, bool, error) {
	if !strings.HasPrefix(name, "ppt/slides/slide") || !strings.HasSuffix(name, ".xml") {
		return data, false, nil
	}
	document := etree.NewDocument()
	if err := document.ReadFromBytes(data); err != nil {
		return nil, false, err
	}
	changed := false
	for _, shape := range document.FindElements("//p:sp") {
		geometry := shape.FindElement("./p:spPr/a:prstGeom")
		if geometry == nil || geometry.SelectAttrValue("prst", "") != "line" {
			continue
		}
		transform := shape.FindElement("./p:spPr/a:xfrm")
		if transform == nil {
			continue
		}
		offset := transform.FindElement("./a:off")
		extent := transform.FindElement("./a:ext")
		if offset == nil || extent == nil {
			continue
		}
		// Handle the x and y axes independently: a negative extent on one axis
		// means the shape extends in the opposite direction on that axis.
		for _, axis := range []struct{ offset, extent string }{{"x", "cx"}, {"y", "cy"}} {
			extentValue, err := strconv.ParseInt(extent.SelectAttrValue(axis.extent, "0"), 10, 64)
			if err != nil {
				return nil, false, err
			}
			if extentValue >= 0 {
				continue
			}
			offsetValue, err := strconv.ParseInt(offset.SelectAttrValue(axis.offset, "0"), 10, 64)
			if err != nil {
				return nil, false, err
			}
			offset.CreateAttr(axis.offset, strconv.FormatInt(offsetValue+extentValue, 10))
			extent.CreateAttr(axis.extent, strconv.FormatInt(-extentValue, 10))
			changed = true
		}
	}
	if !changed {
		return data, false, nil
	}
	updated, err := document.WriteToBytes()
	return updated, true, err
}

// fitWorksheetPart configures a worksheet to fit on a single landscape page.
// This keeps the rendered page count of a workbook predictable regardless of
// the print settings stored in the file.
func fitWorksheetPart(name string, data []byte) ([]byte, bool, error) {
	if !strings.HasPrefix(name, "xl/worksheets/sheet") || !strings.HasSuffix(name, ".xml") {
		return data, false, nil
	}
	document := etree.NewDocument()
	if err := document.ReadFromBytes(data); err != nil {
		return nil, false, err
	}
	root := document.Root()
	// sheetPr/pageSetUpPr must exist for fitToPage to take effect.
	sheetProperties := root.FindElement("./sheetPr")
	if sheetProperties == nil {
		sheetProperties = etree.NewElement("sheetPr")
		root.InsertChildAt(0, sheetProperties)
	}
	pageSetupProperties := sheetProperties.FindElement("./pageSetUpPr")
	if pageSetupProperties == nil {
		pageSetupProperties = sheetProperties.CreateElement("pageSetUpPr")
	}
	pageSetupProperties.CreateAttr("fitToPage", "1")

	// pageSetup must follow pageMargins to satisfy the OOXML schema order.
	pageSetup := root.FindElement("./pageSetup")
	if pageSetup == nil {
		pageSetup = etree.NewElement("pageSetup")
		pageMargins := root.FindElement("./pageMargins")
		if pageMargins == nil {
			root.AddChild(pageSetup)
		} else {
			root.InsertChildAt(pageMargins.Index()+1, pageSetup)
		}
	}
	// Remove any explicit scale so fitToWidth/fitToHeight take precedence.
	pageSetup.RemoveAttr("scale")
	pageSetup.CreateAttr("orientation", "landscape")
	pageSetup.CreateAttr("fitToWidth", "1")
	pageSetup.CreateAttr("fitToHeight", "1")
	updated, err := document.WriteToBytes()
	return updated, true, err
}
