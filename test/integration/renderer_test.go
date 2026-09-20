package integration_test

import (
	"context"
	"crypto/sha256"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mochizuki875/document-image-renderer/pkg/renderer"
	"github.com/xuri/excelize/v2"
)

func TestRenderFixtureDocuments(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run document conversions")
	}
	documents, err := filepath.Glob(filepath.Join("..", "documents", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) == 0 {
		t.Fatal("no fixture documents found")
	}

	for _, source := range documents {
		source := source
		t.Run(filepath.Base(source), func(t *testing.T) {
			if filepath.Ext(source) != ".pdf" && !libreOfficeAvailable() {
				t.Skip("LibreOffice is not installed")
			}
			before, err := fileHash(source)
			if err != nil {
				t.Fatal(err)
			}
			result, err := renderer.RenderDocument(context.Background(), source, t.TempDir(), nil)
			if err != nil {
				t.Fatalf("render document: %v", err)
			}
			if result.PageCount() == 0 {
				t.Fatal("expected at least one rendered page")
			}
			for _, rendered := range result.Images {
				assertImage(t, rendered)
			}
			extracted, err := renderer.ExtractDocument(context.Background(), source)
			if err != nil {
				t.Fatalf("extract document: %v", err)
			}
			if extracted.PartCount() == 0 {
				t.Fatal("expected at least one extracted text part")
			}
			switch filepath.Ext(source) {
			case ".xls", ".xlsx", ".xlsm":
				if result.PageCount() != extracted.PartCount() {
					t.Fatalf("rendered images = %d, worksheets = %d", result.PageCount(), extracted.PartCount())
				}
			}
			after, err := fileHash(source)
			if err != nil {
				t.Fatal(err)
			}
			if before != after {
				t.Fatal("source document was modified")
			}
		})
	}
}

func TestRenderWorkbookCreatesOneImagePerSheet(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run document conversions")
	}
	if !libreOfficeAvailable() {
		t.Skip("LibreOffice is not installed")
	}

	workbook := excelize.NewFile()
	defer workbook.Close()
	if err := workbook.SetSheetName("Sheet1", "First"); err != nil {
		t.Fatal(err)
	}
	if _, err := workbook.NewSheet("Second"); err != nil {
		t.Fatal(err)
	}
	for _, sheet := range workbook.GetSheetList() {
		if err := workbook.SetCellValue(sheet, "A1", sheet); err != nil {
			t.Fatal(err)
		}
		if err := workbook.SetCellValue(sheet, "AZ200", "forces a multi-page print range"); err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(t.TempDir(), "wide.xlsx")
	if err := workbook.SaveAs(source); err != nil {
		t.Fatal(err)
	}

	result, err := renderer.RenderDocument(context.Background(), source, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("render workbook: %v", err)
	}
	if result.PageCount() != 2 {
		t.Fatalf("rendered images = %d, want one for each of 2 sheets", result.PageCount())
	}
}

func libreOfficeAvailable() bool {
	if _, err := exec.LookPath("libreoffice"); err == nil {
		return true
	}
	_, err := exec.LookPath("soffice")
	return err == nil
}

func fileHash(path string) ([sha256.Size]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(data), nil
}

func assertImage(t *testing.T, rendered renderer.RenderedImage) {
	t.Helper()
	input, err := os.Open(rendered.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	decoded, _, err := image.Decode(input)
	if err != nil {
		t.Fatalf("decode rendered image: %v", err)
	}
	if rendered.Width <= 0 || rendered.Height <= 0 {
		t.Fatalf("invalid image dimensions: %+v", rendered)
	}
	if decoded.Bounds().Dx() != rendered.Width || decoded.Bounds().Dy() != rendered.Height {
		t.Fatalf("metadata dimensions do not match image: %+v", rendered)
	}
}
