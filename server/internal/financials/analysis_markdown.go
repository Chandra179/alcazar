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
		for _, section := range sections {
			if section.ID != "" {
				fmt.Fprintf(&out, "\n<a id=\"%s\"></a>\n", section.ID)
			}
			if section.Title != "" {
				fmt.Fprintln(&out, "\n## "+section.Title)
			}
			for _, line := range section.Paragraphs {
				fmt.Fprintln(&out, "\n"+markdownCell(line))
			}
			for _, table := range section.Tables {
				if table.Title != "" {
					fmt.Fprintln(&out, "\n### "+table.Title)
				}
				fmt.Fprintf(&out, "\n| %s |\n", strings.Join(table.Headers, " | "))
				fmt.Fprintf(&out, "|%s\n", strings.Repeat(" --- |", len(table.Headers)))
				for _, row := range table.Rows {
					cells := make([]string, 0, len(row))
					for _, cell := range row {
						cells = append(cells, strings.ReplaceAll(markdownCell(cell), "|", "\\|"))
					}
					fmt.Fprintf(&out, "| %s |\n", strings.Join(cells, " | "))
				}
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
