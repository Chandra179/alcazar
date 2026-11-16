package financials

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

type xlsxCandidate struct {
	Sheet    string
	Cell     string
	Label    string
	Value    string
	Evidence string
	Number   float64
	IsNumber bool
	Currency string
	Unit     string
}

type xlsxCellValue struct {
	cell    string
	text    string
	number  float64
	numeric bool
}

func extractXLSX(filename string) ([]xlsxCandidate, error) {
	workbook, err := excelize.OpenFile(filename)
	if err != nil {
		return nil, fmt.Errorf("open workbook: %w", err)
	}
	defer func() { _ = workbook.Close() }()

	var candidates []xlsxCandidate
	for _, sheet := range workbook.GetSheetList() {
		rows, err := workbook.GetRows(sheet)
		if err != nil {
			return nil, fmt.Errorf("read sheet %q: %w", sheet, err)
		}
		for rowIndex, row := range rows {
			values := make([]xlsxCellValue, 0, len(row))
			for columnIndex, displayValue := range row {
				cellRef, err := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+1)
				if err != nil {
					return nil, fmt.Errorf("build cell address for %s row %d: %w", sheet, rowIndex+1, err)
				}
				displayValue = strings.TrimSpace(displayValue)
				if displayValue == "" {
					continue
				}
				number, numeric := ParseNumber(displayValue)
				values = append(values, xlsxCellValue{
					cell: cellRef, text: displayValue, number: number, numeric: numeric,
				})
			}
			label := ""
			for _, value := range values {
				if value.numeric {
					continue
				}
				if detectCanonicalKey(value.text) != "" {
					label = value.text
					break
				}
			}
			if label == "" {
				continue
			}
			var rowParts []string
			for _, value := range values {
				rowParts = append(rowParts, value.cell+"="+value.text)
			}
			evidence := strings.Join(rowParts, " | ")
			for _, value := range values {
				if !value.numeric {
					continue
				}
				currency, unit := displayedCellMetadata(value.text)
				candidates = append(candidates, xlsxCandidate{
					Sheet: sheet, Cell: value.cell, Label: label, Value: value.text,
					Evidence: evidence, Number: value.number, IsNumber: true,
					Currency: currency, Unit: unit,
				})
			}
		}
	}
	return candidates, nil
}

func displayedCellMetadata(value string) (string, string) {
	lower := strings.ToLower(value)
	currency := ""
	if strings.Contains(lower, "rupiah") || strings.Contains(lower, "rp ") || strings.Contains(lower, "rp.") {
		currency = "IDR"
	}
	unit := ""
	if strings.Contains(value, "%") {
		unit = "percent"
	}
	return currency, unit
}
