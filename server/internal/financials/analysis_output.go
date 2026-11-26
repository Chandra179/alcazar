package financials

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Chandra179/lux/server/internal/report"
)

func (r *AnalysisReport) Write(w io.Writer, format string) error {
	return r.WriteView(w, format, "yearly")
}

func (r *AnalysisReport) WriteView(w io.Writer, format, view string) error {
	if view != "yearly" {
		return fmt.Errorf("unknown analysis view %q (use yearly)", view)
	}
	if len(r.Yearly) == 0 {
		return fmt.Errorf("the reviewed evidence has no yearly stories")
	}
	sections := r.Yearly
	template := yearlyTemplate{ID: report.StandardTemplate, Title: "BBCA Year by Year"}
	if r.YearlyTemplate != nil {
		template = *r.YearlyTemplate
	}
	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(r)
	case "markdown":
		return report.WriteMarkdown(w, template.Title, sections)
	case "html":
		return report.WriteYearlyHTML(w, template, sections)
	default:
		return fmt.Errorf("unknown analysis format %q", format)
	}
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
