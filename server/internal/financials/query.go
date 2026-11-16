package financials

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"regexp"
	"strings"
)

type Coverage struct {
	Documents []Document    `json:"documents"`
	Gaps      []CoverageGap `json:"gaps"`
}

type periodFact struct {
	Fact   Fact
	Number float64
}

func GetCoverage(ctx context.Context, db *sql.DB) (Coverage, error) {
	var result Coverage
	rows, err := db.QueryContext(ctx, `SELECT d.id, d.sha256, d.local_path, d.source_url, d.period,
		d.document_type, d.format, d.audited, d.extraction_status,
		(SELECT COUNT(*) FROM financial_pages p WHERE p.document_id = d.id),
		(SELECT COUNT(*) FROM financial_facts f WHERE f.document_id = d.id)
		FROM financial_documents d ORDER BY d.period, d.local_path`)
	if err != nil {
		return result, fmt.Errorf("query coverage documents: %w", err)
	}
	for rows.Next() {
		var document Document
		var audited int
		if err := rows.Scan(&document.ID, &document.SHA256, &document.LocalPath, &document.SourceURL,
			&document.Period, &document.DocumentType, &document.Format, &audited,
			&document.ExtractionStatus, &document.PageCount, &document.FactCount); err != nil {
			_ = rows.Close()
			return result, fmt.Errorf("scan coverage document: %w", err)
		}
		document.Audited = audited != 0
		result.Documents = append(result.Documents, document)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return result, fmt.Errorf("iterate coverage documents: %w", err)
	}
	_ = rows.Close()
	gapRows, err := db.QueryContext(ctx, `SELECT id, period, description, source_url, status
		FROM financial_coverage_gaps ORDER BY period, id`)
	if err != nil {
		return result, fmt.Errorf("query coverage gaps: %w", err)
	}
	defer func() { _ = gapRows.Close() }()
	for gapRows.Next() {
		var gap CoverageGap
		if err := gapRows.Scan(&gap.ID, &gap.Period, &gap.Description, &gap.SourceURL, &gap.Status); err != nil {
			return result, fmt.Errorf("scan coverage gap: %w", err)
		}
		result.Gaps = append(result.Gaps, gap)
	}
	if err := gapRows.Err(); err != nil {
		return result, fmt.Errorf("iterate coverage gaps: %w", err)
	}
	return result, nil
}

func ListPeriods(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT period FROM (
		SELECT period FROM financial_documents WHERE period <> ''
		UNION SELECT period FROM financial_facts WHERE period <> ''
	) ORDER BY period`)
	if err != nil {
		return nil, fmt.Errorf("query periods: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var periods []string
	for rows.Next() {
		var period string
		if err := rows.Scan(&period); err != nil {
			return nil, fmt.Errorf("scan period: %w", err)
		}
		periods = append(periods, period)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate periods: %w", err)
	}
	return periods, nil
}

func ListApprovedFacts(ctx context.Context, db *sql.DB, period string) ([]Fact, error) {
	rows, err := db.QueryContext(ctx, `SELECT f.id, f.document_id, d.local_path, d.source_url, f.page_number,
		f.period, f.period_start, f.period_end, f.period_type, f.raw_label, f.raw_value,
		f.evidence_text, f.canonical_key, f.normalized_value, f.currency, f.unit,
		f.reporting_basis, f.audited_status, f.extraction_method, f.status, f.review_note
		FROM financial_facts f JOIN financial_documents d ON d.id = f.document_id
		WHERE f.status = 'approved' AND (? = '' OR f.period = ?)
		ORDER BY f.period, d.local_path, f.page_number, f.canonical_key`, period, period)
	if err != nil {
		return nil, fmt.Errorf("query approved facts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	facts := make([]Fact, 0)
	for rows.Next() {
		var fact Fact
		if err := rows.Scan(&fact.ID, &fact.DocumentID, &fact.DocumentPath, &fact.SourceURL, &fact.PageNumber,
			&fact.Period, &fact.PeriodStart, &fact.PeriodEnd, &fact.PeriodType,
			&fact.RawLabel, &fact.RawValue, &fact.EvidenceText, &fact.CanonicalKey,
			&fact.NormalizedValue, &fact.Currency, &fact.Unit, &fact.ReportingBasis,
			&fact.AuditedStatus, &fact.ExtractionMethod, &fact.Status, &fact.ReviewNote); err != nil {
			return nil, fmt.Errorf("scan approved fact: %w", err)
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate approved facts: %w", err)
	}
	return facts, nil
}

func CalculateMetrics(ctx context.Context, db *sql.DB, period string) ([]Metric, error) {
	if strings.TrimSpace(period) == "" {
		return nil, fmt.Errorf("period is required")
	}
	entries, err := listCatalog(db)
	if err != nil {
		return nil, err
	}
	facts, err := approvedFactsForCalculation(ctx, db, period)
	if err != nil {
		return nil, err
	}
	values, conflicts := numericInputs(facts)
	metrics := make([]Metric, 0)
	for _, entry := range entries {
		if entry.Type != "formula" {
			continue
		}
		metric := Metric{EntityID: entry.ID, Period: period, Status: "unsupported"}
		if isNotApplicableToBank(entry.ID) {
			metric.Status = "not_applicable"
			metric.Reason = "This general company formula does not map reliably to a deposit-taking bank's statement presentation."
			metrics = append(metrics, metric)
			continue
		}
		required, calculate, implemented := formulaFor(entry.ID)
		if !implemented {
			metric.Reason = "No deterministic implementation is registered for this formula ID."
			metrics = append(metrics, metric)
			continue
		}
		metric.Unit = metricUnit(entry.ID)
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
		value, err := calculate(values)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			metric.Status = "not_calculable"
			metric.Reason = "Inputs do not support a finite result, for example a zero or negative denominator."
			metrics = append(metrics, metric)
			continue
		}
		metric.Value = &value
		metric.Status = "calculated"
		inputRefs := make(map[string]any, len(required))
		for _, key := range required {
			input := values[key]
			inputRefs[key] = map[string]any{
				"value": input.Number, "fact_id": input.Fact.ID,
				"document_path": input.Fact.DocumentPath, "source_url": input.Fact.SourceURL,
				"page_number": input.Fact.PageNumber,
			}
		}
		metric.Inputs = inputRefs
		metrics = append(metrics, metric)
	}
	return metrics, nil
}

func approvedFactsForCalculation(ctx context.Context, db *sql.DB, period string) ([]periodFact, error) {
	rows, err := db.QueryContext(ctx, `SELECT f.id, f.document_id, d.local_path, d.source_url, f.page_number,
		f.period, f.period_start, f.period_end, f.period_type, f.raw_label, f.raw_value,
		f.evidence_text, f.canonical_key, f.normalized_value, f.currency, f.unit,
		f.reporting_basis, f.audited_status, f.extraction_method, f.status, f.review_note
		FROM financial_facts f JOIN financial_documents d ON d.id = f.document_id
		WHERE f.status = 'approved' AND f.period = ? AND f.canonical_key <> ''`, period)
	if err != nil {
		return nil, fmt.Errorf("query metric inputs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var result []periodFact
	for rows.Next() {
		var fact Fact
		if err := rows.Scan(&fact.ID, &fact.DocumentID, &fact.DocumentPath, &fact.SourceURL, &fact.PageNumber,
			&fact.Period, &fact.PeriodStart, &fact.PeriodEnd, &fact.PeriodType,
			&fact.RawLabel, &fact.RawValue, &fact.EvidenceText, &fact.CanonicalKey,
			&fact.NormalizedValue, &fact.Currency, &fact.Unit, &fact.ReportingBasis,
			&fact.AuditedStatus, &fact.ExtractionMethod, &fact.Status, &fact.ReviewNote); err != nil {
			return nil, fmt.Errorf("scan metric input: %w", err)
		}
		value, ok := ParseNumber(fact.NormalizedValue)
		if !ok {
			continue
		}
		result = append(result, periodFact{Fact: fact, Number: value})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate metric inputs: %w", err)
	}
	return result, nil
}

func numericInputs(facts []periodFact) (map[string]periodFact, map[string]bool) {
	values := make(map[string]periodFact)
	conflicts := make(map[string]bool)
	for _, item := range facts {
		key := item.Fact.CanonicalKey
		if key == "" || conflicts[key] {
			continue
		}
		previous, ok := values[key]
		if !ok {
			values[key] = item
			continue
		}
		if math.Abs(previous.Number-item.Number) > 1e-9 || previous.Fact.Currency != item.Fact.Currency || previous.Fact.Unit != item.Fact.Unit || previous.Fact.ReportingBasis != item.Fact.ReportingBasis || previous.Fact.AuditedStatus != item.Fact.AuditedStatus {
			delete(values, key)
			conflicts[key] = true
		}
	}
	return values, conflicts
}

func formulaFor(id string) ([]string, func(map[string]periodFact) (float64, error), bool) {
	get := func(values map[string]periodFact, key string) float64 { return values[key].Number }
	percentage := func(numerator, denominator float64) (float64, error) {
		if denominator <= 0 {
			return 0, fmt.Errorf("denominator must be positive")
		}
		return numerator / denominator * 100, nil
	}
	switch id {
	case "net-profit-margin":
		return []string{"net_earnings", "revenue"}, func(v map[string]periodFact) (float64, error) {
			return percentage(get(v, "net_earnings"), get(v, "revenue"))
		}, true
	case "earnings-per-share":
		return []string{"net_income_available_to_common_shareholders", "weighted_average_common_shares_outstanding"}, func(v map[string]periodFact) (float64, error) {
			denominator := get(v, "weighted_average_common_shares_outstanding")
			if denominator <= 0 {
				return 0, fmt.Errorf("share count must be positive")
			}
			earningsScale, _ := unitScale(v["net_income_available_to_common_shareholders"].Fact.Unit)
			sharesScale, _ := unitScale(v["weighted_average_common_shares_outstanding"].Fact.Unit)
			return get(v, "net_income_available_to_common_shareholders") * earningsScale / (denominator * sharesScale), nil
		}, true
	case "effective-tax-rate":
		return []string{"income_tax_expense", "income_before_tax"}, func(v map[string]periodFact) (float64, error) {
			return percentage(get(v, "income_tax_expense"), get(v, "income_before_tax"))
		}, true
	case "total-liabilities-to-equity":
		return []string{"total_liabilities", "shareholders_equity"}, func(v map[string]periodFact) (float64, error) {
			denominator := get(v, "shareholders_equity")
			if denominator == 0 {
				return 0, fmt.Errorf("equity must be nonzero")
			}
			return get(v, "total_liabilities") / denominator, nil
		}, true
	case "return-on-assets":
		return []string{"net_earnings", "average_total_assets"}, func(v map[string]periodFact) (float64, error) {
			return percentage(get(v, "net_earnings"), get(v, "average_total_assets"))
		}, true
	case "return-on-equity":
		return []string{"net_income_available_to_common_shareholders", "average_common_shareholders_equity"}, func(v map[string]periodFact) (float64, error) {
			return percentage(get(v, "net_income_available_to_common_shareholders"), get(v, "average_common_shareholders_equity"))
		}, true
	case "return-on-invested-capital":
		return []string{"net_operating_profit_after_tax", "average_invested_capital"}, func(v map[string]periodFact) (float64, error) {
			return percentage(get(v, "net_operating_profit_after_tax"), get(v, "average_invested_capital"))
		}, true
	case "net-change-in-cash":
		keys := []string{"net_cash_from_operating_activities", "net_cash_from_investing_activities", "net_cash_from_financing_activities"}
		return keys, func(v map[string]periodFact) (float64, error) {
			return get(v, keys[0]) + get(v, keys[1]) + get(v, keys[2]), nil
		}, true
	case "gross-margin":
		keys := []string{"revenue", "cost_of_revenue"}
		return keys, func(v map[string]periodFact) (float64, error) {
			return percentage(get(v, keys[0])-get(v, keys[1]), get(v, keys[0]))
		}, true
	case "gross-profit":
		keys := []string{"revenue", "cost_of_revenue"}
		return keys, func(v map[string]periodFact) (float64, error) { return get(v, keys[0]) - get(v, keys[1]), nil }, true
	case "operating-profit":
		keys := []string{"gross_profit", "operating_expenses"}
		return keys, func(v map[string]periodFact) (float64, error) { return get(v, keys[0]) - get(v, keys[1]), nil }, true
	case "net-receivables-to-sales":
		keys := []string{"net_receivables", "revenue"}
		return keys, func(v map[string]periodFact) (float64, error) { return percentage(get(v, keys[0]), get(v, keys[1])) }, true
	case "working-capital":
		keys := []string{"current_assets", "current_liabilities"}
		return keys, func(v map[string]periodFact) (float64, error) { return get(v, keys[0]) - get(v, keys[1]), nil }, true
	case "current-ratio":
		keys := []string{"current_assets", "current_liabilities"}
		return keys, func(v map[string]periodFact) (float64, error) {
			if get(v, keys[1]) <= 0 {
				return 0, fmt.Errorf("current liabilities must be positive")
			}
			return get(v, keys[0]) / get(v, keys[1]), nil
		}, true
	case "quick-ratio":
		keys := []string{"current_assets", "inventory", "current_liabilities"}
		return keys, func(v map[string]periodFact) (float64, error) {
			if get(v, keys[2]) <= 0 {
				return 0, fmt.Errorf("current liabilities must be positive")
			}
			return (get(v, keys[0]) - get(v, keys[1])) / get(v, keys[2]), nil
		}, true
	default:
		return nil, nil, false
	}
}

func metricInputCompatibility(values map[string]periodFact, required []string, formulaID string) string {
	if len(required) < 2 {
		return ""
	}
	first := values[required[0]]
	for _, key := range required[1:] {
		item := values[key]
		if item.Fact.ReportingBasis != first.Fact.ReportingBasis {
			return "Approved inputs use different consolidated or unconsolidated reporting bases."
		}
		if item.Fact.AuditedStatus != first.Fact.AuditedStatus {
			return "Approved inputs mix audited, unaudited, or unknown-status reports."
		}
		if formulaID == "earnings-per-share" {
			continue
		}
		if item.Fact.Currency != first.Fact.Currency {
			return "Approved inputs use different currencies."
		}
		if item.Fact.Unit != first.Fact.Unit {
			return "Approved inputs use different numeric scales or units."
		}
	}
	if formulaID == "earnings-per-share" {
		for _, key := range required {
			if _, ok := unitScale(values[key].Fact.Unit); !ok {
				return "EPS inputs need explicit recognized scales before a per-share value can be calculated."
			}
		}
	}
	return ""
}

func metricUnit(formulaID string) string {
	switch formulaID {
	case "net-profit-margin", "effective-tax-rate", "return-on-assets", "return-on-equity", "gross-margin", "net-receivables-to-sales":
		return "percent"
	case "earnings-per-share":
		return "currency per share"
	case "total-liabilities-to-equity", "current-ratio", "quick-ratio":
		return "ratio"
	case "gross-profit", "operating-profit", "working-capital", "net-change-in-cash":
		return "source currency and scale"
	default:
		return ""
	}
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

var notApplicableBankFormulas = map[string]bool{
	"cost-of-goods-sold":           true,
	"gross-margin":                 true,
	"gross-profit":                 true,
	"sga-to-gross-profit":          true,
	"rd-to-gross-profit":           true,
	"depreciation-to-gross-profit": true,
	"interest-to-operating-income": true,
	"net-receivables-to-sales":     true,
	"return-on-invested-capital":   true,
	"working-capital":              true,
	"net-current-asset-value":      true,
	"current-ratio":                true,
	"quick-ratio":                  true,
}

func isNotApplicableToBank(id string) bool { return notApplicableBankFormulas[id] }

func ListReviewPrompts(ctx context.Context, db *sql.DB, period string) ([]ReviewPrompt, error) {
	entries, err := listCatalog(db)
	if err != nil {
		return nil, err
	}
	pages, err := pagesForPeriod(ctx, db, period)
	if err != nil {
		return nil, err
	}
	var prompts []ReviewPrompt
	for _, entry := range entries {
		if entry.Type != "concept" && entry.Type != "heuristic" {
			continue
		}
		terms := promptTerms(entry)
		prompt := ReviewPrompt{Entity: entry, Period: period}
		for _, page := range pages {
			if containsAnyTerm(page.Text, terms) {
				prompt.Evidence = append(prompt.Evidence, EvidenceRef{
					DocumentID: page.DocumentID, DocumentPath: page.DocumentPath,
					SourceURL: page.SourceURL, PageNumber: page.PageNumber,
					Text: excerpt(page.Text, terms),
				})
				if len(prompt.Evidence) == 5 {
					break
				}
			}
		}
		prompts = append(prompts, prompt)
	}
	return prompts, nil
}

type promptPage struct {
	DocumentID   string
	DocumentPath string
	SourceURL    string
	PageNumber   int
	Text         string
}

func pagesForPeriod(ctx context.Context, db *sql.DB, period string) ([]promptPage, error) {
	rows, err := db.QueryContext(ctx, `SELECT p.document_id, d.local_path, d.source_url, p.page_number, p.text
		FROM financial_pages p JOIN financial_documents d ON d.id = p.document_id
		WHERE d.period = ? ORDER BY d.local_path, p.page_number`, period)
	if err != nil {
		return nil, fmt.Errorf("query pages for prompts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var pages []promptPage
	for rows.Next() {
		var page promptPage
		if err := rows.Scan(&page.DocumentID, &page.DocumentPath, &page.SourceURL, &page.PageNumber, &page.Text); err != nil {
			return nil, fmt.Errorf("scan prompt page: %w", err)
		}
		pages = append(pages, page)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prompt pages: %w", err)
	}
	return pages, nil
}

var promptStopWords = map[string]bool{"the": true, "and": true, "for": true, "with": true, "from": true, "over": true, "into": true, "as": true, "of": true, "to": true, "in": true, "vs": true, "a": true}

func promptTerms(entry CatalogEntry) []string {
	text := strings.ToLower(entry.ID + " " + entry.Description)
	words := regexp.MustCompile(`[a-z]{4,}`).FindAllString(text, -1)
	seen := make(map[string]bool)
	var terms []string
	for _, word := range words {
		if !promptStopWords[word] && !seen[word] {
			seen[word] = true
			terms = append(terms, word)
		}
	}
	return terms
}

func containsAnyTerm(text string, terms []string) bool {
	lower := strings.ToLower(text)
	for _, term := range terms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func excerpt(text string, terms []string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if containsAnyTerm(line, terms) {
			line = strings.TrimSpace(line)
			if len(line) > 320 {
				line = line[:320]
			}
			return line
		}
	}
	text = strings.TrimSpace(text)
	if len(text) > 320 {
		return text[:320]
	}
	return text
}
