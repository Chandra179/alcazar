package financials

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type bankScenarioConfig struct {
	Period string             `yaml:"period" json:"period"`
	Cases  []bankScenarioCase `yaml:"cases" json:"cases"`
}

type bankScenarioCase struct {
	Name           string                `yaml:"name" json:"name"`
	NormalizedROE  float64               `yaml:"normalized_roe" json:"normalized_roe"`
	EarningsGrowth float64               `yaml:"earnings_growth" json:"earnings_growth"`
	RWAGrowth      float64               `yaml:"rwa_growth" json:"rwa_growth"`
	TerminalGrowth float64               `yaml:"terminal_growth" json:"terminal_growth"`
	CostOfEquity   float64               `yaml:"cost_of_equity" json:"cost_of_equity"`
	Payout         float64               `yaml:"payout" json:"payout"`
	CET1Target     float64               `yaml:"cet1_target" json:"cet1_target"`
	Stress         bankStressAssumptions `yaml:"one_year_stress" json:"one_year_stress"`
}

type bankStressAssumptions struct {
	AffectedLoans  float64 `yaml:"affected_loan_share" json:"affected_loan_share"`
	LossSeverity   float64 `yaml:"incremental_loss_severity" json:"incremental_loss_severity"`
	RateShock      float64 `yaml:"policy_rate_increase" json:"policy_rate_increase"`
	DepositBeta    float64 `yaml:"deposit_beta" json:"deposit_beta"`
	Repricing      float64 `yaml:"deposit_repricing_share" json:"deposit_repricing_share"`
	Runoff         float64 `yaml:"deposit_runoff_share" json:"deposit_runoff_share"`
	Replacement    float64 `yaml:"replacement_funding_spread" json:"replacement_funding_spread"`
	RWAIncrease    float64 `yaml:"rwa_increase" json:"rwa_increase"`
	CET1Adjustment float64 `yaml:"cet1_adjustment_idr_million" json:"cet1_adjustment_idr_million"`
	TaxRate        float64 `yaml:"tax_rate" json:"tax_rate"`
}

type bankScenarioResult struct {
	Case                    bankScenarioCase    `json:"assumptions"`
	NormalizedIncome        float64             `json:"normalized_common_earnings_idr_million"`
	DividendValue           float64             `json:"dividend_value_per_share"`
	DividendValueHigherRate float64             `json:"dividend_value_at_cost_plus_1pp"`
	ResidualValue           float64             `json:"residual_income_value_per_share"`
	Forecast                []bankValuationYear `json:"forecast"`
	CreditLoss              float64             `json:"incremental_credit_loss_idr_million"`
	FundingCost             float64             `json:"incremental_funding_cost_idr_million"`
	StressedIncome          float64             `json:"stressed_common_earnings_idr_million"`
	StressedCET1            float64             `json:"stressed_cet1_idr_million"`
	StressedRWA             float64             `json:"stressed_rwa_idr_million"`
	StressedPayout          float64             `json:"stressed_distribution_idr_million"`
	CET1Headroom            float64             `json:"cet1_headroom_idr_million"`
}

type bankScenarioAnalysis struct {
	Period  string                `json:"period"`
	Inputs  []analysisObservation `json:"inputs"`
	Results []bankScenarioResult  `json:"results"`
}

func readBankScenarios(path string) (bankScenarioConfig, error) {
	var config bankScenarioConfig
	file, err := os.Open(path)
	if err != nil {
		return config, err
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return config, fmt.Errorf("parse bank scenarios: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return config, fmt.Errorf("bank scenarios must contain one YAML document")
	}
	if _, _, ok := expectedBankPeriod(config.Period); !ok || len(config.Cases) == 0 {
		return config, fmt.Errorf("bank scenarios require a reporting period and at least one case")
	}
	names := map[string]bool{}
	for _, item := range config.Cases {
		stress := item.Stress
		if strings.TrimSpace(item.Name) == "" || names[item.Name] ||
			!bankRatesValid(item.NormalizedROE, item.EarningsGrowth, item.RWAGrowth, item.TerminalGrowth,
				item.CostOfEquity, item.Payout, item.CET1Target, stress.AffectedLoans, stress.LossSeverity,
				stress.RateShock, stress.DepositBeta, stress.Repricing, stress.Runoff, stress.Replacement, stress.RWAIncrease, stress.TaxRate) ||
			math.IsNaN(stress.CET1Adjustment) || math.IsInf(stress.CET1Adjustment, 0) || item.NormalizedROE == 0 || item.CET1Target == 0 || item.CostOfEquity <= item.TerminalGrowth {
			return config, fmt.Errorf("invalid or duplicate bank scenario %q", item.Name)
		}
		names[item.Name] = true
	}
	return config, nil
}

func bankRatesValid(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || value < 0 || value > 1 {
			return false
		}
	}
	return true
}

func analyzeBankScenarios(config bankScenarioConfig, evidence bankAnalysisEvidence) (*bankScenarioAnalysis, error) {
	result := &bankScenarioAnalysis{Period: config.Period}
	values := map[string]float64{}
	for _, key := range []string{"Common Equity Tier 1 capital", "Risk-weighted assets", "Common equity attributable to parent", "Closing common shares ex treasury", "Gross loans including group financing", "Customer deposits"} {
		item, ok, reason, _ := bankObservation(key, config.Period, "consolidated", evidence.Observations, evidence.InputConflicts)
		unit := "IDR million"
		if key == "Closing common shares ex treasury" {
			unit = "shares"
		}
		if reason != "" || !ok || item.PeriodType != "balance" || item.Unit != unit || *item.Value <= 0 {
			return nil, fmt.Errorf("bank scenario needs a positive, unconflicted consolidated %s balance in %s for %s", key, unit, config.Period)
		}
		result.Inputs = append(result.Inputs, item)
		values[key] = *item.Value
	}
	inputs := bankValuationInputs{
		CommonEquity: values["Common equity attributable to parent"], CET1: values["Common Equity Tier 1 capital"],
		RWA: values["Risk-weighted assets"], Shares: values["Closing common shares ex treasury"],
	}
	for _, item := range config.Cases {
		result.Results = append(result.Results, evaluateBankScenario(item, inputs, values["Gross loans including group financing"], values["Customer deposits"]))
	}
	return result, nil
}

func evaluateBankScenario(item bankScenarioCase, inputs bankValuationInputs, loans, deposits float64) bankScenarioResult {
	model := bankValuationScenario{
		ROE: item.NormalizedROE, Growth: item.EarningsGrowth, RWAGrowth: &item.RWAGrowth,
		Terminal: item.TerminalGrowth, Discount: item.CostOfEquity, Payout: item.Payout, CapitalTarget: item.CET1Target,
	}
	dividendValue, forecast, _ := bankDividendValue(inputs, model)
	model.Discount += .01
	valueAtHigherRate, _, _ := bankDividendValue(inputs, model)

	// Carry forward common book equity using the same earnings and distributions.
	book, residual := inputs.CommonEquity, inputs.CommonEquity
	for _, year := range forecast {
		residual += (year.Earnings - item.CostOfEquity*book) / math.Pow(1+item.CostOfEquity, float64(year.Year))
		book += year.Earnings - year.DividendPerShare*inputs.Shares/1e6
	}
	terminalIncome := forecast[len(forecast)-1].Earnings * (1 + item.TerminalGrowth)
	residual += (terminalIncome - item.CostOfEquity*book) / (item.CostOfEquity - item.TerminalGrowth) /
		math.Pow(1+item.CostOfEquity, float64(len(forecast)))

	stress := item.Stress
	credit := loans * stress.AffectedLoans * stress.LossSeverity
	funding := deposits * ((1-stress.Runoff)*stress.Repricing*stress.RateShock*stress.DepositBeta + stress.Runoff*stress.Replacement)
	earnings := forecast[0].Earnings - (credit+funding)*(1-stress.TaxRate)
	rwa := forecast[0].RWA * (1 + stress.RWAIncrease)
	required := rwa * item.CET1Target
	available := inputs.CET1 + earnings + stress.CET1Adjustment
	payout := min(max(0, earnings*item.Payout), max(0, available-required))
	capital := available - payout
	return bankScenarioResult{
		Case: item, NormalizedIncome: inputs.CommonEquity * item.NormalizedROE, DividendValue: dividendValue,
		DividendValueHigherRate: valueAtHigherRate, ResidualValue: residual * 1e6 / inputs.Shares, Forecast: forecast, CreditLoss: credit, FundingCost: funding,
		StressedIncome: earnings, StressedCET1: capital, StressedRWA: rwa, StressedPayout: payout, CET1Headroom: capital - required,
	}
}
