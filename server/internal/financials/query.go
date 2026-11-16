package financials

import (
	"context"
	"database/sql"
	"fmt"
)

type Coverage struct {
	Documents   []Document       `json:"documents"`
	Gaps        []CoverageGap    `json:"gaps"`
	Periods     []PeriodCoverage `json:"periods"`
	CatalogHash string           `json:"catalog_hash,omitempty"`
}

type PeriodCoverage struct {
	Period                       string   `json:"period"`
	Pending                      int      `json:"pending"`
	Approved                     int      `json:"approved"`
	MissingConsolidatedCoreFacts []string `json:"missing_consolidated_core_facts"`
}

var coreStatementFacts = []string{"total_assets", "total_liabilities", "shareholders_equity", "common_shareholders_equity", "income_before_tax", "income_tax_expense", "net_earnings", "net_income_available_to_common_shareholders", "net_cash_from_operating_activities", "net_cash_from_investing_activities", "net_cash_from_financing_activities"}

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
		switch {
		case document.DocumentType == "Capital/risk exposure":
			document.StatementStatus = "outside_statement_scope"
		case document.ExtractionStatus != "processed":
			document.StatementStatus = "pending"
		case document.FactCount == 0:
			document.StatementStatus = "no_statement_facts"
		default:
			document.StatementStatus = "has_candidates"
		}
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
	if err := gapRows.Close(); err != nil {
		return result, err
	}
	result.Periods, err = approvedCoverage(ctx, db)
	if err != nil {
		return result, err
	}
	err = db.QueryRowContext(ctx, `SELECT catalog_hash FROM fundamental_entities LIMIT 1`).Scan(&result.CatalogHash)
	if err == sql.ErrNoRows {
		err = nil
	}
	return result, err
}

func approvedCoverage(ctx context.Context, db *sql.DB) ([]PeriodCoverage, error) {
	periods, err := ListPeriods(ctx, db)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT period, canonical_key, reporting_basis, status, count(*) FROM financial_facts
        WHERE period <> '' AND status IN ('pending','approved') GROUP BY period, canonical_key, reporting_basis, status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]PeriodCoverage)
	approvedKeys := make(map[string]map[string]bool)
	for rows.Next() {
		var period, key, basis, status string
		var count int
		if err := rows.Scan(&period, &key, &basis, &status, &count); err != nil {
			return nil, err
		}
		coverage := counts[period]
		if status == "pending" {
			coverage.Pending += count
		} else {
			coverage.Approved += count
			if basis == "consolidated" {
				if approvedKeys[period] == nil {
					approvedKeys[period] = make(map[string]bool)
				}
				approvedKeys[period][key] = true
			}
		}
		counts[period] = coverage
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]PeriodCoverage, 0, len(periods))
	for _, period := range periods {
		coverage := counts[period]
		coverage.Period = period
		coverage.MissingConsolidatedCoreFacts = make([]string, 0)
		for _, key := range coreStatementFacts {
			if !approvedKeys[period][key] {
				coverage.MissingConsolidatedCoreFacts = append(coverage.MissingConsolidatedCoreFacts, key)
			}
		}
		result = append(result, coverage)
	}
	return result, nil
}

func ListPeriods(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT period FROM (
		SELECT period FROM financial_documents WHERE period <> ''
		UNION SELECT period FROM financial_facts WHERE period <> '' AND status <> 'rejected'
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
		COALESCE(s.source_locator, ''),
		f.period, f.period_start, f.period_end, f.period_type, f.raw_label, f.raw_value,
		f.evidence_text, f.canonical_key, f.normalized_value, f.currency, f.unit,
		f.reporting_basis, f.audited_status, f.extraction_method, f.status, f.review_note
		FROM financial_facts f JOIN financial_documents d ON d.id = f.document_id
		LEFT JOIN financial_fact_sources s ON s.fact_id = f.id
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
			&fact.SourceLocator,
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
