package financials

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

const officeRelNamespace = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

type xlsxSheetRef struct {
	Name string `xml:"name,attr"`
	RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
}

type xlsxWorkbook struct {
	Sheets []xlsxSheetRef `xml:"sheets>sheet"`
}

type xlsxRelationship struct {
	ID     string `xml:"Id,attr"`
	Target string `xml:"Target,attr"`
}

type xlsxRelationships struct {
	Items []xlsxRelationship `xml:"Relationship"`
}

type xlsxCell struct {
	Ref   string `xml:"r,attr"`
	Type  string `xml:"t,attr"`
	Value string `xml:"v"`
	Text  struct {
		Value string `xml:"t"`
		Runs  []struct {
			Value string `xml:"t"`
		} `xml:"r"`
	} `xml:"is"`
}

type xlsxRow struct {
	Cells []xlsxCell `xml:"c"`
}

type xlsxWorksheet struct {
	Rows []xlsxRow `xml:"sheetData>row"`
}

type xlsxCandidate struct {
	Sheet    string
	Cell     string
	Label    string
	Value    string
	Evidence string
	Number   float64
	IsNumber bool
}

func extractXLSX(filename string) ([]xlsxCandidate, error) {
	archive, err := zip.OpenReader(filename)
	if err != nil {
		return nil, fmt.Errorf("open workbook: %w", err)
	}
	defer func() { _ = archive.Close() }()
	files := make(map[string]*zip.File, len(archive.File))
	for _, file := range archive.File {
		files[file.Name] = file
	}
	workbookData, err := readZipFile(files, "xl/workbook.xml")
	if err != nil {
		return nil, err
	}
	var workbook xlsxWorkbook
	if err := xml.Unmarshal(workbookData, &workbook); err != nil {
		return nil, fmt.Errorf("parse workbook metadata: %w", err)
	}
	relsData, err := readZipFile(files, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return nil, err
	}
	var relationships xlsxRelationships
	if err := xml.Unmarshal(relsData, &relationships); err != nil {
		return nil, fmt.Errorf("parse workbook relationships: %w", err)
	}
	relTargets := make(map[string]string, len(relationships.Items))
	for _, relationship := range relationships.Items {
		target := strings.TrimPrefix(relationship.Target, "/")
		if !strings.HasPrefix(target, "xl/") {
			target = path.Join("xl", target)
		}
		relTargets[relationship.ID] = path.Clean(target)
	}
	shared := readSharedStrings(files["xl/sharedStrings.xml"])

	var candidates []xlsxCandidate
	for _, sheet := range workbook.Sheets {
		sheetPath, ok := relTargets[sheet.RID]
		if !ok {
			continue
		}
		sheetData, err := readZipFile(files, sheetPath)
		if err != nil {
			return nil, fmt.Errorf("read sheet %q: %w", sheet.Name, err)
		}
		var parsed xlsxWorksheet
		if err := xml.Unmarshal(sheetData, &parsed); err != nil {
			return nil, fmt.Errorf("parse sheet %q: %w", sheet.Name, err)
		}
		for _, row := range parsed.Rows {
			var values []struct {
				cell    string
				text    string
				number  float64
				numeric bool
			}
			for _, cell := range row.Cells {
				value := cell.Value
				switch cell.Type {
				case "s":
					if index, parseErr := strconv.Atoi(value); parseErr == nil && index >= 0 && index < len(shared) {
						value = shared[index]
					}
				case "inlineStr":
					value = cell.Text.Value
					for _, run := range cell.Text.Runs {
						value += run.Value
					}
				}
				if strings.TrimSpace(value) != "" {
					number, numeric := 0.0, false
					if cell.Type == "" || cell.Type == "n" {
						number, err = strconv.ParseFloat(value, 64)
						numeric = err == nil
					}
					if !numeric {
						number, numeric = ParseNumber(value)
					}
					values = append(values, struct {
						cell    string
						text    string
						number  float64
						numeric bool
					}{cell: cell.Ref, text: strings.TrimSpace(value), number: number, numeric: numeric})
				}
			}
			var label string
			for _, value := range values {
				if !value.numeric {
					label = value.text
					if detectCanonicalKey(label) != "" {
						break
					}
				}
			}
			if label == "" && len(values) > 0 {
				label = values[0].text
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
				candidates = append(candidates, xlsxCandidate{
					Sheet: sheet.Name, Cell: value.cell, Label: label, Value: value.text,
					Evidence: evidence, Number: value.number, IsNumber: true,
				})
			}
		}
	}
	return candidates, nil
}

func readZipFile(files map[string]*zip.File, name string) ([]byte, error) {
	file := files[name]
	if file == nil {
		return nil, fmt.Errorf("workbook entry %q is missing", name)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open workbook entry %q: %w", name, err)
	}
	defer func() { _ = reader.Close() }()
	return io.ReadAll(reader)
}

func readSharedStrings(file *zip.File) []string {
	if file == nil {
		return nil
	}
	reader, err := file.Open()
	if err != nil {
		return nil
	}
	defer func() { _ = reader.Close() }()
	decoder := xml.NewDecoder(reader)
	var values []string
	var current strings.Builder
	insideItem := false
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch element := token.(type) {
		case xml.StartElement:
			if element.Name.Local == "si" {
				insideItem = true
				current.Reset()
			}
			if insideItem && element.Name.Local == "t" {
				var text string
				if decoder.DecodeElement(&text, &element) == nil {
					current.WriteString(text)
				}
			}
		case xml.EndElement:
			if element.Name.Local == "si" && insideItem {
				values = append(values, current.String())
				insideItem = false
			}
		}
	}
	return values
}
