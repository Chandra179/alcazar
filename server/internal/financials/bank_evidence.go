package financials

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func readBankAnalysisEvidence(path string) (bankAnalysisEvidence, error) {
	var evidence bankAnalysisEvidence
	file, err := os.Open(path)
	if err != nil {
		return evidence, fmt.Errorf("read bank analysis evidence %q: %w", path, err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&evidence); err != nil {
		return evidence, fmt.Errorf("parse bank analysis evidence: %w", err)
	}
	if extra := decoder.Decode(new(any)); extra != io.EOF {
		return evidence, fmt.Errorf("bank analysis evidence must contain one YAML document")
	}
	if evidence.CatalogHash == "" || evidence.FactsHash == "" || len(evidence.Rules) == 0 {
		return evidence, fmt.Errorf("bank analysis evidence requires catalog_hash, facts_hash, and rule assessments")
	}
	if len(evidence.Sources) == 0 {
		return evidence, fmt.Errorf("bank analysis evidence requires source references")
	}
	for key, source := range evidence.Sources {
		if source.Title == "" || source.Where == "" || !strings.HasPrefix(source.URL, "https://") {
			return evidence, fmt.Errorf("incomplete bank evidence source %s", key)
		}
	}
	validateAssessment := func(name string, assessment analysisAssessment) error {
		if assessment.Status == "" || assessment.Finding == "" || len(assessment.Sources) == 0 {
			return fmt.Errorf("incomplete bank assessment %s", name)
		}
		for _, source := range assessment.Sources {
			if _, ok := evidence.Sources[source]; !ok {
				return fmt.Errorf("bank assessment %s references unknown source %s", name, source)
			}
		}
		return nil
	}
	for id, assessment := range evidence.Rules {
		if err := validateAssessment(id, assessment); err != nil {
			return evidence, err
		}
	}
	seen := make(map[string]bool)
	for _, item := range evidence.Observations {
		key := strings.Join([]string{item.Key, item.Subject, item.Period, item.Basis}, "/")
		if item.Source == "" || evidence.Sources[item.Source].Title == "" || item.Value == nil ||
			math.IsNaN(*item.Value) || math.IsInf(*item.Value, 0) || item.Key == "" ||
			(item.Subject != "BBCA" && item.Key != "Reported CASA share") || item.Subject == "" || item.Period == "" || item.Basis == "" || item.Unit == "" || seen[key] {
			return evidence, fmt.Errorf("duplicate or invalid bank observation %s", key)
		}
		if item.Corroboration != "" && evidence.Sources[item.Corroboration].Title == "" {
			return evidence, fmt.Errorf("bank observation %s references unknown corroboration %s", key, item.Corroboration)
		}
		if err := validateObservationPeriod(item); err != nil {
			return evidence, fmt.Errorf("invalid bank observation %s: %w", key, err)
		}
		seen[key] = true
	}
	conflicts := make(map[string]bool, len(evidence.InputConflicts))
	for _, conflict := range evidence.InputConflicts {
		_, end := bankConflictPeriod(conflict.Period)
		key := bankInputKey(conflict.Key, end, conflict.Basis)
		if conflict.Key == "" || end == "" || conflict.Basis == "" || conflict.Reason == "" || conflicts[key] {
			return evidence, fmt.Errorf("invalid or duplicate bank input conflict %s", bankInputKey(conflict.Key, conflict.Period, conflict.Basis))
		}
		for _, source := range conflict.Sources {
			if evidence.Sources[source].Title == "" {
				return evidence, fmt.Errorf("bank input conflict %s references unknown source %s", key, source)
			}
		}
		conflicts[key] = true
	}
	return evidence, nil
}

func validateObservationPeriod(item analysisObservation) error {
	if item.PeriodType != "annual" && item.PeriodType != "interim" && item.PeriodType != "balance" {
		return fmt.Errorf("period_type must be annual, interim, or balance")
	}
	end, err := time.Parse("2006-01-02", item.PeriodEnd)
	if err != nil {
		return fmt.Errorf("period_end must be an ISO date")
	}
	if item.PeriodType == "balance" {
		if item.PeriodStart != "" {
			return fmt.Errorf("balance observation must not have period_start")
		}
		_, expectedEnd, ok := expectedBankPeriod(item.Period)
		if !ok || item.PeriodEnd != expectedEnd {
			return fmt.Errorf("balance period label must match period_end")
		}
		return nil
	}
	start, err := time.Parse("2006-01-02", item.PeriodStart)
	if err != nil || start.After(end) {
		return fmt.Errorf("flow observation requires valid period_start no later than period_end")
	}
	_, _, kind := periodBounds(item.Period)
	if kind != item.PeriodType {
		return fmt.Errorf("period label and period_type do not agree")
	}
	expectedStart, expectedEnd, ok := expectedBankPeriod(item.Period)
	if !ok || item.PeriodStart != expectedStart || item.PeriodEnd != expectedEnd {
		return fmt.Errorf("flow dates must match the labeled annual or year-to-date period")
	}
	return nil
}

func expectedBankPeriod(period string) (string, string, bool) {
	if len(period) == 4 {
		start, err := time.Parse("2006", period)
		if err != nil {
			return "", "", false
		}
		return start.Format("2006-01-02"), time.Date(start.Year(), time.December, 31, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), true
	}
	start, err := time.Parse("2006-01", period)
	if err != nil {
		return "", "", false
	}
	end := time.Date(start.Year(), start.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	return time.Date(start.Year(), time.January, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), end.Format("2006-01-02"), true
}

// bankConflictPeriod accepts the report period label or its ISO closing date.
func bankConflictPeriod(period string) (label, end string) {
	if _, end, ok := expectedBankPeriod(period); ok {
		return period, end
	}
	date, err := time.Parse("2006-01-02", period)
	if err != nil {
		return "", ""
	}
	label = date.Format("2006-01")
	if date.Month() == time.December {
		label = date.Format("2006")
	}
	_, end, _ = expectedBankPeriod(label)
	if period != end {
		return "", ""
	}
	return label, end
}

func bankInputKey(key, period, basis string) string {
	return strings.Join([]string{key, period, basis}, "/")
}
