package financials

import (
	"math"
	"testing"
)

func TestAnalysisMarketResultsUseMatchingInputs(t *testing.T) {
	eps := 466.8529777572306
	fy25 := analysisPeriod{
		Period: "2025",
		Values: map[string]Metric{"earnings-per-share": {Value: &eps, Status: "calculated"}},
		Facts:  map[string]Fact{"common_shareholders_equity": {NormalizedValue: "281466478", Unit: "millions"}},
	}
	point := func(key, period, basis, unit string, value float64) analysisObservation {
		return analysisObservation{Key: key, Period: period, Basis: basis, Unit: unit, Value: &value, Subject: "BBCA"}
	}
	items := []analysisObservation{
		point("Year-end close", "2024", "market", "IDR/share", 9675),
		point("Year-end close", "2025", "market", "IDR/share", 8075),
		point("Cash dividends paid", "2025", "market", "IDR/share", 305),
		point("Fiscal dividends", "2025", "market", "IDR/share", 336),
		point("CPI growth", "2025", "Indonesia CPI", "%", 2.92),
		point("Closing common shares ex treasury", "2025", "consolidated", "shares", 123013033200),
	}
	values := analysisResults(analysisPeriod{Period: "2024"}, fy25, items, nil)
	for id, want := range map[string]float64{
		"price-earnings-ratio": 17.2967, "price-to-book-ratio": 3.5291,
		"dividend-yield": 3.7771, "fiscal-dividend-payout": 71.9713,
		"real-return-after-inflation": -15.8424, "dividend-value-base": 4901.74,
	} {
		got := values[id]
		if got.Status != "calculated" || got.Value == nil || math.Abs(*got.Value-want) > 0.01 {
			t.Errorf("%s = %#v, want %.4f", id, got, want)
		}
	}

	items[5].Basis = "unconsolidated"
	items[4].Period = "2024"
	items[3].Period = "2024"
	values = analysisResults(analysisPeriod{Period: "2024"}, fy25, items, nil)
	for _, id := range []string{"price-to-book-ratio", "real-return-after-inflation", "dividend-value-base", "margin-of-safety"} {
		got := values[id]
		if got.Status != "missing_inputs" || got.Value != nil || got.Reason == "" {
			t.Errorf("%s accepted mismatched inputs: %#v", id, got)
		}
	}
}

func TestAnalysisPeerComparisonDoesNotMixReportingBases(t *testing.T) {
	v := 2.0
	items := []analysisObservation{
		{Key: "Gross NPL", Period: "2025", Basis: "bank only", Unit: "%", Subject: "BBCA", Value: &v},
		{Key: "Gross NPL", Period: "2025", Basis: "bank only", Unit: "%", Subject: "BBRI", Value: &v},
		{Key: "Gross NPL", Period: "2025", Basis: "consolidated", Unit: "%", Subject: "BMRI", Value: &v},
	}
	row := analysisPeers(items)[0]
	if row.Status == "comparable" || len(row.Observations) != 2 {
		t.Fatalf("basis mismatch entered comparison: %#v", row)
	}
	items[2].Basis = "bank only"
	row = analysisPeers(items)[0]
	if row.Status != "comparable" || len(row.Observations) != 3 {
		t.Fatalf("matching observations were excluded: %#v", row)
	}
}
