package report

import (
	"fmt"
	"io"
	"strings"
)

func WriteMarkdown(w io.Writer, title string, sections []Section, yearly bool) error {
	var out strings.Builder
	fmt.Fprintln(&out, "# "+title)
	for i, section := range sections {
		if yearly && i > 0 {
			fmt.Fprintln(&out, "\n---")
		}
		if section.ID != "" {
			fmt.Fprintf(&out, "\n<a id=\"%s\"></a>\n", section.ID)
		}
		if section.Title != "" {
			fmt.Fprintln(&out, "\n## "+section.Title)
		}
		prefix := "\n"
		takeaways := section.Layout != nil && section.Layout.Takeaways
		if takeaways {
			prefix = "\n- "
		}
		for _, cell := range section.Body() {
			fmt.Fprintln(&out, prefix+markdownCell(cell, " "))
		}
		for _, table := range section.Tables {
			writeMarkdownTable(&out, table, yearly, takeaways)
		}
		for _, cell := range section.Sources() {
			fmt.Fprintln(&out, "\n"+markdownCell(cell, " · "))
		}
	}
	_, err := io.WriteString(w, out.String())
	return err
}

func writeMarkdownTable(out *strings.Builder, table Table, yearly, takeaways bool) {
	if table.Title != "" {
		fmt.Fprintln(out, "\n### "+table.Title)
	}
	fmt.Fprintln(out)
	headers := make([]string, len(table.Headers))
	alignments := make([]string, len(table.Headers))
	for i, header := range table.Headers {
		headers[i], alignments[i] = header, "---"
		role := table.ColumnRole(i)
		if role != "" {
			alignments[i] = "---:"
		}
		if role == "current" {
			headers[i] = bold(header)
		}
	}
	writeMarkdownRow(out, headers)
	writeMarkdownRow(out, alignments)
	for i, row := range table.Rows {
		group := table.Group(i, row)
		cells := make([]string, len(row))
		for column, cell := range row {
			role := table.ColumnRole(column)
			if takeaways && role != "" {
				cell.Links = nil
			}
			value := markdownCell(cell, " ")
			if yearly {
				value = strings.TrimSpace(value)
			}
			value = strings.ReplaceAll(value, "|", "\\|")
			if (group && column == 0) || (!group && role == "current" && value != "") {
				value = bold(value)
			}
			cells[column] = value
		}
		writeMarkdownRow(out, cells)
	}
}

func writeMarkdownRow(out *strings.Builder, cells []string) {
	fmt.Fprintf(out, "| %s |\n", strings.Join(cells, " | "))
}

func bold(value string) string {
	return "**" + value + "**"
}

func markdownCell(cell Cell, separator string) string {
	var parts []string
	if cell.Label != "" {
		parts = append(parts, bold(cell.Label+"."))
	}
	parts = append(parts, cell.Text)
	if len(cell.Links) > 0 {
		links := make([]string, 0, len(cell.Links))
		for _, link := range cell.Links {
			links = append(links, fmt.Sprintf("[%s](%s)", link.Text, link.URL))
		}
		parts = append(parts, strings.Join(links, separator))
	}
	return strings.Join(parts, " ")
}
