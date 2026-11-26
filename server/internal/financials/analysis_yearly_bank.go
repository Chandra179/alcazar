package financials

import "fmt"

func (r *AnalysisReport) yearlyBankRisk() reportSection {
	compare := reportTable{Headers: []string{"Measure", "Dec 2025", "Sep 2026", "Scope"},
		NumericColumns: []int{1, 2}, CurrentColumn: 2}
	for _, measure := range []struct{ id, label string }{
		{bankStage2Rule, "Gross Stage 2 loan share"},
		{bankStage3Rule, "Gross Stage 3 loan share"},
	} {
		row := textCells(measure.label)
		for _, period := range []string{"2025", "2026-09"} {
			cell := reportCell{Text: "—"}
			for _, metric := range r.BankMetrics {
				if metric.EntityID == measure.id && metric.Period == period {
					cell.Text = metricText(metric)
				}
			}
			row = append(row, cell)
		}
		compare.Rows = append(compare.Rows, append(row, reportCell{Text: "Bank only"}))
	}
	observationValue := func(key, period, basis, unit string, scale float64) string {
		item, ok, _, _ := bankObservation(key, period, basis, r.bankObservations, r.bankConflicts)
		if !ok || item.Unit != unit {
			return "—"
		}
		switch unit {
		case "%":
			return fmt.Sprintf("%.2f%%", *item.Value)
		case "years":
			return fmt.Sprintf("%.0f years", *item.Value)
		default:
			return fmt.Sprintf("%.2fT", *item.Value*scale)
		}
	}
	for _, measure := range []struct{ key, label, unit string }{
		{"Common Equity Tier 1 capital", "Common Equity Tier 1 (CET1)", "IDR million"},
		{"Risk-weighted assets", "Risk-weighted assets", "IDR million"},
		{"CET1 ratio", "CET1 ratio", "%"},
		{"CET1 available after minimum capital", "CET1 available after minimum capital", "%"},
		{"CET1 buffer requirements", "Bank-specific CET1 buffer requirements", "%"},
		{"Maximum EVE reduction relative to Tier 1", "Maximum EVE reduction / Tier 1 capital", "%"},
		{"Maximum NII reduction relative to projected income", "Maximum NII reduction / projected income", "%"},
	} {
		compare.Rows = append(compare.Rows, textCells(measure.label,
			observationValue(measure.key, "2025", "consolidated", measure.unit, 1e-6),
			observationValue(measure.key, "2026-09", "consolidated", measure.unit, 1e-6), "Group"))
	}

	peers := reportTable{Title: "Issuer-reported CASA at September 2026", Headers: []string{"Bank", "Share", "Reported scope"},
		NumericColumns: []int{1}}
	for _, item := range r.bankObservations {
		if item.Key != "Reported CASA share" || item.Period != "2026-09" || item.Unit != "%" {
			continue
		}
		peers.Rows = append(peers.Rows, []reportCell{{Text: item.Subject, Links: r.sourceLinks(item.Source)},
			{Text: fmt.Sprintf("%.1f%%", *item.Value)}, {Text: item.Basis}})
	}
	return reportSection{
		ID: "yearly-bank-risk", Kind: "analysis", Title: "Bank risk and funding",
		Paragraphs: []reportCell{
			{Label: "Credit", Text: "Stage 2 indicates a significant increase in credit risk. Stage 3 measures credit-impaired loans under the accounting policy. Read both alongside the separately defined prudential NPL series."},
			{Label: "Capital", Text: "Risk-weighted assets grew faster than CET1 between December 2025 and September 2026. Disclosed regulatory buffers and the scenario’s management targets answer different questions."},
			{Label: "Rates", Text: "BCA’s end-2025 rate shocks measure the change in economic value of equity (EVE) and net interest income (NII). Each uses the denominator shown below."},
			{Label: "Funding", Text: "CASA comparisons retain each issuer’s definition. A matched series of funding costs, deposit repricing and withdrawals is still needed."},
			{Label: "Incentives", Text: "BCA’s FY2025 report discloses three-year deferred pay and conditional bonus reduction or clawback for material risk takers. Application of the policy remains a research gap."},
			{Label: "Sources", Links: r.sourceLinks("bank-loan-stages", "bank-2026-09-capital-risk", "bank-fy25-irrbb", "bank-fy25-remuneration")},
		},
		Tables: []reportTable{compare, peers},
	}
}

func (r *AnalysisReport) yearlyBankScenarios() reportSection {
	scenarios := r.BankScenarios
	values := reportTable{Title: "Valuation in rupiah per share", Headers: []string{"Case", "Dividend value", "Residual income", "Normalized earnings", "Year 1 dividend"},
		NumericColumns: []int{1, 2, 3, 4}}
	stress := reportTable{Title: "One-year stress", Headers: []string{"Case", "Credit hit", "Funding hit", "Closing CET1 / target", "CET1 gap vs target"},
		NumericColumns: []int{1, 2, 3, 4}}
	rates := reportTable{Title: "Valuation assumptions", Headers: []string{"Case", "Earnings / RWA growth", "Cost of equity / terminal growth", "Payout target", "Value at +1 pp cost"},
		NumericColumns: []int{1, 2, 3, 4}}
	for _, row := range scenarios.Results {
		item := row.Case
		values.Rows = append(values.Rows, textCells(item.Name, fmt.Sprintf("Rp%.0f", row.DividendValue),
			fmt.Sprintf("Rp%.0f", row.ResidualValue), fmt.Sprintf("%.2fT", row.NormalizedIncome/1e6), fmt.Sprintf("Rp%.0f", row.Forecast[0].DividendPerShare)))
		stress.Rows = append(stress.Rows, textCells(item.Name, fmt.Sprintf("%.2fT", row.CreditLoss/1e6),
			fmt.Sprintf("%.2fT", row.FundingCost/1e6), fmt.Sprintf("%.2f%% / %.0f%%", 100*row.StressedCET1/row.StressedRWA, 100*item.CET1Target),
			fmt.Sprintf("%+.2fT", row.CET1Headroom/1e6)))
		rates.Rows = append(rates.Rows, textCells(item.Name, fmt.Sprintf("%.1f%% / %.1f%%", 100*item.EarningsGrowth, 100*item.RWAGrowth),
			fmt.Sprintf("%.1f%% / %.1f%%", 100*item.CostOfEquity, 100*item.TerminalGrowth), fmt.Sprintf("%.0f%%", 100*item.Payout), fmt.Sprintf("Rp%.0f", row.DividendValueHigherRate)))
	}
	var sourceIDs []string
	seen := map[string]bool{}
	for _, item := range scenarios.Inputs {
		if !seen[item.Source] {
			sourceIDs = append(sourceIDs, item.Source)
			seen[item.Source] = true
		}
	}
	return reportSection{
		ID: "yearly-bank-scenarios", Kind: "analysis", Title: "Bank stress and valuation",
		Paragraphs: []reportCell{
			{Label: "Starting point", Text: "Consolidated filing balances at " + scenarios.Period + ". ROE, growth, loss severity, funding stress and capital targets are illustrative inputs. T means IDR trillion."},
			{Label: "Capital", Text: "The one-year stress adds credit and funding costs beyond normalized earnings, then applies the assumed tax rate and stressed RWA. Closing CET1 includes modeled distributions and the case’s regulatory capital adjustments. A CET1 surplus shows additional after-tax loss capacity at fixed stressed RWA and payout."},
			{Label: "Valuation", Text: "Both methods share the five-year earnings and distributions. The dividend model caps terminal payouts by modeled capital needs and payout policy. Residual income uses common book equity and assumes terminal residual income grows at the stated rate."},
			{Label: "Sources", Links: r.sourceLinks(sourceIDs...)},
		},
		Tables: []reportTable{rates, stress, values},
	}
}
