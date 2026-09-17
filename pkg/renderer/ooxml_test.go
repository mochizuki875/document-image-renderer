package renderer

import (
	"bytes"
	"strings"
	"testing"
)

func TestNormalizePPTXNegativeLineExtents(t *testing.T) {
	xml := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
  <p:cSld><p:spTree><p:sp><p:spPr><a:xfrm><a:off x="6400800" y="3547872"/><a:ext cx="-960120" cy="886968"/></a:xfrm><a:prstGeom prst="line"/></p:spPr></p:sp></p:spTree></p:cSld>
</p:sld>`)

	updated, changed, err := normalizePPTXPart("ppt/slides/slide1.xml", xml)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || !bytes.Contains(updated, []byte(`x="5440680"`)) || !bytes.Contains(updated, []byte(`cx="960120"`)) {
		t.Fatalf("negative extent was not normalized: %s", updated)
	}
}

func TestFitWorksheetToOneLandscapePage(t *testing.T) {
	xml := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData/><pageMargins left="0.7"/><pageSetup scale="75"/></worksheet>`)

	updated, changed, err := fitWorksheetPart("xl/worksheets/sheet1.xml", xml)
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	for _, expected := range []string{`fitToPage="1"`, `orientation="landscape"`, `fitToWidth="1"`, `fitToHeight="1"`} {
		if !changed || !strings.Contains(text, expected) {
			t.Fatalf("missing %s in %s", expected, text)
		}
	}
	if strings.Contains(text, `scale=`) {
		t.Fatalf("scale attribute must be removed: %s", text)
	}
}
