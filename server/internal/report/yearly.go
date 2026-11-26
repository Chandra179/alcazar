package report

import "slices"

// PrepareYearly resolves layout roles without changing source paragraphs, tables, or section order.
// The prepared sections can feed both Go writers and the PDF adapter.
func PrepareYearly(sections []Section) []Section {
	result := slices.Clone(sections)
	for i := range result {
		section := &result[i]
		layout := &SectionLayout{}
		switch section.Kind {
		case "annual", "interim", "analysis":
			layout.Takeaways = true
		}
		for index, cell := range section.Paragraphs {
			if cell.Label == "Sources" {
				layout.SourceRows = append(layout.SourceRows, index)
			}
		}
		section.Layout = layout
		section.Tables = slices.Clone(section.Tables)
		for index := range section.Tables {
			table := &section.Tables[index]
			table.Layout = prepareYearlyTable(*table, section.Kind == "cover")
		}
	}
	return result
}

func prepareYearlyTable(table Table, cover bool) *TableLayout {
	layout := &TableLayout{
		Contents:    cover && slices.Equal(table.Headers, []string{"Section", "Coverage"}),
		ColumnRoles: make([]string, len(table.Headers)),
	}
	for _, column := range table.NumericColumns {
		if column >= 0 && column < len(layout.ColumnRoles) {
			layout.ColumnRoles[column] = "numeric"
		}
	}
	if column := table.CurrentColumn; column > 0 && column < len(layout.ColumnRoles) {
		layout.ColumnRoles[column] = "current"
	}
	for i, row := range table.Rows {
		if len(row) > 1 && groupRow(row) {
			layout.GroupRows = append(layout.GroupRows, i)
		}
	}
	return layout
}

// YearlyContents links to sections in reading order and collapses consecutive annual entries.
func YearlyContents(sections []Section) Table {
	table := Table{Title: "In this report", Headers: []string{"Section", "Coverage"}}
	for i := 0; i < len(sections); i++ {
		section := sections[i]
		title, coverage := section.Title, ""
		switch section.Kind {
		case "interim":
			title, coverage = "Interim update", section.PeriodLabel
		case "annual":
			title, coverage = "Annual comparison", section.PeriodLabel
			first := i
			for i+1 < len(sections) && sections[i+1].Kind == "annual" {
				i++
			}
			if i != first {
				coverage += "–" + sections[i].PeriodLabel
			}
		case "glossary":
			coverage = "Definitions and comparison notes"
		}
		table.Rows = append(table.Rows, []Cell{
			{Links: []Link{{Text: title, URL: "#" + section.ID}}}, {Text: coverage},
		})
	}
	return table
}
