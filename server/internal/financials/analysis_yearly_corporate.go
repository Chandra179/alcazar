package financials

import "fmt"

// yearlyCorporate builds a general corporate profitability, cash-flow, and leverage section
// when `company.industry == "corporate"` and corporate observations are present.
func (r *AnalysisReport) yearlyCorporate() *reportSection {
	if r.profile().Industry != "corporate" || len(r.Annual) == 0 {
		return nil
	}
	latest := r.Annual[len(r.Annual)-1].Period
	prior := ""
	if len(r.Annual) > 1 {
		prior = r.Annual[len(r.Annual)-2].Period
	}
	ticker := r.companyTicker()

	obsText := func(key, period, unit string) string {
		if period == "" {
			return "—"
		}
		item := r.findObservation(key, ticker, period, "consolidated", unit)
		if item.Value == nil {
			return "—"
		}
		if unit == "%" {
			return fmt.Sprintf("%.2f%%", *item.Value)
		}
		if unit == "x" {
			return fmt.Sprintf("%.2f×", *item.Value)
		}
		return fmt.Sprintf("%.2fT", *item.Value)
	}

	table := reportTable{
		Title:          "Corporate performance & leverage",
		Headers:        []string{"Measure", "Prior", "FY " + latest, "Scope"},
		NumericColumns: []int{1, 2},
		CurrentColumn:  2,
	}
	for _, m := range []struct{ key, label, unit string }{
		{"Revenue", "Revenue", "T"},
		{"Operating profit", "Operating profit", "T"},
		{"EBITDA", "EBITDA", "T"},
		{"Free cash flow", "Free cash flow", "T"},
		{"Gross margin", "Gross margin", "%"},
		{"Operating margin", "Operating margin", "%"},
		{"ROIC", "ROIC", "%"},
		{"Net debt / EBITDA", "Net debt / EBITDA", "x"},
		{"Interest coverage", "Interest coverage", "x"},
	} {
		pVal := obsText(m.key, prior, m.unit)
		cVal := obsText(m.key, latest, m.unit)
		if pVal == "—" && cVal == "—" {
			continue
		}
		table.Rows = append(table.Rows, textCells(m.label, pVal, cVal, "Group"))
	}
	if len(table.Rows) == 0 {
		return nil
	}
	return &reportSection{
		ID:     "yearly-corporate",
		Kind:   "analysis",
		Title:  "Corporate performance & leverage",
		Tables: []reportTable{table},
	}
}
