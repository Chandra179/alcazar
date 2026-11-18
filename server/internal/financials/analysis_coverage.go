package financials

import (
	"context"
	"database/sql"
	"math/big"
	"sort"
	"strings"
)

type candidateScope struct {
	Measure  string `json:"measure"`
	Period   string `json:"period"`
	Start    string `json:"start,omitempty"`
	End      string `json:"end,omitempty"`
	Kind     string `json:"kind"`
	Basis    string `json:"basis"`
	Audit    string `json:"audit"`
	Currency string `json:"currency,omitempty"`
	Unit     string `json:"unit"`
}

type candidateSource struct {
	ID       string `json:"id"`
	Document string `json:"document"`
	URL      string `json:"url"`
	Locator  string `json:"locator"`
	Status   string `json:"status"`
	Page     int    `json:"page,omitempty"`
}

type candidateValue struct {
	Value   string            `json:"value"`
	Sources []candidateSource `json:"sources"`
}

type candidateGroup struct {
	candidateScope
	Values []candidateValue `json:"values"`
}

type analysisDocumentCoverage struct {
	Path           string   `json:"path"`
	URL            string   `json:"url"`
	Period         string   `json:"period"`
	Approved       int      `json:"approved"`
	Pending        int      `json:"pending"`
	ReviewedInputs int      `json:"reviewed_statement_inputs,omitempty"`
	Rules          []string `json:"rules,omitempty"`
	Periods        []string `json:"periods,omitempty"`
	Corroborated   int      `json:"corroborated"`
	Role           string   `json:"role"`
}

type analysisCoverage struct {
	Documents  []analysisDocumentCoverage `json:"documents"`
	Candidates []candidateGroup           `json:"candidates"`
}

func groupAnalysisCandidates(facts []Fact) []candidateGroup {
	index := make(map[candidateScope]int)
	values := make(map[candidateScope]map[string]int)
	var groups []candidateGroup
	for _, f := range facts {
		if _, ok := parseCanonicalNumber(f.NormalizedValue); !ok || f.CanonicalKey == "" {
			continue
		}
		v, ok := new(big.Rat).SetString(f.NormalizedValue)
		if !ok {
			continue
		}
		unit := f.Unit
		if f.Currency != "" && isMoneyScale(unit) {
			scale, _ := unitScale(unit)
			v.Mul(v, new(big.Rat).SetInt64(int64(scale)))
			unit = "units"
		}
		key := candidateScope{f.CanonicalKey, f.Period, f.PeriodStart, f.PeriodEnd, f.PeriodType, f.ReportingBasis, f.AuditedStatus, f.Currency, unit}
		idx, exists := index[key]
		if !exists {
			idx = len(groups)
			index[key] = idx
			values[key] = make(map[string]int)
			groups = append(groups, candidateGroup{candidateScope: key})
		}
		group := &groups[idx]
		normalized := v.RatString()
		valueIndex, found := values[key][normalized]
		if !found {
			valueIndex = len(group.Values)
			values[key][normalized] = valueIndex
			value := strings.TrimRight(strings.TrimRight(v.FloatString(18), "0"), ".")
			group.Values = append(group.Values, candidateValue{Value: value})
		}
		source := candidateSource{f.ID, f.DocumentPath, f.SourceURL, f.SourceLocator, f.Status, f.PageNumber}
		group.Values[valueIndex].Sources = append(group.Values[valueIndex].Sources, source)
	}
	return groups
}

func buildAnalysisCoverage(ctx context.Context, tx *sql.Tx, report *AnalysisReport, inputs []analysisStatementInput) (analysisCoverage, error) {
	var coverage analysisCoverage
	rows, err := tx.QueryContext(ctx, "SELECT local_path, source_url, period FROM financial_documents ORDER BY period, local_path")
	if err != nil {
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var document analysisDocumentCoverage
		if err := rows.Scan(&document.Path, &document.URL, &document.Period); err != nil {
			return coverage, err
		}
		coverage.Documents = append(coverage.Documents, document)
	}
	if err := rows.Err(); err != nil {
		return coverage, err
	}
	if err := rows.Close(); err != nil {
		return coverage, err
	}
	facts, err := listFinancialFacts(ctx, tx, "", false)
	if err != nil {
		return coverage, err
	}
	coverage.Candidates = groupAnalysisCandidates(facts)
	byPath := make(map[string]*analysisDocumentCoverage)
	byURL := make(map[string]*analysisDocumentCoverage)
	rules := make(map[string]map[string]bool)
	periods := make(map[string]map[string]bool)
	for i := range coverage.Documents {
		doc := &coverage.Documents[i]
		byPath[doc.Path] = doc
		if doc.URL != "" {
			byURL[doc.URL] = doc
		}
		rules[doc.Path], periods[doc.Path] = make(map[string]bool), make(map[string]bool)
	}
	for _, f := range facts {
		doc := byPath[f.DocumentPath]
		switch f.Status {
		case "approved":
			doc.Approved++
		case "pending":
			doc.Pending++
		}
		if f.Period != "" && f.Status != "rejected" {
			periods[doc.Path][f.Period] = true
		}
		for _, rule := range rulesForFact(f.CanonicalKey) {
			if f.Status != "rejected" {
				rules[doc.Path][rule] = true
			}
		}
	}
	for _, group := range coverage.Candidates {
		for _, value := range group.Values {
			var approved, seen = make(map[string]bool), make(map[string]bool)
			for _, src := range value.Sources {
				if src.Status == "approved" {
					approved[src.Document] = true
				}
			}
			for _, src := range value.Sources {
				if src.Status == "pending" && !approved[src.Document] && len(approved) > 0 && !seen[src.Document] {
					byPath[src.Document].Corroborated++
					seen[src.Document] = true
				}
			}
		}
	}
	reviewed := make(map[string]bool)
	corroboratingDisclosure := make(map[string]bool)
	for _, input := range inputs {
		url, _, _ := strings.Cut(report.Sources[input.Source].URL, "#")
		if doc := byURL[url]; doc != nil {
			periods[doc.Path][input.Period] = true
			reviewed[doc.Path] = true
			doc.ReviewedInputs += len(input.Values)
			for key := range input.Values {
				for _, rule := range rulesForFact(key) {
					rules[doc.Path][rule] = true
				}
			}
		}
	}
	for _, rule := range report.Rules {
		for _, source := range rule.Sources {
			url, _, _ := strings.Cut(report.Sources[source].URL, "#")
			if doc := byURL[url]; doc != nil {
				rules[doc.Path][rule.Rule.ID] = true
				reviewed[doc.Path] = true
			}
		}
	}
	for _, obs := range report.Observations {
		for _, key := range []string{obs.Source, obs.Corroboration} {
			url, _, _ := strings.Cut(report.Sources[key].URL, "#")
			if doc := byURL[url]; doc != nil {
				periods[doc.Path][obs.Period] = true
				if key == obs.Corroboration {
					corroboratingDisclosure[doc.Path] = true
				} else {
					reviewed[doc.Path] = true
				}
				if strings.HasSuffix(obs.Key, " (KM1)") {
					rules[doc.Path]["liquidity-as-financial-optionality"] = true
					rules[doc.Path]["debt-maturity-and-refinancing-risk"] = true
				}
			}
		}
	}
	for i := range coverage.Documents {
		doc := &coverage.Documents[i]
		for key := range rules[doc.Path] {
			doc.Rules = append(doc.Rules, key)
		}
		for period := range periods[doc.Path] {
			doc.Periods = append(doc.Periods, period)
		}
		sort.Strings(doc.Rules)
		sort.Strings(doc.Periods)
		switch {
		case doc.Approved > 0 || reviewed[doc.Path]:
			doc.Role = "Reviewed input or disclosure"
		case corroboratingDisclosure[doc.Path]:
			doc.Role = "Corroborates reviewed disclosure"
		case doc.Corroborated > 0:
			doc.Role = "Corroborates approved facts"
		case doc.Pending > 0:
			doc.Role = "Candidates need source review"
		default:
			doc.Role = "No matched evidence"
		}
	}
	return coverage, nil
}

func rulesForFact(key string) []string {
	var ids []string
	for id, definition := range formulaDefinitions {
		for _, input := range definition.inputs {
			if input == key ||
				(key == "total_assets" || key == "restated_total_assets") && input == "average_total_assets" ||
				(key == "common_shareholders_equity" || key == "restated_common_shareholders_equity") && input == "average_common_shareholders_equity" {
				ids = append(ids, id)
				break
			}
		}
	}
	for id, keys := range promptFactKeys {
		for _, candidate := range keys {
			if candidate == key && id != "statement-of-shareholders-equity" {
				ids = append(ids, id)
				break
			}
		}
	}
	return ids
}
