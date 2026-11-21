package financials

import (
	"math"
	"strings"
	"testing"
)

func TestYearlyStockSummarySeparatesPriceFromDividendReturn(t *testing.T) {
	close := func(year string, price float64) analysisObservation {
		return analysisObservation{Key: "Year-end close", Period: year, Subject: "BBCA", Basis: "market",
			Unit: "IDR/share", ShareBasis: analysisShareBasis, Value: &price}
	}
	ret := -13.385
	row := analysisPeriod{Period: "2025", Values: map[string]Metric{"nominal-shareholder-return": {Value: &ret}}}
	report := AnalysisReport{Observations: []analysisObservation{close("2024", 9675)}}
	if got := report.yearlyStockSummary(row, close("2025", 8075)); got != "The share price fell 16.5%. Return with dividends was -13.4%." {
		t.Fatalf("wrong 2025 market summary: %s", got)
	}
	report.Observations[0].ShareBasis = "as-filed"
	if got := report.yearlyStockSummary(row, close("2025", 8075)); !strings.Contains(got, "unavailable") || strings.Contains(got, "%") {
		t.Fatalf("used a mismatched opening price: %s", got)
	}
	if got := report.yearlyStockSummary(analysisPeriod{Period: "2000"}, close("2000", 41.875)); !strings.Contains(got, "Rp41.875") || !strings.Contains(got, "no full-calendar return") {
		t.Fatalf("wrong listing-year summary: %s", got)
	}
	report.Observations = []analysisObservation{close("2011", 1600)}
	if got := report.yearlyStockSummary(analysisPeriod{Period: "2012"}, close("2012", 1820)); !strings.Contains(got, "rose 13.8%") {
		t.Fatalf("rounded the 2012 price change incorrectly: %s", got)
	}
}

func TestAnalysisMarketResultsUseMatchingInputs(t *testing.T) {
	eps := 466.8529777572306
	weightedShares := 123244982342.0
	fy25 := analysisPeriod{
		Period: "2025",
		Values: map[string]Metric{"earnings-per-share": {Value: &eps, Status: "calculated", Inputs: map[string]any{
			"weighted_average_common_shares_outstanding": map[string]any{"value": weightedShares, "unit": "shares"},
		}}},
		Facts: map[string]Fact{"common_shareholders_equity": {NormalizedValue: "281466478", Unit: "millions"}},
	}
	point := func(key, period, basis, unit string, value float64) analysisObservation {
		return analysisObservation{Key: key, Period: period, Basis: basis, Unit: unit, Value: &value, Subject: "BBCA", ShareBasis: analysisShareBasis}
	}
	items := []analysisObservation{
		point("Year-end close", "2024", "market", "IDR/share", 9675),
		point("Year-end close", "2025", "market", "IDR/share", 8075),
		point("Cash dividends paid", "2025", "market", "IDR/share", 305),
		point("Fiscal dividends", "2025", "market", "IDR/share", 336),
		point("CPI growth", "2025", "Indonesia CPI", "%", 2.92),
		point("Closing common shares ex treasury", "2025", "consolidated", "shares", 123013033200),
		point("Dividends earned", "2025", "market", "IDR/share", 305),
		point("Split-adjusted weighted average shares", "2025", "consolidated", "shares", weightedShares),
	}
	values := analysisResults(analysisPeriod{Period: "2024"}, fy25, items, nil)
	for id, want := range map[string]float64{
		"price-earnings-ratio": 17.2967, "price-to-book-ratio": 3.5291,
		"dividend-yield": 3.7771, "fiscal-dividend-payout": 71.9713,
		"real-return-after-inflation": -15.8424,
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
	for _, id := range []string{"price-to-book-ratio", "real-return-after-inflation", "fiscal-dividend-payout"} {
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

func TestBankDividendValueFundsRequiredCapitalFirst(t *testing.T) {
	inputs := bankValuationInputs{CommonEquity: 100, CET1: 20, RWA: 100, Shares: 1e6}
	scenario := bankValuationScenario{ROE: .10, CapitalTarget: .30, Payout: .50, Discount: .10}
	value, forecast, terminalDPS := bankDividendValue(inputs, scenario)
	if forecast[0].DividendPerShare != 0 || forecast[0].ClosingCET1 != 30 || forecast[1].DividendPerShare != 5 {
		t.Fatalf("dividend paid before funding the capital target: %+v", forecast)
	}
	if math.Abs(value-(50.0/1.1)) > 1e-9 || terminalDPS != 5 {
		t.Fatalf("value = %f, terminal DPS = %f", value, terminalDPS)
	}

	scenario.Terminal = .08
	_, _, terminalDPS = bankDividendValue(inputs, scenario)
	if math.Abs(terminalDPS-5.4) > 1e-9 {
		t.Fatalf("terminal DPS = %f", terminalDPS)
	}
	scenario.CapitalTarget = 1
	_, _, terminalDPS = bankDividendValue(inputs, scenario)
	if math.Abs(terminalDPS-2.8) > 1e-9 {
		t.Fatalf("terminal DPS after capital reinvestment = %f", terminalDPS)
	}
}

func TestAnalysisRejectsChangedCalculation(t *testing.T) {
	entry := CatalogEntry{ID: "return-on-assets", Type: "formula", Formula: "(net_earnings / average_total_assets) * 100"}
	if !formulaMatches(entry) {
		t.Fatal("reviewed formula did not match")
	}
	entry.Formula = "(net_earnings / average_total_assets) * 10"
	if formulaMatches(entry) {
		t.Fatal("changed formula matched the registered calculation")
	}
}

func TestAnalysisFingerprintCoversReviewedFacts(t *testing.T) {
	facts := []Fact{{ID: "earnings", NormalizedValue: "42", SourceLocator: "page:1"}}
	original, err := approvedFactsHash(facts)
	if err != nil {
		t.Fatal(err)
	}
	for _, updated := range []Fact{
		{ID: "earnings", NormalizedValue: "43", SourceLocator: "page:1"},
		{ID: "earnings", NormalizedValue: "42", SourceLocator: "page:2"},
	} {
		hash, err := approvedFactsHash([]Fact{updated})
		if err != nil || hash == original {
			t.Fatalf("updated fact kept the same fingerprint: %s, %v", hash, err)
		}
	}
}

func TestGroupedAnalysisCandidatesPreserveScopeAndSources(t *testing.T) {
	base := Fact{CanonicalKey: "total_assets", Period: "2025", PeriodType: "instant",
		PeriodEnd: "2025-12-31", ReportingBasis: "consolidated", AuditedStatus: "audited",
		NormalizedValue: "1500", Currency: "IDR", Unit: "millions", Status: "approved", DocumentPath: "audited.pdf"}
	corroborating := base
	corroborating.NormalizedValue = "1.5"
	corroborating.Unit = "billions"
	corroborating.DocumentPath = "annual.pdf"
	corroborating.Status = "pending"
	conflicting := base
	conflicting.NormalizedValue = "1499"
	bankOnly := base
	bankOnly.ReportingBasis = "unconsolidated"
	restated := base
	restated.CanonicalKey = "restated_total_assets"
	groups := groupAnalysisCandidates([]Fact{base, corroborating, conflicting, bankOnly, restated})
	if len(groups) != 3 || len(groups[0].Values) != 2 {
		t.Fatalf("expected one group with variants and two separate reporting scopes, got %+v", groups)
	}
	same := groups[0].Values[0]
	if same.Value != "1500000000" || len(same.Sources) != 2 ||
		same.Sources[0].Document != "audited.pdf" || same.Sources[1].Document != "annual.pdf" ||
		same.Sources[0].Status != "approved" || same.Sources[1].Status != "pending" {
		t.Fatalf("group lost source provenance: %+v", same)
	}
}

func TestBankMarginUsesComparablePublishedIncome(t *testing.T) {
	net, revenue := 48458.0, 96728.0
	const id = "net-profit-margin"
	row := analysisPeriod{Period: "2023", Values: map[string]Metric{id: {Status: "missing_inputs"}}}
	obs := func(key string, value *float64, source string) analysisObservation {
		return analysisObservation{Key: key, Period: row.Period, Basis: "consolidated", Unit: "IDR billion",
			Subject: "BBCA", Value: value, Source: source}
	}
	report := AnalysisReport{Annual: []analysisPeriod{row}, Observations: []analysisObservation{
		obs("Net income (operating-income presentation)", &net, "highlights"),
		obs("Reported operating income", &revenue, "different-statement"),
	}}
	addAnnualObservations(&report)
	if row.Values[id].Value != nil {
		t.Fatal("margin mixed different presentations")
	}
	report.Observations[1].Source = "highlights"
	addAnnualObservations(&report)
	got := row.Values[id].Value
	if got == nil || math.Abs(*got-net/revenue*100) > 1e-10 {
		t.Fatalf("published banking margin = %v", got)
	}
}

func TestHistoricalMarketRatiosRequireMatchingSplitBasis(t *testing.T) {
	period := "2003"
	row := analysisPeriod{Period: period}
	price, eps := 166.25, 19.75
	report := &AnalysisReport{Observations: []analysisObservation{
		{Key: "Year-end close", Period: period, Subject: "BBCA", Basis: "market", Unit: "IDR/share", Value: &price, ShareBasis: analysisShareBasis},
		{Key: "Basic EPS on 2025 share basis", Period: period, Subject: "BBCA", Basis: "consolidated", Unit: "IDR/share", Value: &eps, ShareBasis: "as-filed"},
	}}
	if got := annualMarketMetrics(report, row, "2002")["price-earnings-ratio"]; got.Value != nil {
		t.Fatalf("P/E combined inconsistent share bases: %+v", got)
	}
	report.Observations[1].ShareBasis = analysisShareBasis
	if got := annualMarketMetrics(report, row, "2002")["price-earnings-ratio"]; got.Value == nil || *got.Value != price/eps {
		t.Fatalf("historical P/E failed with matching bases: %+v", got)
	}
}

func TestListingYearHasNoFullCalendarReturn(t *testing.T) {
	const period = "2000"
	point := func(key, year string, value float64) analysisObservation {
		return analysisObservation{Key: key, Period: year, Subject: "BBCA", Basis: "market", Unit: "IDR/share",
			ShareBasis: analysisShareBasis, Value: &value}
	}
	report := AnalysisReport{Observations: []analysisObservation{
		point("Year-end close", "1999", 40), point("Year-end close", period, 41.875),
		point("Cash dividends paid", period, 0), point("Dividends earned", period, 0),
	}}
	metrics := annualMarketMetrics(&report, analysisPeriod{Period: period}, "1999")
	if yield := metrics["dividend-yield"]; yield.Value == nil || *yield.Value != 0 {
		t.Fatalf("expected zero listing-year cash yield: %+v", yield)
	}
	for _, key := range []string{"nominal-shareholder-return", "real-return-after-inflation"} {
		if got := metrics[key]; got.Value != nil || got.Status != "not_applicable" {
			t.Errorf("%s must be inapplicable in the listing year: %+v", key, got)
		}
	}
}

func TestAnnualMarketRatiosPreferExactEPSOnMatchedShareBasis(t *testing.T) {
	const period = "2020"
	price, roundedEPS, earnings, weightedShares := 6770.0, 220.0, 27131109e6, 123275050000.0
	shares := analysisObservation{Key: "Split-adjusted weighted average shares", Period: period, Basis: "consolidated", Unit: "shares",
		Value: &weightedShares, Subject: "BBCA", ShareBasis: analysisShareBasis}
	report := &AnalysisReport{Observations: []analysisObservation{
		{Key: "Year-end close", Period: period, Basis: "market", Unit: "IDR/share", Value: &price, Subject: "BBCA", ShareBasis: analysisShareBasis},
		{Key: "Basic EPS on 2025 share basis", Period: period, Basis: "consolidated", Unit: "IDR/share", Value: &roundedEPS,
			Subject: "BBCA", ShareBasis: analysisShareBasis},
		shares,
	}}
	row := analysisPeriod{Period: period, Values: map[string]Metric{
		"earnings-per-share": derived("earnings-per-share", period, "currency per share", earnings/weightedShares,
			map[string]any{"split_adjusted_weighted_average_shares": shares}),
	}}
	if got := annualMarketMetrics(report, row, "2019")["price-earnings-ratio"]; got.Value == nil || *got.Value != price/(earnings/weightedShares) {
		t.Fatalf("expected exact split-adjusted EPS: %+v", got)
	}
	preSplitShares := weightedShares / 5
	shares.Value, shares.ShareBasis = &preSplitShares, "as-filed"
	row.Values["earnings-per-share"] = derived("earnings-per-share", period, "currency per share", earnings/preSplitShares,
		map[string]any{"split_adjusted_weighted_average_shares": shares})
	if got := annualMarketMetrics(report, row, "2019")["price-earnings-ratio"]; got.Value == nil || *got.Value != price/roundedEPS {
		t.Fatalf("expected normalized published EPS when calculated EPS has a different basis: %+v", got)
	}
}

func TestFlowMetricsRejectDifferentStatementSpans(t *testing.T) {
	pbt := Fact{Period: "2026-10", PeriodStart: "2026-01-01", PeriodEnd: "2026-10-31", PeriodType: "interim",
		ReportingBasis: "unconsolidated", AuditedStatus: "unaudited", Currency: "IDR", Unit: "millions"}
	tax := pbt
	tax.PeriodStart = "2026-10-01"
	facts := map[string]periodFact{"income_tax_expense": {Fact: tax}, "income_before_tax": {Fact: pbt}}
	if reason := metricInputCompatibility(facts, []string{"income_tax_expense", "income_before_tax"}, "effective-tax-rate"); reason == "" {
		t.Fatal("tax rate accepted an October-only expense against year-to-date income")
	}
}
