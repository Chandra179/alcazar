package financials

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func (r *AnalysisReport) Write(w io.Writer, format string) error {
	return r.WriteView(w, format, "brief")
}

func (r *AnalysisReport) WriteView(w io.Writer, format, view string) error {
	sections := r.Brief
	title := "BBCA fundamental analysis · FY2025"
	switch view {
	case "brief":
	case "evidence":
		title = "BBCA evidence companion · FY2025"
		sections = r.Appendix
	case "yearly":
		if len(r.Yearly) == 0 {
			return fmt.Errorf("the reviewed evidence has no yearly stories")
		}
		title = "BBCA Year by Year"
		sections = r.Yearly
	default:
		return fmt.Errorf("unknown analysis view %q", view)
	}
	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(r)
	case "markdown":
		var out strings.Builder
		fmt.Fprintln(&out, "# "+title)
		for index, section := range sections {
			if view == "yearly" && index > 0 {
				fmt.Fprintln(&out, "\n---")
			}
			if section.ID != "" {
				fmt.Fprintf(&out, "\n<a id=\"%s\"></a>\n", section.ID)
			}
			if section.Title != "" {
				fmt.Fprintln(&out, "\n## "+section.Title)
			}
			var trailing []reportCell
			for _, line := range section.Paragraphs {
				if view == "yearly" && strings.HasPrefix(section.ID, "year-") && line.Label == "Sources" {
					trailing = append(trailing, line)
					continue
				}
				prefix := "\n"
				if view == "yearly" && strings.HasPrefix(section.ID, "year-") {
					prefix = "\n- "
				}
				fmt.Fprintln(&out, prefix+markdownCell(line))
			}
			for _, table := range section.Tables {
				if table.Title != "" {
					fmt.Fprintln(&out, "\n### "+table.Title)
				}
				fmt.Fprintf(&out, "\n| %s |\n", strings.Join(table.Headers, " | "))
				fmt.Fprintf(&out, "|%s\n", strings.Repeat(" --- |", len(table.Headers)))
				for _, row := range table.Rows {
					group := len(row) > 0 && row[0].Text != ""
					for _, cell := range row[1:] {
						if cell.Text != "" || len(cell.Links) > 0 {
							group = false
							break
						}
					}
					cells := make([]string, 0, len(row))
					for _, cell := range row {
						if view == "yearly" {
							cell.Links = nil
						}
						cells = append(cells, strings.ReplaceAll(markdownCell(cell), "|", "\\|"))
					}
					if group {
						cells[0] = "**" + cells[0] + "**"
					}
					fmt.Fprintf(&out, "| %s |\n", strings.Join(cells, " | "))
				}
			}
			for _, line := range trailing {
				fmt.Fprintln(&out, "\n"+markdownCell(line))
			}
		}
		_, err := io.WriteString(w, out.String())
		return err
	default:
		return fmt.Errorf("unknown analysis format %q", format)
	}
}

func markdownCell(cell reportCell) string {
	var out []string
	if cell.Label != "" {
		out = append(out, "**"+cell.Label+".**")
	}
	out = append(out, cell.Text)
	for _, link := range cell.Links {
		out = append(out, fmt.Sprintf("[%s](%s)", link.Text, link.URL))
	}
	return strings.Join(out, " ")
}

func metricText(metric Metric) string {
	if metric.Value == nil {
		return "—"
	}
	switch metric.Unit {
	case "percent", "%":
		return fmt.Sprintf("%.2f%%", *metric.Value)
	case "ratio", "×":
		return fmt.Sprintf("%.2f×", *metric.Value)
	case "currency per share":
		return fmt.Sprintf("%.2f IDR/share", *metric.Value)
	default:
		return fmt.Sprintf("%.2f %s", *metric.Value, metric.Unit)
	}
}

func (r *AnalysisReport) findObservation(key, subject, period, basis, unit string) analysisObservation {
	for _, item := range r.Observations {
		if item.Key == key && item.Subject == subject && item.Period == period && item.Basis == basis && item.Unit == unit {
			return item
		}
	}
	return analysisObservation{}
}
