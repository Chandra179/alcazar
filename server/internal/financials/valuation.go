package financials

import (
	"fmt"
	"math"
	"slices"
)

type bankValuationInputs struct {
	CommonEquity, CET1, RWA, Shares float64
}

type bankValuationScenario struct {
	Name                            string
	ROE, Growth, Terminal, Discount float64
	Payout, CapitalTarget, Weight   float64
}

type bankValuationYear struct {
	Year             int     `json:"year"`
	Earnings         float64 `json:"common_earnings_idr_million"`
	RWA              float64 `json:"rwa_idr_million"`
	RequiredCET1     float64 `json:"required_cet1_idr_million"`
	ClosingCET1      float64 `json:"closing_cet1_idr_million"`
	DividendPerShare float64 `json:"dividend_idr_per_share"`
}

func bankDividendValue(inputs bankValuationInputs, scenario bankValuationScenario) (float64, []bankValuationYear, float64) {
	earnings, rwa, capital := inputs.CommonEquity*scenario.ROE, inputs.RWA, inputs.CET1
	var forecast []bankValuationYear
	value := 0.0
	for year := 1; year <= 5; year++ {
		earnings *= 1 + scenario.Growth
		rwa *= 1 + scenario.Growth
		required := rwa * scenario.CapitalTarget
		dividend := min(earnings*scenario.Payout, max(0, capital+earnings-required))
		capital += earnings - dividend
		dps := dividend * 1e6 / inputs.Shares
		value += dps / math.Pow(1+scenario.Discount, float64(year))
		forecast = append(forecast, bankValuationYear{year, earnings, rwa, required, capital, dps})
	}
	// In steady state, earnings must first replenish CET1 consumed by RWA growth.
	// Account for any remaining shortfall at the end of year five.
	terminalEarnings := earnings * (1 + scenario.Terminal)
	retention := scenario.CapitalTarget * rwa * scenario.Terminal
	terminalDividend := min(terminalEarnings*scenario.Payout, max(0, terminalEarnings-retention))
	shortfall := max(0, scenario.CapitalTarget*rwa-capital)
	// A shortfall reduces the terminal stream's value by the amount owners would
	// need to fund at the transition. Excess capital is not valued separately.
	terminalDPS := terminalDividend * 1e6 / inputs.Shares
	terminalValue := max(0, terminalDPS/(scenario.Discount-scenario.Terminal)-shortfall*1e6/inputs.Shares)
	return value + terminalValue/math.Pow(1+scenario.Discount, 5), forecast, terminalDPS
}

func addBankValuation(report *AnalysisReport) {
	const period = "2025"
	results := report.Results
	reason := "Requires five approved annual ROEs, positive FY2025 common equity and shares, and comparable consolidated CET1 and RWA disclosures."
	for _, id := range []string{"dividend-value-conservative", "dividend-value-base", "dividend-value-favorable", "dividend-value-weighted", "margin-of-safety"} {
		results[id] = Metric{EntityID: id, Period: period, Status: "missing_inputs", Reason: reason}
	}
	var roe []float64
	var historical []Metric
	var equity Fact
	for _, row := range report.Annual {
		if row.Period < "2021" || row.Period > period {
			continue
		}
		metric := row.Values["return-on-equity"]
		if metric.Value == nil || *metric.Value <= 0 {
			return
		}
		roe = append(roe, *metric.Value/100)
		historical = append(historical, metric)
		if row.Period == period {
			equity = row.Facts["common_shareholders_equity"]
		}
	}
	if len(roe) != 5 {
		return
	}
	slices.Sort(roe)
	commonEquity, equityOK := parseCanonicalNumber(equity.NormalizedValue)
	scale, scaleOK := unitScale(equity.Unit)
	cet1 := report.findObservation("Common Equity Tier 1 capital", "BBCA", period, "consolidated", "IDR million")
	shares := report.findObservation("Closing common shares ex treasury", "BBCA", period, "consolidated", "shares")
	rwa := make([]analysisObservation, 0, 3)
	for _, year := range []string{"2023", "2024", "2025"} {
		observation := report.findObservation("Risk-weighted assets", "BBCA", year, "consolidated", "IDR million")
		if observation.Value == nil || *observation.Value <= 0 {
			return
		}
		rwa = append(rwa, observation)
	}
	if !equityOK || !scaleOK || commonEquity <= 0 || equity.Currency != "IDR" ||
		cet1.Value == nil || *cet1.Value <= 0 || shares.Value == nil || *shares.Value <= 0 {
		return
	}
	inputs := bankValuationInputs{commonEquity * scale / 1e6, *cet1.Value, *rwa[2].Value, *shares.Value}
	firstGrowth := *rwa[1].Value / *rwa[0].Value - 1
	lastGrowth := *rwa[2].Value / *rwa[1].Value - 1
	scenarios := []bankValuationScenario{
		{"conservative", roe[0], min(firstGrowth, lastGrowth), .02, .12, .60, .29, .25},
		{"base", roe[2], (firstGrowth + lastGrowth) / 2, .03, .11, .65, .28, .50},
		{"favorable", roe[4], max(firstGrowth, lastGrowth), .04, .10, .70, .27, .25},
	}
	weighted := 0.0
	var values []Metric
	for _, scenario := range scenarios {
		value, forecast, terminalDPS := bankDividendValue(inputs, scenario)
		id := "dividend-value-" + scenario.Name
		results[id] = derived(id, period, "IDR/share", value, map[string]any{
			"common_equity": equity, "closing_cet1": cet1, "historical_roe": historical,
			"historical_rwa": rwa, "closing_shares": shares, "normalized_roe": scenario.ROE,
			"normalized_common_earnings_idr_million": inputs.CommonEquity * scenario.ROE,
			"growth":                                 scenario.Growth, "terminal_growth": scenario.Terminal, "cost_of_equity": scenario.Discount,
			"payout_target": scenario.Payout, "cet1_target": scenario.CapitalTarget, "scenario_weight": scenario.Weight,
			"forecast": forecast, "terminal_dividend_per_share": terminalDPS,
		})
		values = append(values, results[id])
		weighted += value * scenario.Weight
	}
	results["dividend-value-weighted"] = derived("dividend-value-weighted", period, "IDR/share", weighted,
		map[string]any{"weights": []float64{.25, .50, .25}, "scenarios": []string{values[0].EntityID, values[1].EntityID, values[2].EntityID}})
	close := report.findObservation("Year-end close", "BBCA", period, "market", "IDR/share")
	conservative := values[0]
	results["margin-of-safety"] = Metric{EntityID: "margin-of-safety", Period: period, Status: "missing_inputs",
		Reason: "Requires a positive year-end close and conservative scenario value."}
	if close.Value != nil && *close.Value > 0 && *conservative.Value > 0 {
		results["margin-of-safety"] = derived("margin-of-safety", period, "%", (1 - *close.Value / *conservative.Value)*100,
			map[string]any{"close": close, "conservative_value": conservative})
	}
	base := scenarios[1]
	for _, rate := range []float64{.10, .11, .12} {
		for _, growth := range []float64{.02, .03, .04} {
			base.Discount, base.Terminal = rate, growth
			value, _, _ := bankDividendValue(inputs, base)
			id := fmt.Sprintf("sensitivity-%.0f-%.0f", rate*100, growth*100)
			results[id] = derived(id, period, "IDR/share", value, map[string]any{
				"base_scenario": "dividend-value-base", "cost_of_equity": rate, "terminal_growth": growth,
			})
		}
	}
}
