package renderer

import (
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// extractWorkbookText extracts the content of every worksheet from an XLSX or
// XLSM workbook. Each sheet becomes one TextPart whose text is a lightweight
// markup of rows separated by tabs.
func extractWorkbookText(source string) ([]TextPart, error) {
	workbook, err := excelize.OpenFile(source, excelize.Options{Password: ""})
	if err != nil {
		return nil, err
	}
	defer workbook.Close()
	parts := make([]TextPart, 0, len(workbook.GetSheetList()))
	for index, sheet := range workbook.GetSheetList() {
		rows, err := workbook.GetRows(sheet, excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, err
		}
		// Wrap the sheet content in tags so the sheet name is preserved.
		lines := make([]string, 0, len(rows)+2)
		lines = append(lines, `<sheet name=`+strconv.Quote(sheet)+`>`)
		for _, row := range rows {
			lines = append(lines, strings.Join(row, "\t"))
		}
		lines = append(lines, "</sheet>")
		parts = append(parts, TextPart{PartNumber: index + 1, Text: strings.Join(lines, "\n")})
	}
	return parts, nil
}
