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

type ooxmlTransform func(name string, data []byte) ([]byte, bool, error)

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

func fitWorksheetPart(name string, data []byte) ([]byte, bool, error) {
	if !strings.HasPrefix(name, "xl/worksheets/sheet") || !strings.HasSuffix(name, ".xml") {
		return data, false, nil
	}
	document := etree.NewDocument()
	if err := document.ReadFromBytes(data); err != nil {
		return nil, false, err
	}
	root := document.Root()
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
	pageSetup.RemoveAttr("scale")
	pageSetup.CreateAttr("orientation", "landscape")
	pageSetup.CreateAttr("fitToWidth", "1")
	pageSetup.CreateAttr("fitToHeight", "1")
	updated, err := document.WriteToBytes()
	return updated, true, err
}
