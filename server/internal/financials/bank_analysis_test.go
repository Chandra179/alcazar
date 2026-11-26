package financials

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCatalogOverlayDependencies(t *testing.T) {
	t.Parallel()
	base := []CatalogEntry{{ID: "shared-rule", Type: "heuristic", Description: "Shared"}}
	valid := `entries:
  - id: bank-rule-a
    type: heuristic
    description: Bank rule
    depends_on: [shared-rule, bank-rule-b]
  - id: bank-rule-b
    type: concept
    description: Related bank rule
`
	path := filepath.Join(t.TempDir(), "bank.yaml")
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, hash, err := LoadCatalogOverlay(path, base)
	if err != nil {
		t.Fatalf("LoadCatalogOverlay accepted dependencies: %v", err)
	}
	if len(entries) != 2 || hash == "" {
		t.Fatalf("loaded overlay = %d entries, hash %q", len(entries), hash)
	}

	for name, catalog := range map[string]string{
		"duplicate": `entries:
  - id: shared-rule
    type: heuristic
    description: Duplicate
`,
		"missing dependency": `entries:
  - id: bank-rule-a
    type: heuristic
    description: Missing dependency
    depends_on: [unknown-rule]
`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(catalog), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := LoadCatalogOverlay(path, base); err == nil {
				t.Fatalf("LoadCatalogOverlay accepted %s", name)
			}
		})
	}
}

func TestBankCatalogPinsAllRegisteredFormulas(t *testing.T) {
	t.Parallel()
	entries := make([]CatalogEntry, 0, len(bankFormulaDefinitions))
	for _, definition := range bankFormulaDefinitions {
		entries = append(entries, CatalogEntry{ID: definition.id, Type: "formula", Formula: definition.formula})
	}
	if err := validateBankFormulaCatalog(entries); err != nil {
		t.Fatalf("valid registered formulas rejected: %v", err)
	}
	if err := validateBankFormulaCatalog(entries[1:]); err == nil || !strings.Contains(err.Error(), bankCreditCostRule) {
		t.Fatalf("missing formula error = %v", err)
	}
	entries[0].Formula = "different calculation"
	if err := validateBankFormulaCatalog(entries); err == nil {
		t.Fatal("changed formula accepted")
	}
}

func TestBankMetricRatiosUseMatchingPeriodAndScope(t *testing.T) {
	observations := bankMetricFixture()
	metrics := calculateBankMetrics(observations, nil)
	want := map[string]struct {
		value  float64
		status string
	}{
		bankCreditCostRule + "/unconsolidated":  {1, "calculated"},
		bankNPLCoverageRule + "/unconsolidated": {200, "proxy_calculated"},
		bankDepositCostRule + "/consolidated":   {2, "calculated"},
	}
	for _, metric := range metrics {
		key := metric.EntityID + "/" + metric.Basis
		if expected, ok := want[key]; ok {
			if metric.Status != expected.status || metric.Value == nil || *metric.Value != expected.value {
				t.Errorf("%s result = %#v; want %s %v", key, metric, expected.status, expected.value)
			}
			delete(want, key)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing metrics: %v", want)
	}
}

func TestBankMetricGapsAndConflictsStayExplicit(t *testing.T) {
	t.Run("missing matching close date", func(t *testing.T) {
		observations := bankMetricFixture()
		observations = removeBankObservation(observations, "Gross loans and financing", "2025", "unconsolidated")
		metric := findBankMetric(calculateBankMetrics(observations, nil), bankCreditCostRule, "2025", "unconsolidated")
		if metric.Status != "missing_inputs" || metric.Value != nil || !strings.Contains(metric.Reason, "same period") {
			t.Fatalf("missing balance result = %#v", metric)
		}
	})

	t.Run("mismatched units", func(t *testing.T) {
		observations := bankMetricFixture()
		for i := range observations {
			if observations[i].Key == "Loan impairment expense on loans" {
				observations[i].Unit = "IDR"
			}
		}
		metric := findBankMetric(calculateBankMetrics(observations, nil), bankCreditCostRule, "2025", "unconsolidated")
		if metric.Status != "conflict" || metric.Value != nil {
			t.Fatalf("unit mismatch result = %#v", metric)
		}
	})

	t.Run("conflicting reviewed value", func(t *testing.T) {
		observations := bankMetricFixture()
		conflicts := []bankInputConflict{{Key: "Loan impairment expense on loans", Period: "2025", Basis: "unconsolidated", Reason: "Two reviewed filing values differ.", Sources: []string{"source-a", "source-b"}}}
		metric := findBankMetric(calculateBankMetrics(observations, conflicts), bankCreditCostRule, "2025", "unconsolidated")
		if metric.Status != "conflict" || metric.Value != nil || len(metric.Inputs["conflicts"].([]bankInputConflict)) != 1 {
			t.Fatalf("conflict result = %#v", metric)
		}
	})

	t.Run("conflicts on observed opening and closing balances", func(t *testing.T) {
		const key = "Gross loans and financing"
		const basis = "unconsolidated"
		conflicts := []bankInputConflict{
			{Key: key, Period: "2024", Basis: basis, Reason: "Opening balances disagree."},
			{Key: key, Period: "2025-12-31", Basis: basis, Reason: "Closing balances disagree."},
		}
		metric := findBankMetric(calculateBankMetrics(bankMetricFixture(), conflicts), bankCreditCostRule, "2025", basis)
		reported, _ := metric.Inputs["conflicts"].([]bankInputConflict)
		if metric.Status != "conflict" || metric.Value != nil || len(reported) != 2 {
			t.Fatalf("balance conflicts = %#v", metric)
		}
	})

	t.Run("conflict without an observation in the same period and scope", func(t *testing.T) {
		const key = "Interest expense on customer deposits"
		const basis = "consolidated"
		observations := removeBankObservation(bankMetricFixture(), key, "2025", basis)
		conflicts := []bankInputConflict{{Key: key, Period: "2025", Basis: basis, Reason: "Reported expense figures disagree."}}
		metric := findBankMetric(calculateBankMetrics(observations, conflicts), bankDepositCostRule, "2025", basis)
		if metric.Status != "conflict" || metric.Value != nil || metric.Reason != conflicts[0].Reason {
			t.Fatalf("conflict-only period = %#v", metric)
		}
	})

	t.Run("zero denominator", func(t *testing.T) {
		cases := []struct {
			name, id, basis, key string
		}{
			{"credit cost", bankCreditCostRule, "unconsolidated", "Gross loans and financing"},
			{"NPL coverage", bankNPLCoverageRule, "unconsolidated", "Gross non-performing loan amount"},
			{"deposit cost", bankDepositCostRule, "consolidated", "Customer deposits"},
		}
		for _, test := range cases {
			t.Run(test.name, func(t *testing.T) {
				observations := bankMetricFixture()
				for i := range observations {
					if observations[i].Key == test.key {
						observations[i].Value = bankFloat(0)
					}
				}
				metric := findBankMetric(calculateBankMetrics(observations, nil), test.id, "2025", test.basis)
				if metric.Status != "not_calculable" || metric.Value != nil {
					t.Fatalf("zero denominator result = %#v", metric)
				}
			})
		}
	})

	t.Run("mismatched scope", func(t *testing.T) {
		observations := bankMetricFixture()
		for i := range observations {
			if observations[i].Key == "Gross loans and financing" && observations[i].Period == "2024" {
				observations[i].Basis = "consolidated"
			}
			if observations[i].Key == "Gross loans and financing" && observations[i].Period == "2025" {
				observations[i].Basis = "consolidated"
			}
		}
		metric := findBankMetric(calculateBankMetrics(observations, nil), bankCreditCostRule, "2025", "unconsolidated")
		if metric.Status != "missing_inputs" || metric.Value != nil {
			t.Fatalf("scope mismatch result = %#v", metric)
		}
	})

	t.Run("mismatched date", func(t *testing.T) {
		observations := bankMetricFixture()
		for i := range observations {
			if observations[i].Key == "Gross loans and financing" && observations[i].Period == "2025" {
				observations[i].PeriodEnd = "2025-09-30"
			}
		}
		metric := findBankMetric(calculateBankMetrics(observations, nil), bankCreditCostRule, "2025", "unconsolidated")
		if metric.Status != "missing_inputs" || metric.Value != nil {
			t.Fatalf("date mismatch result = %#v", metric)
		}
	})
}

func TestBankEvidenceRequiresOneAssessmentPerOverlayRule(t *testing.T) {
	t.Parallel()
	catalog := []CatalogEntry{{ID: "bank-one"}, {ID: "bank-two"}}
	evidence := bankAnalysisEvidence{Rules: map[string]analysisAssessment{
		"bank-one": {Status: "partial", Finding: "Reviewed", Sources: []string{"source"}},
		"bank-two": {Status: "gap", Finding: "Gap", Sources: []string{"source"}},
	}}
	if err := validateBankRuleAssessments(catalog, evidence); err != nil {
		t.Fatalf("complete bank assessments rejected: %v", err)
	}
	delete(evidence.Rules, "bank-two")
	if err := validateBankRuleAssessments(catalog, evidence); err == nil {
		t.Fatal("missing bank rule assessment accepted")
	}
}

func TestObservationPeriodsMatchExactCalendarBounds(t *testing.T) {
	annual := analysisObservation{Period: "2025", PeriodType: "annual", PeriodStart: "2025-01-01", PeriodEnd: "2025-12-31"}
	if err := validateObservationPeriod(annual); err != nil {
		t.Fatalf("annual period rejected: %v", err)
	}
	interim := analysisObservation{Period: "2026-09", PeriodType: "interim", PeriodStart: "2026-01-01", PeriodEnd: "2026-09-30"}
	if err := validateObservationPeriod(interim); err != nil {
		t.Fatalf("interim period rejected: %v", err)
	}
	interim.PeriodStart = "2026-07-01"
	if err := validateObservationPeriod(interim); err == nil {
		t.Fatal("quarter-only flow labeled as YTD accepted")
	}
}

func TestInterimBankRatioUsesYTDFlowWithoutAnnualizing(t *testing.T) {
	observations := []analysisObservation{
		bankBalance("Gross loans and financing", "2024", "unconsolidated", 1000, "2024-12-31"),
		{Key: "Loan impairment expense on loans", Subject: "BBCA", Period: "2025-06", PeriodType: "interim",
			PeriodStart: "2025-01-01", PeriodEnd: "2025-06-30", Basis: "unconsolidated", Unit: "IDR million", Value: bankFloat(20)},
		bankBalance("Gross loans and financing", "2025-06", "unconsolidated", 1000, "2025-06-30"),
	}
	metric := findBankMetric(calculateBankMetrics(observations, nil), bankCreditCostRule, "2025-06", "unconsolidated")
	if metric.Status != "calculated" || metric.Value == nil || *metric.Value != 2 {
		t.Fatalf("interim cost ratio = %#v; want 2%% period-to-date", metric)
	}
}

func TestLatestBankMetricsUsesPeriodEnd(t *testing.T) {
	metrics := []Metric{
		{EntityID: bankDepositCostRule, Period: "2026"},
		{EntityID: bankDepositCostRule, Period: "2026-09"},
		{EntityID: bankDepositCostRule, Period: "2026-12"},
		{EntityID: bankCreditCostRule, Period: "2025"},
		{EntityID: bankCreditCostRule, Period: "2026-03"},
	}
	latest := latestBankMetrics(metrics)
	if latest[bankDepositCostRule].Period != "2026" || latest[bankCreditCostRule].Period != "2026-03" {
		t.Fatalf("latest periods = %#v", latest)
	}
}

func bankMetricFixture() []analysisObservation {
	return []analysisObservation{
		bankBalance("Gross loans and financing", "2024", "unconsolidated", 1000, "2024-12-31"),
		bankFlow("Loan impairment expense on loans", "2025", "unconsolidated", 10),
		bankBalance("Gross loans and financing", "2025", "unconsolidated", 1000, "2025-12-31"),
		bankBalance("Loan loss allowance", "2025", "unconsolidated", 100, "2025-12-31"),
		bankBalance("Gross non-performing loan amount", "2025", "unconsolidated", 50, "2025-12-31"),
		bankBalance("Customer deposits", "2024", "consolidated", 1000, "2024-12-31"),
		bankFlow("Interest expense on customer deposits", "2025", "consolidated", 20),
		bankBalance("Customer deposits", "2025", "consolidated", 1000, "2025-12-31"),
	}
}

func bankBalance(key, period, basis string, value float64, date string) analysisObservation {
	return analysisObservation{Key: key, Subject: "BBCA", Period: period, PeriodType: "balance", PeriodEnd: date,
		Basis: basis, Unit: "IDR million", Value: bankFloat(value)}
}

func bankFlow(key, period, basis string, value float64) analysisObservation {
	return analysisObservation{Key: key, Subject: "BBCA", Period: period, PeriodType: "annual",
		PeriodStart: period + "-01-01", PeriodEnd: period + "-12-31", Basis: basis,
		Unit: "IDR million", Value: bankFloat(value)}
}

func bankFloat(value float64) *float64 { return &value }

func removeBankObservation(observations []analysisObservation, key, period, basis string) []analysisObservation {
	var result []analysisObservation
	for _, observation := range observations {
		if observation.Key == key && observation.Period == period && observation.Basis == basis {
			continue
		}
		result = append(result, observation)
	}
	return result
}

func findBankMetric(metrics []Metric, id, period, basis string) Metric {
	for _, metric := range metrics {
		if metric.EntityID == id && metric.Period == period && metric.Basis == basis {
			return metric
		}
	}
	return Metric{EntityID: id, Period: period, Basis: basis, Status: "missing metric"}
}
