package financials

import (
	"math"
	"testing"
)

func TestBankScenarioSeparatesCapitalGrowthAndFundingRunoff(t *testing.T) {
	inputs := bankValuationInputs{CommonEquity: 1000, CET1: 300, RWA: 1000, Shares: 1e6}
	item := bankScenarioCase{
		Name: "Example", NormalizedROE: .12, EarningsGrowth: .05, RWAGrowth: .10,
		TerminalGrowth: .02, CostOfEquity: .10, Payout: .70, CET1Target: .30,
		Stress: bankStressAssumptions{
			AffectedLoans: .10, LossSeverity: .20, RateShock: .04, DepositBeta: .50,
			Repricing: .50, Runoff: .20, Replacement: .03, RWAIncrease: .10,
			TaxRate: .25, CET1Adjustment: -10,
		},
	}
	result := evaluateBankScenario(item, inputs, 800, 1000)
	for _, want := range []struct {
		name     string
		got, val float64
	}{
		{"first year earnings", result.Forecast[0].Earnings, 126},
		{"first year RWA", result.Forecast[0].RWA, 1100},
		{"incremental credit loss", result.CreditLoss, 16},
		{"funding cost excluding withdrawn deposits from repricing", result.FundingCost, 14},
		{"stressed earnings after tax", result.StressedIncome, 103.5},
		{"stressed RWA", result.StressedRWA, 1210},
		{"closing CET1", result.StressedCET1, 363},
		{"distribution capped by CET1 target", result.StressedPayout, 30.5},
	} {
		if math.Abs(want.got-want.val) > 1e-8 {
			t.Errorf("%s = %g, want %g", want.name, want.got, want.val)
		}
	}
	if result.DividendValueHigherRate >= result.DividendValue || result.ResidualValue <= 0 {
		t.Fatalf("unexpected values %+v", result)
	}
}

func TestBankStageSharesUseMatchedGrossBalances(t *testing.T) {
	basis, period, end := "unconsolidated", "2026-09", "2026-09-30"
	observations := []analysisObservation{
		bankBalance("Gross Stage 2 loans", period, basis, 21, end),
		bankBalance("Gross Stage 3 loans", period, basis, 18, end),
		bankBalance("Gross staged loans", period, basis, 1000, end),
		bankBalance("Gross staged loans", period, "consolidated", 2000, end),
	}
	metrics := calculateBankMetrics(observations, nil)
	for id, expected := range map[string]float64{bankStage2Rule: 2.1, bankStage3Rule: 1.8} {
		metric := findBankMetric(metrics, id, period, basis)
		if metric.Status != "calculated" || metric.Value == nil || math.Abs(*metric.Value-expected) > 1e-8 {
			t.Fatalf("%s = %#v, want %g", id, metric, expected)
		}
	}
	conflicts := []bankInputConflict{{Key: "Gross staged loans", Period: period, Basis: basis, Reason: "conflicting balance"}}
	metric := findBankMetric(calculateBankMetrics(observations, conflicts), bankStage2Rule, period, basis)
	if metric.Status != "conflict" || metric.Value != nil {
		t.Fatalf("conflicted stage share = %#v", metric)
	}
}
