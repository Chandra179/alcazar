package financials

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func (r *AnalysisReport) Write(w io.Writer, format string) error {
	if format == "json" {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(r)
	}
	if format != "markdown" {
		return fmt.Errorf("unknown analysis format %q", format)
	}
	var out strings.Builder
	fmt.Fprintln(&out, "# BBCA fundamental analysis · FY2025")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "FY2025 audited consolidated statements, FY1998–FY2025 annual history and the available September and October 2026 interim evidence.")
	fmt.Fprintf(&out, "\nCatalog SHA-256: `%s`. Reviewed catalog SHA-256: `%s`.\n", r.CatalogHash, r.ReviewedHash)
	if r.CatalogHash != r.ReviewedHash {
		fmt.Fprintln(&out, "\nThe catalog changed after the report evidence was reviewed. Recheck revised entries and entries marked `unreviewed_rule`.")
	}
	for _, section := range r.Sections {
		fmt.Fprintf(&out, "\n## %s\n\n%s\n", section.Title, r.assessmentText(section.analysisAssessment))
	}

	fmt.Fprintln(&out, "\n## FY2025 statement and valuation calculations")
	fmt.Fprintln(&out, "\n| Measure | Value |")
	fmt.Fprintln(&out, "| --- | ---: |")
	for _, id := range []string{"return-on-assets", "return-on-equity", "effective-tax-rate", "earnings-per-share",
		"total-liabilities-to-equity", "capital-expenditure-to-net-earnings", "net-change-in-cash",
		"bank-net-income-to-operating-income-2024", "bank-net-income-to-operating-income-2025",
		"pretax-bridge-residual-2024", "pretax-bridge-residual-2025", "cash-reconciliation-residual-2024", "cash-reconciliation-residual-2025",
		"price-earnings-ratio", "price-to-book-ratio", "dividend-yield", "fiscal-dividend-payout",
		"nominal-shareholder-return", "real-return-after-inflation", "dividend-value-conservative",
		"dividend-value-base", "dividend-value-favorable", "margin-of-safety"} {
		metric := r.Results[id]
		fmt.Fprintf(&out, "| %s | %s |\n", displayKey(id), metricText(metric))
	}
	fmt.Fprintln(&out, "\n### Base-case dividend value sensitivity (IDR per share)")
	fmt.Fprintln(&out, "\n| Cost of equity | Terminal growth 2% | 3% | 4% |")
	fmt.Fprintln(&out, "| --- | ---: | ---: | ---: |")
	for _, discount := range []int{10, 11, 12} {
		fmt.Fprintf(&out, "| %d%% |", discount)
		for _, growth := range []int{2, 3, 4} {
			fmt.Fprintf(&out, " %s |", percentMetric(r.Results[fmt.Sprintf("sensitivity-%d-%d", discount, growth)]))
		}
		fmt.Fprintln(&out)
	}
	fmt.Fprintln(&out, "\n## Annual financial history")
	fmt.Fprintln(&out, "\nMoney amounts are IDR trillion. EPS is in IDR on the October 2021 share basis and uses the issuer's reported basic EPS, split adjusted where needed. Historical highlights are rounded. A dash indicates a gap in the verified series. ROA and ROE use average opening and closing balances. Each annual return has its inputs and sources in the JSON output.")
	fmt.Fprintln(&out, "\n| FY | Assets | Parent equity | Group equity | Group profit | Parent profit | Basic EPS | ROA % | ROE % | Source or comparability note |")
	fmt.Fprintln(&out, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |")
	for _, row := range r.Annual {
		fmt.Fprintf(&out, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", row.Period,
			historyFact(row.Facts["total_assets"], true), historyFact(row.Facts["common_shareholders_equity"], true),
			historyFact(row.Facts["shareholders_equity"], true), historyFact(row.Facts["net_earnings"], true),
			historyFact(row.Facts["net_income_available_to_common_shareholders"], true), r.historyEPS(row),
			percentMetric(row.Values["return-on-assets"]), percentMetric(row.Values["return-on-equity"]), row.Note)
	}
	fmt.Fprintln(&out, "\n## 2026 interim snapshot")
	fmt.Fprintln(&out, "\n| Period | Reporting basis | Assets (IDR T) | Group or bank equity (IDR T) | YTD group profit (IDR T) | Coverage |")
	fmt.Fprintln(&out, "| --- | --- | ---: | ---: | ---: | --- |")
	for _, row := range r.Interim {
		fmt.Fprintf(&out, "| %s | %s | %s | %s | %s | %s |\n", row.Period, row.Basis,
			r.interimPosition(row, "total_assets", "Closing assets"), r.interimPosition(row, "shareholders_equity", "Closing equity"),
			historyFact(row.Facts["net_earnings"], true), row.Note)
	}

	fmt.Fprintln(&out, "\n## Banking disclosures")
	fmt.Fprintln(&out, "\n| Measure | Basis | Unit | FY2024 | FY2025 |")
	fmt.Fprintln(&out, "| --- | --- | --- | ---: | ---: |")
	for _, key := range []string{"Gross loans and financing", "Corporate loans", "Third-party funds", "CASA ratio", "Gross NPL", "Loan at risk", "Net interest margin",
		"Cost to income", "Impairment losses", "Capital adequacy ratio", "Liquidity coverage ratio", "Net stable funding ratio", "Loan to deposit ratio"} {
		for _, basis := range []string{"consolidated", "bank only"} {
			for _, unit := range []string{"%", "IDR million", "IDR billion"} {
				before := r.findObservation(key, "BBCA", "2024", basis, unit)
				after := r.findObservation(key, "BBCA", "2025", basis, unit)
				if before.Value != nil || after.Value != nil {
					fmt.Fprintf(&out, "| %s | %s | %s | %s | %s |\n", key, basis, unit, r.observationLink(before), r.observationLink(after))
				}
			}
		}
	}
	fmt.Fprintln(&out, "\n### FY2025 peer comparison, bank only")
	fmt.Fprintln(&out, "\n| Measure | BBCA | BBRI | BMRI | Comparison |")
	fmt.Fprintln(&out, "| --- | ---: | ---: | ---: | --- |")
	for _, row := range r.PeerRows {
		fmt.Fprintf(&out, "| %s |", row.Key)
		for _, symbol := range []string{"BBCA", "BBRI", "BMRI"} {
			item := analysisObservation{}
			for _, observation := range row.Observations {
				if observation.Subject == symbol {
					item = observation
				}
			}
			fmt.Fprintf(&out, " %s |", r.observationLink(item))
		}
		fmt.Fprintf(&out, " %s |\n", row.Status)
	}

	fmt.Fprintln(&out, "\n## Rule appendix")
	for _, rule := range r.Rules {
		fmt.Fprintf(&out, "\n### %d. %s\n\n", rule.Number, rule.Rule.ID)
		fmt.Fprintf(&out, "**%s · %s**\n\n", rule.Rule.Type, displayKey(strings.ReplaceAll(rule.Status, "_", "-")))
		if rule.Metric != nil {
			fmt.Fprintf(&out, "%s\n\n", metricText(*rule.Metric))
		}
		fmt.Fprintln(&out, r.assessmentText(rule.analysisAssessment))
	}
	_, err := io.WriteString(w, out.String())
	return err
}

func metricText(metric Metric) string {
	if metric.Value == nil {
		if metric.Reason != "" {
			return metric.Reason
		}
		return "Unavailable"
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

func (r *AnalysisReport) assessmentText(a analysisAssessment) string {
	var parts []string
	parts = append(parts, a.Finding)
	if a.Gap != "" {
		parts = append(parts, a.Gap)
	}
	for _, key := range a.Sources {
		s := r.Sources[key]
		parts = append(parts, fmt.Sprintf("[%s, %s](%s)", s.Title, s.Where, s.URL))
	}
	return strings.Join(parts, " ")
}

func (r *AnalysisReport) observationLink(item analysisObservation) string {
	if item.Value == nil {
		return "—"
	}
	source := r.Sources[item.Source]
	return fmt.Sprintf("[%.2f](%s)", *item.Value, source.URL)
}

func (r *AnalysisReport) findObservation(key, subject, period, basis, unit string) analysisObservation {
	for _, item := range r.Observations {
		if item.Key == key && item.Subject == subject && item.Period == period && item.Basis == basis && item.Unit == unit {
			return item
		}
	}
	return analysisObservation{}
}

func (r *AnalysisReport) historyEPS(row analysisPeriod) string {
	eps := r.findObservation("Basic EPS on 2025 share basis", "BBCA", row.Period, "consolidated", "IDR/share")
	if eps.Value != nil {
		return r.observationLink(eps)
	}
	return historyFact(row.Facts["reported_earnings_per_share"], false)
}

func (r *AnalysisReport) interimPosition(row analysisPeriod, factKey, supplementalKey string) string {
	if fact, ok := row.Facts[factKey]; ok {
		return historyFact(fact, true)
	}
	item := r.findObservation(supplementalKey, "BBCA", row.Period, row.Basis, "IDR million")
	if item.Value == nil {
		return "—"
	}
	trillions := *item.Value / 1e6
	item.Value = &trillions
	return r.observationLink(item)
}
