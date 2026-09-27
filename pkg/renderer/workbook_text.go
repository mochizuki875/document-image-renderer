package renderer

import (
	"math"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

func extractWorkbookText(source string, options ExtractOptions, budget *characterBudget) ([]TextPart, error) {
	limits := extractOOXMLLimits(options)
	archive, err := openAndCheckOOXMLHeaders(source, limits)
	if err != nil {
		return nil, err
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	unzipSizeLimit := int64(math.MaxInt64)
	if limits.maxTotalBytes > 0 && limits.maxTotalBytes <= math.MaxInt64 {
		unzipSizeLimit = int64(limits.maxTotalBytes)
	}
	unzipXMLSizeLimit := unzipSizeLimit
	if limits.maxMemberBytes > 0 && limits.maxMemberBytes <= uint64(unzipSizeLimit) {
		unzipXMLSizeLimit = int64(limits.maxMemberBytes)
	}
	workbook, err := excelize.OpenFile(source, excelize.Options{
		Password: "", UnzipSizeLimit: unzipSizeLimit, UnzipXMLSizeLimit: unzipXMLSizeLimit,
	})
	if err != nil {
		if strings.Contains(err.Error(), "unzip size exceeds") {
			return nil, &OOXMLLimitExceededError{LimitType: "total uncompressed bytes", Actual: limits.maxTotalBytes + 1, Limit: limits.maxTotalBytes}
		}
		return nil, err
	}
	defer workbook.Close()
	parts := make([]TextPart, 0, len(workbook.GetSheetList()))
	for index, sheet := range workbook.GetSheetList() {
		rows, err := workbook.Rows(sheet)
		if err != nil {
			return nil, err
		}
		var text strings.Builder
		if err := budget.append(&text, `<sheet name=`+strconv.Quote(sheet)+`>`); err != nil {
			_ = rows.Close()
			return nil, err
		}
		for rows.Next() {
			columns, err := rows.Columns(excelize.Options{RawCellValue: true})
			if err != nil {
				_ = rows.Close()
				return nil, err
			}
			if err := budget.append(&text, "\n"+strings.Join(columns, "\t")); err != nil {
				_ = rows.Close()
				return nil, err
			}
		}
		if err := rows.Error(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if err := budget.append(&text, "\n</sheet>"); err != nil {
			return nil, err
		}
		parts = append(parts, TextPart{PartNumber: index + 1, Text: text.String()})
	}
	return parts, nil
}
