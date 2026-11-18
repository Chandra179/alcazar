package financials

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type periodFact struct {
	Fact    Fact
	Number  float64
	Sources []Fact
}

func CalculateMetrics(ctx context.Context, db *sql.DB, period string) ([]Metric, error) {
	if strings.TrimSpace(period) == "" {
		return nil, fmt.Errorf("period is required")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	metrics, err := calculateMetrics(ctx, tx, period, "")
	if err != nil {
		return nil, err
	}
	return metrics, tx.Commit()
}

func calculateMetrics(ctx context.Context, tx *sql.Tx, period, basis string) ([]Metric, error) {
	return calculateMetricsFrom(ctx, tx, period, func(wanted string) ([]periodFact, error) {
		return approvedFactsForCalculation(ctx, tx, wanted, basis)
	})
}

func calculateMetricsFrom(ctx context.Context, tx *sql.Tx, period string, load func(string) ([]periodFact, error)) ([]Metric, error) {
	entries, err := listCatalog(tx)
	if err != nil {
		return nil, err
	}
	facts, err := load(period)
	if err != nil {
		return nil, err
	}
	values, conflicts := numericInputs(facts)
	needsAverageBalances := false
	for _, entry := range entries {
		if entry.Type == "formula" && formulaDefinitions[entry.ID].openingBalance {
			needsAverageBalances = true
			break
		}
	}
	if previousPeriod := previousBalancePeriod(period); needsAverageBalances && previousPeriod != "" {
		previousFacts, err := load(previousPeriod)
		if err != nil {
			return nil, err
		}
		previousValues, previousConflicts := numericInputs(previousFacts)
		addAverageBalanceInput(values, conflicts, previousValues, previousConflicts,
			"total_assets", "average_total_assets", period)
		addAverageBalanceInput(values, conflicts, previousValues, previousConflicts,
			"common_shareholders_equity", "average_common_shareholders_equity", period)
	}
	metrics := make([]Metric, 0)
	for _, entry := range entries {
		if entry.Type != "formula" {
			continue
		}
		metric := Metric{EntityID: entry.ID, Period: period, Status: "unsupported"}
		definition, implemented := formulaDefinitions[entry.ID]
		if !implemented {
			metric.Reason = "No deterministic implementation is registered for this formula ID."
			metrics = append(metrics, metric)
			continue
		}
		if !formulaMatches(entry) {
			metric.Reason = "The catalog expression differs from the registered calculation."
			metrics = append(metrics, metric)
			continue
		}
		if definition.status != "" {
			metric.Status, metric.Reason = definition.status, definition.reason
			metrics = append(metrics, metric)
			continue
		}
		if definition.years != 0 {
			metric, err = calculateCapitalExpenditureBurden(period, definition, load)
			if err != nil {
				return nil, err
			}
			metrics = append(metrics, metric)
			continue
		}
		required, calculate := definition.inputs, definition.compute
		metric.Unit = definition.unit
		var missing []string
		for _, key := range required {
			if conflicts[key] {
				metric.Status = "conflict"
				missing = append(missing, key+" (conflicting approved values)")
			} else if _, ok := values[key]; !ok {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			if metric.Status != "conflict" {
				metric.Status = "missing_inputs"
			}
			metric.Reason = "Required approved inputs are missing or conflicting."
			metric.Inputs = map[string]any{"missing": missing}
			metrics = append(metrics, metric)
			continue
		}
		if reason := metricInputCompatibility(values, required, entry.ID); reason != "" {
			metric.Status = "conflict"
			metric.Reason = reason
			metrics = append(metrics, metric)
			continue
		}
		value, err := calculate(metricValues(values, required, entry.ID))
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			metric.Status = "not_calculable"
			metric.Reason = "Inputs do not support a finite result."
			if err != nil {
				metric.Reason = err.Error()
			}
			metrics = append(metrics, metric)
			continue
		}
		if entry.ID == "net-change-in-cash" {
			metric.Unit = values[required[0]].Fact.Currency + " " + values[required[0]].Fact.Unit
		}
		metric.Value = &value
		metric.Status = "calculated"
		inputRefs := make(map[string]any, len(required))
		for _, key := range required {
			input := values[key]
			reference := map[string]any{"value": input.Number, "unit": input.Fact.Unit, "currency": input.Fact.Currency, "period_end": input.Fact.PeriodEnd}
			if len(input.Sources) > 1 {
				sourceFacts := make([]map[string]any, 0, len(input.Sources))
				for _, fact := range input.Sources {
					sourceFacts = append(sourceFacts, map[string]any{
						"fact_id": fact.ID, "period": fact.Period,
						"document_path": fact.DocumentPath, "source_url": fact.SourceURL,
						"page_number": fact.PageNumber, "source_locator": fact.SourceLocator,
					})
				}
				reference["source_facts"] = sourceFacts
			} else {
				reference["fact_id"] = input.Fact.ID
				reference["document_path"] = input.Fact.DocumentPath
				reference["source_url"] = input.Fact.SourceURL
				reference["page_number"] = input.Fact.PageNumber
				reference["source_locator"] = input.Fact.SourceLocator
			}
			inputRefs[key] = reference
		}
		metric.Inputs = inputRefs
		metrics = append(metrics, metric)
	}
	return metrics, nil
}

func calculateCapitalExpenditureBurden(period string, definition formulaDefinition, load func(string) ([]periodFact, error)) (Metric, error) {
	metric := Metric{EntityID: "capital-expenditure-to-net-earnings", Period: period, Unit: definition.unit}
	_, _, periodType := periodBounds(period)
	if periodType != "annual" {
		metric.Status = "requires_multi_period"
		metric.Reason = "Select a fiscal year; this formula uses a trailing ten-year window of annual statements."
		return metric, nil
	}
	endYear, err := strconv.Atoi(period)
	if err != nil || endYear < definition.years {
		metric.Status = "requires_multi_period"
		metric.Reason = "A complete trailing ten-year fiscal-year window cannot be formed for this period."
		return metric, nil
	}
	startYear := endYear - definition.years + 1
	windowStart, windowEnd := strconv.Itoa(startYear), strconv.Itoa(endYear)
	inputs := map[string]periodFact{}
	inputKeys := make([]string, 0, len(definition.inputs)*definition.years)
	annualInputs := make([]map[string]any, 0, definition.years)
	missing := make([]string, 0)
	conflicting := make([]string, 0)

	for year := startYear; year <= endYear; year++ {
		annualPeriod := strconv.Itoa(year)
		facts, err := load(annualPeriod)
		if err != nil {
			return metric, err
		}
		values, conflicts := numericInputs(facts)
		annual := map[string]any{"period": annualPeriod}
		for _, canonicalKey := range definition.inputs {
			windowKey := canonicalKey + "@" + annualPeriod
			inputKeys = append(inputKeys, windowKey)
			if conflicts[canonicalKey] {
				conflicting = append(conflicting, windowKey)
				continue
			}
			input, ok := values[canonicalKey]
			if !ok {
				missing = append(missing, windowKey)
				continue
			}
			if input.Fact.Period != annualPeriod || (input.Fact.PeriodType != "" && !samePeriodFlow(input.Fact)) {
				conflicting = append(conflicting, windowKey+" (statement dates)")
				continue
			}
			inputs[windowKey] = input
			annual[canonicalKey] = map[string]any{
				"value":        input.Number,
				"currency":     input.Fact.Currency,
				"unit":         input.Fact.Unit,
				"source_facts": sourceFactReferences(input.Sources),
			}
		}
		annualInputs = append(annualInputs, annual)
	}
	metric.Inputs = map[string]any{
		"window_start":  windowStart,
		"window_end":    windowEnd,
		"annual_inputs": annualInputs,
	}
	if len(conflicting) > 0 {
		metric.Status = "conflict"
		metric.Reason = "The ten-year window includes conflicting approved values."
		metric.Inputs["conflicting"] = conflicting
		if len(missing) > 0 {
			metric.Inputs["missing"] = missing
		}
		return metric, nil
	}
	if len(missing) > 0 {
		metric.Status = "missing_inputs"
		metric.Reason = "Every fiscal year in the ten-year window needs approved capital expenditures and net earnings."
		metric.Inputs["missing"] = missing
		return metric, nil
	}
	if reason := metricInputCompatibility(inputs, inputKeys, metric.EntityID); reason != "" {
		metric.Status = "conflict"
		metric.Reason = reason
		return metric, nil
	}

	closingInput := inputs["net_earnings@"+period].Fact
	scaleUnit := closingInput.Unit
	var totalCapitalExpenditures, totalNetEarnings float64
	for year := startYear; year <= endYear; year++ {
		annualPeriod := strconv.Itoa(year)
		totalCapitalExpenditures += math.Abs(numberAtUnit(inputs["capital_expenditures@"+annualPeriod], scaleUnit))
		totalNetEarnings += numberAtUnit(inputs["net_earnings@"+annualPeriod], scaleUnit)
	}
	metric.Inputs["total_capital_expenditures"] = totalCapitalExpenditures
	metric.Inputs["total_net_earnings"] = totalNetEarnings
	metric.Inputs["totals_unit"] = scaleUnit
	metric.Inputs["totals_currency"] = closingInput.Currency
	if math.IsNaN(totalCapitalExpenditures) || math.IsInf(totalCapitalExpenditures, 0) ||
		math.IsNaN(totalNetEarnings) || math.IsInf(totalNetEarnings, 0) {
		metric.Status = "not_calculable"
		metric.Reason = "The ten-year cumulative totals do not support a finite result."
		return metric, nil
	}
	if totalNetEarnings <= 0 {
		metric.Status = "not_calculable"
		metric.Reason = "Cumulative net earnings over the ten-year window must be positive."
		return metric, nil
	}
	value := totalCapitalExpenditures / totalNetEarnings * 100
	if math.IsNaN(value) || math.IsInf(value, 0) {
		metric.Status = "not_calculable"
		metric.Reason = "The ten-year ratio does not support a finite result."
		return metric, nil
	}
	metric.Value = &value
	metric.Status = "calculated"
	return metric, nil
}

func sourceFactReferences(facts []Fact) []map[string]any {
	references := make([]map[string]any, 0, len(facts))
	for _, fact := range facts {
		references = append(references, map[string]any{
			"fact_id": fact.ID, "period": fact.Period,
			"document_path": fact.DocumentPath, "source_url": fact.SourceURL,
			"page_number": fact.PageNumber, "source_locator": fact.SourceLocator,
		})
	}
	return references
}

func approvedFactsForCalculation(ctx context.Context, db *sql.Tx, period, basis string) ([]periodFact, error) {
	rows, err := db.QueryContext(ctx, `SELECT f.id, f.document_id, d.local_path, d.source_url, f.page_number,
		COALESCE(s.source_locator, ''),
		f.period, f.period_start, f.period_end, f.period_type, f.raw_label, f.raw_value,
		f.evidence_text, f.canonical_key, f.normalized_value, f.currency, f.unit,
		f.reporting_basis, f.audited_status, f.extraction_method, f.status, f.review_note
		FROM financial_facts f JOIN financial_documents d ON d.id = f.document_id
		LEFT JOIN financial_fact_sources s ON s.fact_id = f.id
		WHERE f.status = 'approved' AND f.period = ? AND f.canonical_key <> ''
		AND (? = '' OR f.reporting_basis = ?)
		ORDER BY d.local_path, f.page_number, f.id`, period, basis, basis)
	if err != nil {
		return nil, fmt.Errorf("query metric inputs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var result []periodFact
	for rows.Next() {
		var fact Fact
		if err := rows.Scan(&fact.ID, &fact.DocumentID, &fact.DocumentPath, &fact.SourceURL, &fact.PageNumber,
			&fact.SourceLocator,
			&fact.Period, &fact.PeriodStart, &fact.PeriodEnd, &fact.PeriodType,
			&fact.RawLabel, &fact.RawValue, &fact.EvidenceText, &fact.CanonicalKey,
			&fact.NormalizedValue, &fact.Currency, &fact.Unit, &fact.ReportingBasis,
			&fact.AuditedStatus, &fact.ExtractionMethod, &fact.Status, &fact.ReviewNote); err != nil {
			return nil, fmt.Errorf("scan metric input: %w", err)
		}
		value, ok := parseCanonicalNumber(fact.NormalizedValue)
		if !ok {
			continue
		}
		result = append(result, periodFact{Fact: fact, Number: value, Sources: []Fact{fact}})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate metric inputs: %w", err)
	}
	return result, nil
}

func previousBalancePeriod(period string) string {
	_, _, periodType := periodBounds(period)
	if periodType != "annual" && periodType != "interim" {
		return ""
	}
	year, err := strconv.Atoi(period[:4])
	if err != nil || year <= 1 {
		return ""
	}
	return strconv.Itoa(year - 1)
}

func addAverageBalanceInput(current map[string]periodFact, currentConflicts map[string]bool,
	previous map[string]periodFact, previousConflicts map[string]bool,
	sourceKey, averageKey, period string,
) {
	if _, exists := current[averageKey]; exists || currentConflicts[averageKey] {
		return
	}
	if currentConflicts[sourceKey] || previousConflicts[sourceKey] {
		currentConflicts[averageKey] = true
		return
	}
	ending, ok := current[sourceKey]
	if !ok {
		return
	}
	beginning, ok := previous[sourceKey]
	restatedKey := "restated_" + sourceKey
	if previousConflicts[restatedKey] {
		currentConflicts[averageKey] = true
		return
	}
	if restated, exists := previous[restatedKey]; exists && restated.Fact.DocumentID == ending.Fact.DocumentID {
		beginning, ok = restated, true
	}
	if !ok {
		return
	}
	balances := map[string]periodFact{"beginning": beginning, "ending": ending}
	if metricInputCompatibility(balances, []string{"beginning", "ending"}, "balance-average") != "" {
		currentConflicts[averageKey] = true
		return
	}
	derivedFact := ending.Fact
	derivedFact.ID = ""
	derivedFact.CanonicalKey = averageKey
	derivedFact.Period = period
	average := (ending.Number + numberAtUnit(beginning, ending.Fact.Unit)) / 2
	derivedFact.NormalizedValue = strconv.FormatFloat(average, 'f', -1, 64)
	current[averageKey] = periodFact{
		Fact: derivedFact, Number: average,
		Sources: append(append([]Fact(nil), ending.Sources...), beginning.Sources...),
	}
}

func numericInputs(facts []periodFact) (map[string]periodFact, map[string]bool) {
	values := make(map[string]periodFact)
	conflicts := make(map[string]bool)
	for _, item := range facts {
		key := item.Fact.CanonicalKey
		if key == "" {
			continue
		}
		previous, exists := values[key]
		if !exists || approvedSourcePriority(item.Fact) > approvedSourcePriority(previous.Fact) {
			values[key] = item
			delete(conflicts, key)
			continue
		}
		if approvedSourcePriority(item.Fact) < approvedSourcePriority(previous.Fact) {
			continue
		}
		if !equivalentInputs(previous, item) {
			conflicts[key] = true
			continue
		}
		previous.Sources = append(previous.Sources, item.Sources...)
		values[key] = previous
	}
	return values, conflicts
}

func approvedSourcePriority(fact Fact) int {
	rank := 0
	if fact.ReportingBasis == "consolidated" {
		rank += 8
	}
	if !strings.Contains(fact.SourceLocator, "/highlights/") {
		rank += 4
	}
	if !strings.Contains(fact.SourceLocator, "/published_") {
		rank += 2
	}
	if fact.AuditedStatus == "audited" {
		rank++
	}
	return rank
}

func equivalentInputs(a, b periodFact) bool {
	if a.Fact.ReportingBasis != b.Fact.ReportingBasis || a.Fact.AuditedStatus != b.Fact.AuditedStatus || a.Fact.Currency != b.Fact.Currency ||
		a.Fact.PeriodStart != b.Fact.PeriodStart || a.Fact.PeriodEnd != b.Fact.PeriodEnd || a.Fact.PeriodType != b.Fact.PeriodType {
		return false
	}
	if a.Fact.Unit != b.Fact.Unit && (!isMoneyScale(a.Fact.Unit) || !isMoneyScale(b.Fact.Unit)) {
		return false
	}
	converted := numberAtUnit(b, a.Fact.Unit)
	return math.Abs(a.Number-converted) <= math.Max(1e-9, math.Abs(a.Number)*1e-15)
}

func isMoneyScale(unit string) bool {
	switch unit {
	case "idr", "units", "thousands", "millions", "billions":
		return true
	default:
		return false
	}
}

func numberAtUnit(input periodFact, unit string) float64 {
	if input.Fact.Unit == unit {
		return input.Number
	}
	from, _ := unitScale(input.Fact.Unit)
	to, _ := unitScale(unit)
	return input.Number * (from / to)
}

func metricValues(values map[string]periodFact, required []string, formula string) map[string]periodFact {
	if formula == "earnings-per-share" {
		return values
	}
	scaled := make(map[string]periodFact, len(required))
	unit := values[required[0]].Fact.Unit
	for _, key := range required {
		input := values[key]
		input.Number = numberAtUnit(input, unit)
		scaled[key] = input
	}
	return scaled
}

func metricInputCompatibility(values map[string]periodFact, required []string, formulaID string) string {
	var flowStart, flowEnd string
	for _, key := range required {
		fact := values[key].Fact
		if fact.ReportingBasis != "consolidated" && fact.ReportingBasis != "unconsolidated" {
			return "Approved inputs include an unknown reporting basis; identify consolidated or unconsolidated statements before calculation."
		}
		if fact.AuditedStatus != "audited" && fact.AuditedStatus != "unaudited" {
			return "Approved inputs include unknown audit status; identify audited or unaudited statements before calculation."
		}
		if formulaID != "balance-average" && formulaID != "capital-expenditure-to-net-earnings" &&
			fact.PeriodType != "instant" && fact.PeriodType != "" {
			if !samePeriodFlow(fact) {
				return "Flow dates must match the reviewed reporting period."
			}
			if flowStart != "" && formulaID != "interim-net-earnings-yoy" &&
				(flowStart != fact.PeriodStart || flowEnd != fact.PeriodEnd) {
				return "Inputs cover different statement dates."
			}
			flowStart, flowEnd = fact.PeriodStart, fact.PeriodEnd
		}
	}
	if len(required) < 2 {
		return ""
	}
	first := values[required[0]]
	for _, key := range required[1:] {
		item := values[key]
		if item.Fact.ReportingBasis != first.Fact.ReportingBasis {
			return "Approved inputs use different consolidated or unconsolidated reporting bases."
		}
		if formulaID != "balance-average" && item.Fact.AuditedStatus != first.Fact.AuditedStatus {
			return "Approved inputs mix audited, unaudited, or unknown-status reports."
		}
		if formulaID == "earnings-per-share" {
			continue
		}
		if first.Fact.Currency == "" || item.Fact.Currency == "" {
			return "Approved inputs need a known currency before calculation."
		}
		if first.Fact.Unit == "" || item.Fact.Unit == "" || first.Fact.Unit == "as displayed" || item.Fact.Unit == "as displayed" {
			return "Approved inputs need explicit numeric units or scales before calculation."
		}
		if item.Fact.Currency != first.Fact.Currency {
			return "Approved inputs use different currencies."
		}
		if !isMoneyScale(item.Fact.Unit) || !isMoneyScale(first.Fact.Unit) {
			return "Approved inputs need compatible monetary units."
		}
	}
	if formulaID == "earnings-per-share" {
		if values[required[0]].Fact.Currency == "" {
			return "EPS earnings input needs a known currency before calculation."
		}
		if !isMoneyScale(values[required[0]].Fact.Unit) || values[required[1]].Fact.Unit != "shares" {
			return "EPS needs a monetary earnings amount and a share count in shares."
		}
	}
	return ""
}

func samePeriodFlow(fact Fact) bool {
	start, end, kind := periodBounds(fact.Period)
	return start != "" && fact.PeriodType == kind && fact.PeriodStart == start && fact.PeriodEnd == end
}

func unitScale(unit string) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "units", "shares", "idr", "per share":
		return 1, true
	case "thousands":
		return 1_000, true
	case "millions":
		return 1_000_000, true
	case "billions":
		return 1_000_000_000, true
	default:
		return 0, false
	}
}
