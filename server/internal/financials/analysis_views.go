package financials

import (
	"fmt"
	"strings"

	"github.com/Chandra179/lux/server/internal/report"
)

type (
	reportLink     = report.Link
	reportCell     = report.Cell
	reportTable    = report.Table
	reportSection  = report.Section
	yearlyTemplate = report.Template
)

func textCells(values ...string) []reportCell {
	row := make([]reportCell, 0, len(values))
	for _, value := range values {
		row = append(row, reportCell{Text: value})
	}
	return row
}

func pageURL(url string, page int) string {
	if page > 0 && url != "" {
		base, _, _ := strings.Cut(url, "#")
		return fmt.Sprintf("%s#page=%d", base, page)
	}
	return url
}

func (r *AnalysisReport) sourceLinks(keys ...string) []reportLink {
	var links []reportLink
	for _, key := range keys {
		if src, ok := r.Sources[key]; ok {
			links = append(links, reportLink{src.Title + " · " + src.Where, src.URL})
		}
	}
	return links
}
