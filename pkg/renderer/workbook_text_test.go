package renderer

import (
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestExtractWorkbookTextReadsSheetsInOrder(t *testing.T) {
	workbook := excelize.NewFile()
	defer workbook.Close()
	if err := workbook.SetSheetName("Sheet1", "First"); err != nil {
		t.Fatal(err)
	}
	if _, err := workbook.NewSheet("Second"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCellValue("First", "A1", "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCellValue("Second", "B2", "beta"); err != nil {
		t.Fatal(err)
	}
	source := writeWorkbookFixture(t, workbook)

	options := DefaultExtractOptions()
	parts, err := extractWorkbookText(source, options, &characterBudget{})
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0].PartNumber != 1 || !strings.Contains(parts[0].Text, "First") || !strings.Contains(parts[0].Text, "alpha") {
		t.Fatalf("unexpected first part: %#v", parts[0])
	}
	if parts[1].PartNumber != 2 || !strings.Contains(parts[1].Text, "Second") || !strings.Contains(parts[1].Text, "beta") {
		t.Fatalf("unexpected second part: %#v", parts[1])
	}
}

func TestExtractWorkbookTextEnforcesCharacterBudget(t *testing.T) {
	workbook := excelize.NewFile()
	defer workbook.Close()
	if err := workbook.SetCellValue("Sheet1", "A1", "hello"); err != nil {
		t.Fatal(err)
	}
	source := writeWorkbookFixture(t, workbook)

	options := DefaultExtractOptions()
	options.MaxCharacters = 1
	_, err := extractWorkbookText(source, options, &characterBudget{max: options.MaxCharacters})
	var exceeded *CharacterLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected CharacterLimitExceededError, got %v", err)
	}
}

func TestExtractWorkbookTextRejectsInvalidArchive(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{"xl/workbook.xml": "<workbook/>"})
	options := DefaultExtractOptions()
	if _, err := extractWorkbookText(source, options, &characterBudget{}); err == nil {
		t.Fatal("expected error for invalid workbook archive")
	}
}

func TestExtractWorkbookTextEnforcesOOXMLLimits(t *testing.T) {
	workbook := excelize.NewFile()
	defer workbook.Close()
	if err := workbook.SetCellValue("Sheet1", "A1", "data"); err != nil {
		t.Fatal(err)
	}
	source := writeWorkbookFixture(t, workbook)

	options := DefaultExtractOptions()
	options.MaxOOXMLMembers = 1
	_, err := extractWorkbookText(source, options, &characterBudget{})
	var exceeded *OOXMLLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected OOXMLLimitExceededError, got %v", err)
	}
}

func writeWorkbookFixture(t *testing.T, workbook *excelize.File) string {
	t.Helper()
	path := t.TempDir() + "/workbook.xlsx"
	if err := workbook.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}
