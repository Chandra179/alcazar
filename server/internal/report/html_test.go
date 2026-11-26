package report

import (
	"strings"
	"testing"
)

func TestStandardTemplateRendersNonBankReport(t *testing.T) {
	sections := []Section{
		{ID: "cover", Kind: "cover", Paragraphs: []Cell{{Text: "Manufacturing results."}}},
		{ID: "current", Kind: "interim", Title: "Q1 2027 — Shipments", PeriodLabel: "Q1 2027",
			Paragraphs: []Cell{
				{Label: "Sources", Links: []Link{{Text: "Quarterly filing", URL: "https://example.test/filing?a=1&b=2"}}},
				{Label: "Operations", Text: "Demand for <widgets> grew."},
			},
			Tables: []Table{{
				Title: "Shipment volumes", Headers: []string{"Measure", "Prior", "Current"},
				NumericColumns: []int{1, 2}, CurrentColumn: 2,
				Rows: [][]Cell{
					{{Text: "Products"}, {}, {}},
					{{Text: "Widgets"}, {Text: "12"}, {Text: "15"}},
				},
			}},
		},
		{ID: "annual", Kind: "annual", Title: "Fiscal 2026 — Results", PeriodLabel: "Fiscal 2026"},
		{ID: "glossary", Kind: "glossary", Title: "Glossary"},
	}
	sections[0].Tables = []Table{YearlyContents(sections[1:])}
	prepared := PrepareYearly(sections)
	if sections[1].Layout != nil || sections[1].Tables[0].Layout != nil {
		t.Fatal("preparing the layout mutated the source report")
	}
	var out strings.Builder
	metadata := Template{ID: StandardTemplate, Title: "Manufacturing & Co", Footer: "Financial history"}
	if err := WriteYearlyHTML(&out, metadata, prepared); err != nil {
		t.Fatal(err)
	}
	document := out.String()
	for _, fragment := range []string{
		"<title>Manufacturing &amp; Co</title>", `href="#current"`, `href="#annual"`, `href="#glossary"`,
		"Fiscal 2026", "Demand for &lt;widgets&gt; grew.", `colspan="3">Products`,
		`<td class="current">15</td>`, `href="https://example.test/filing?a=1&amp;b=2"`,
		"<footer>Financial history</footer>",
	} {
		if !strings.Contains(document, fragment) {
			t.Errorf("HTML missing %q", fragment)
		}
	}
	table := strings.Index(document, "<caption>Shipment volumes</caption>")
	source := strings.Index(document, "Quarterly filing")
	if table < 0 || source <= table {
		t.Fatal("sources should follow the section tables")
	}
}
