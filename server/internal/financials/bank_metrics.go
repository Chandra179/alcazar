package financials

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	bankCreditCostRule  = "bank-credit-cost-ratio"
	bankNPLCoverageRule = "bank-npl-coverage-ratio"
	bankDepositCostRule = "bank-deposit-cost-ratio"
	bankStage2Rule      = "bank-stage-2-loan-share"
	bankStage3Rule      = "bank-stage-3-loan-share"
)

type bankFormulaDefinition struct {
	id, formula, flowKey, currentBalanceKey string
	openingBalanceKey, unit, basis          string
	coverageKey                             string
	optional                                bool
}

var bankFormulaDefinitions = []bankFormulaDefinition{
	{
		id: bankCreditCostRule, formula: "(loan_impairment_expense / average_gross_loans) * 100",
		flowKey: "Loan impairment expense on loans", currentBalanceKey: "Gross loans and financing",
		openingBalanceKey: "Gross loans and financing", unit: "percent", basis: "unconsolidated",
	},
	{
		id: bankNPLCoverageRule, formula: "(loan_loss_allowance / gross_nonperforming_loans) * 100",
		currentBalanceKey: "Loan loss allowance", coverageKey: "Gross non-performing loan amount", unit: "percent", basis: "unconsolidated",
	},
	{
		id: bankDepositCostRule, formula: "(interest_expense_on_customer_deposits / average_customer_deposits) * 100",
		flowKey: "Interest expense on customer deposits", currentBalanceKey: "Customer deposits",
		openingBalanceKey: "Customer deposits", unit: "percent", basis: "consolidated",
	},
	{id: bankStage2Rule, formula: "(gross_stage_2_loans / gross_staged_loans) * 100", currentBalanceKey: "Gross Stage 2 loans", coverageKey: "Gross staged loans", unit: "percent", basis: "unconsolidated", optional: true},
	{id: bankStage3Rule, formula: "(gross_stage_3_loans / gross_staged_loans) * 100", currentBalanceKey: "Gross Stage 3 loans", coverageKey: "Gross staged loans", unit: "percent", basis: "unconsolidated", optional: true},
}

func bankFormulaMatches(entry CatalogEntry) bool {
	for _, definition := range bankFormulaDefinitions {
		if entry.ID == definition.id {
			return entry.Type == "formula" && normalizeFormula(entry.Formula) == normalizeFormula(definition.formula)
		}
	}
	return entry.Type != "formula"
}

func normalizeFormula(formula string) string {
	return strings.Join(strings.Fields(formula), "")
}

func calculateBankMetrics(observations []analysisObservation, conflicts []bankInputConflict) []Metric {
	periods := make(map[string]map[string]bool)
	addPeriod := func(period, basis string) {
		if periods[period] == nil {
			periods[period] = make(map[string]bool)
		}
		periods[period][basis] = true
	}
	for _, observation := range observations {
		if observation.Subject == "BBCA" && bankReportInput(observation.Key, observation.Basis, observation.PeriodType) {
			addPeriod(observation.Period, observation.Basis)
		}
	}
	for _, conflict := range conflicts {
		period, end := bankConflictPeriod(conflict.Period)
		if period == "" || !bankReportInput(conflict.Key, conflict.Basis, "") {
			continue
		}
		// A December YTD record and a full-year record end on the same date.
		if strings.HasSuffix(end, "-12-31") &&
			(periods[end[:4]][conflict.Basis] || periods[end[:7]][conflict.Basis]) {
			continue
		}
		addPeriod(period, conflict.Basis)
	}
	orderedPeriods := make([]string, 0, len(periods))
	for period := range periods {
		orderedPeriods = append(orderedPeriods, period)
	}
	sort.Strings(orderedPeriods)

	var metrics []Metric
	for _, period := range orderedPeriods {
		bases := make([]string, 0, len(periods[period]))
		for basis := range periods[period] {
			bases = append(bases, basis)
		}
		sort.Strings(bases)
		for _, basis := range bases {
			for _, definition := range bankFormulaDefinitions {
				if basis != definition.basis {
					continue
				}
				if definition.optional {
					_, observed, conflict, _ := bankObservation(definition.currentBalanceKey, period, basis, observations, conflicts)
					_, denominatorObserved, denominatorConflict, _ := bankObservation(definition.coverageKey, period, basis, observations, conflicts)
					if !observed && !denominatorObserved && conflict == "" && denominatorConflict == "" {
						continue
					}
				}
				metric := calculateBankMetric(definition, period, basis, observations, conflicts)
				metrics = append(metrics, metric)
			}
		}
	}
	return metrics
}

// Opening and closing balances for cost ratios can exist solely as supporting inputs.
func bankReportInput(key, basis, periodType string) bool {
	for _, definition := range bankFormulaDefinitions {
		if basis != definition.basis {
			continue
		}
		if definition.flowKey != "" {
			if key == definition.flowKey && periodType != "balance" {
				return true
			}
		} else if (key == definition.currentBalanceKey || key == definition.coverageKey) &&
			(periodType == "" || periodType == "balance") {
			return true
		}
	}
	return false
}

func latestBankMetrics(metrics []Metric) map[string]Metric {
	latest := make(map[string]Metric)
	for _, metric := range metrics {
		_, end, _ := expectedBankPeriod(metric.Period)
		prior, exists := latest[metric.EntityID]
		_, priorEnd, _ := expectedBankPeriod(prior.Period)
		if !exists || end > priorEnd || (end == priorEnd && len(metric.Period) < len(prior.Period)) {
			latest[metric.EntityID] = metric
		}
	}
	return latest
}

func calculateBankMetric(definition bankFormulaDefinition, period, basis string, observations []analysisObservation, conflicts []bankInputConflict) Metric {
	metric := Metric{EntityID: definition.id, Period: period, Unit: definition.unit, Basis: basis, RelatedRule: definition.id}
	inputs := make(map[string]any)
	var missing []string
	var reasons []string
	conflicted := false

	if definition.flowKey != "" {
		flow, ok, conflictReason, inputConflict := bankObservation(definition.flowKey, period, basis, observations, conflicts)
		if conflictReason != "" {
			conflicted = true
			reasons = append(reasons, conflictReason)
			inputs["conflicts"] = appendBankConflict(inputs["conflicts"], inputConflict)
		} else if !ok {
			missing = append(missing, definition.flowKey)
		} else {
			inputs[definition.flowKey] = flow
			opening, closing, missingBalances, balanceConflicts := averageBankBalance(
				definition.currentBalanceKey, basis, flow, observations, conflicts)
			for _, balanceConflict := range balanceConflicts {
				conflicted = true
				reasons = append(reasons, balanceConflict.Reason)
				inputs["conflicts"] = appendBankConflict(inputs["conflicts"], &balanceConflict)
			}
			if opening != nil {
				inputs["opening_"+definition.openingBalanceKey] = opening
			}
			if closing != nil {
				inputs["closing_"+definition.currentBalanceKey] = closing
			}
			missing = append(missing, missingBalances...)
			if opening != nil && closing != nil {
				if opening.Unit != closing.Unit {
					conflicted = true
					reasons = append(reasons, "opening and closing balance units differ")
				} else {
					inputs["average_"+definition.currentBalanceKey] = (*opening.Value + *closing.Value) / 2
				}
			}
		}
	} else {
		numerator, ok, conflictReason, inputConflict := bankObservation(definition.currentBalanceKey, period, basis, observations, conflicts)
		if conflictReason != "" {
			conflicted = true
			reasons = append(reasons, conflictReason)
			inputs["conflicts"] = appendBankConflict(inputs["conflicts"], inputConflict)
		} else if !ok {
			missing = append(missing, definition.currentBalanceKey)
		} else {
			inputs[definition.currentBalanceKey] = numerator
		}
		denominator, ok, conflictReason, inputConflict := bankObservation(definition.coverageKey, period, basis, observations, conflicts)
		if conflictReason != "" {
			conflicted = true
			reasons = append(reasons, conflictReason)
			inputs["conflicts"] = appendBankConflict(inputs["conflicts"], inputConflict)
		} else if !ok {
			missing = append(missing, definition.coverageKey)
		} else {
			inputs[definition.coverageKey] = denominator
		}
	}
	if len(missing) > 0 {
		inputs["missing"] = missing
	}
	if conflicted {
		metric.Status = "conflict"
		metric.Reason = strings.Join(reasons, "; ")
		metric.Inputs = inputs
		return metric
	}
	if len(missing) > 0 {
		metric.Status = "missing_inputs"
		metric.Reason = "Required reviewed inputs are unavailable for the same period and reporting scope."
		metric.Inputs = inputs
		return metric
	}

	var numerator, denominator float64
	if definition.flowKey != "" {
		numerator = *inputs[definition.flowKey].(analysisObservation).Value
		denominator = inputs["average_"+definition.currentBalanceKey].(float64)
		flow := inputs[definition.flowKey].(analysisObservation)
		opening := inputs["opening_"+definition.openingBalanceKey].(*analysisObservation)
		closing := inputs["closing_"+definition.currentBalanceKey].(*analysisObservation)
		if flow.Unit != opening.Unit || flow.Unit != closing.Unit {
			metric.Status = "conflict"
			metric.Reason = "Flow and balance inputs use different units."
			metric.Inputs = inputs
			return metric
		}
		if flow.PeriodType != "annual" && flow.PeriodType != "interim" || flow.PeriodStart == "" || flow.PeriodEnd == "" {
			metric.Status = "not_calculable"
			metric.Reason = "Flow dates do not define a comparable measurement period."
			metric.Inputs = inputs
			return metric
		}
	} else {
		numerator = *inputs[definition.currentBalanceKey].(analysisObservation).Value
		denominator = *inputs[definition.coverageKey].(analysisObservation).Value
		balance := inputs[definition.currentBalanceKey].(analysisObservation)
		matchedBalance := inputs[definition.coverageKey].(analysisObservation)
		if balance.Unit != matchedBalance.Unit || balance.PeriodEnd != matchedBalance.PeriodEnd || balance.PeriodType != "balance" || matchedBalance.PeriodType != "balance" {
			metric.Status = "conflict"
			metric.Reason = "The inputs must be balances with the same unit and reporting date."
			metric.Inputs = inputs
			return metric
		}
	}
	if definition.optional && (numerator < 0 || numerator > denominator) {
		metric.Status = "not_calculable"
		metric.Reason = "Staged loans must be between zero and total gross staged loans."
		metric.Inputs = inputs
		return metric
	}
	if denominator <= 0 || math.IsNaN(denominator) || math.IsInf(denominator, 0) {
		metric.Status = "not_calculable"
		metric.Reason = "The matched denominator must be positive."
		metric.Inputs = inputs
		return metric
	}
	value := numerator / denominator * 100
	if math.IsNaN(value) || math.IsInf(value, 0) {
		metric.Status = "not_calculable"
		metric.Reason = "Inputs do not support a finite result."
		metric.Inputs = inputs
		return metric
	}
	metric.Value = &value
	metric.Status = "calculated"
	if definition.id == bankNPLCoverageRule {
		metric.Status = "proxy_calculated"
		metric.Reason = "Proxy: the accounting loan-loss allowance is compared with gross NPL classified under prudential rules; both are bank-only at the same date, but an identical regulatory loan perimeter has not been established."
	}
	metric.Inputs = inputs
	return metric
}

func bankObservation(key, period, basis string, observations []analysisObservation, conflicts []bankInputConflict) (analysisObservation, bool, string, *bankInputConflict) {
	_, end, _ := expectedBankPeriod(period)
	if conflict := bankConflictAt(key, basis, end, conflicts); conflict != nil {
		return analysisObservation{}, false, conflict.Reason, conflict
	}
	for _, item := range observations {
		if item.Subject == "BBCA" && item.Key == key && item.Period == period && item.Basis == basis {
			return item, true, "", nil
		}
	}
	return analysisObservation{}, false, "", nil
}

func appendBankConflict(current any, conflict *bankInputConflict) []bankInputConflict {
	result, _ := current.([]bankInputConflict)
	if conflict != nil {
		result = append(result, *conflict)
	}
	return result
}

func averageBankBalance(key, basis string, flow analysisObservation, observations []analysisObservation, conflicts []bankInputConflict) (*analysisObservation, *analysisObservation, []string, []bankInputConflict) {
	start, err := time.Parse("2006-01-02", flow.PeriodStart)
	if err != nil {
		return nil, nil, []string{key + " opening balance"}, nil
	}
	end, err := time.Parse("2006-01-02", flow.PeriodEnd)
	if err != nil {
		return nil, nil, []string{key + " closing balance"}, nil
	}
	openingDate := start.AddDate(0, 0, -1).Format("2006-01-02")
	closingDate := end.Format("2006-01-02")
	var opening, closing *analysisObservation
	for _, item := range observations {
		if item.Subject != "BBCA" || item.Key != key || item.Basis != basis || item.PeriodType != "balance" {
			continue
		}
		if item.PeriodEnd == openingDate {
			value := item
			opening = &value
		}
		if item.PeriodEnd == closingDate {
			value := item
			closing = &value
		}
	}
	var missing []string
	var balanceConflicts []bankInputConflict
	if conflict := bankConflictAt(key, basis, openingDate, conflicts); conflict != nil {
		opening = nil
		balanceConflicts = append(balanceConflicts, *conflict)
	} else if opening == nil {
		missing = append(missing, key+" opening balance at "+openingDate)
	}
	if conflict := bankConflictAt(key, basis, closingDate, conflicts); conflict != nil {
		closing = nil
		balanceConflicts = append(balanceConflicts, *conflict)
	} else if closing == nil {
		missing = append(missing, key+" closing balance at "+closingDate)
	}
	return opening, closing, missing, balanceConflicts
}

func bankConflictAt(key, basis, date string, conflicts []bankInputConflict) *bankInputConflict {
	if date == "" {
		return nil
	}
	for _, conflict := range conflicts {
		if conflict.Key == key && conflict.Basis == basis {
			_, end := bankConflictPeriod(conflict.Period)
			if end == date {
				return &conflict
			}
		}
	}
	return nil
}

func validateBankFormulaCatalog(entries []CatalogEntry) error {
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if !bankFormulaMatches(entry) {
			return fmt.Errorf("bank catalog rule %s differs from the registered calculation", entry.ID)
		}
		seen[entry.ID] = true
	}
	for _, definition := range bankFormulaDefinitions {
		if !definition.optional && !seen[definition.id] {
			return fmt.Errorf("bank catalog is missing registered calculation %s", definition.id)
		}
	}
	return nil
}
