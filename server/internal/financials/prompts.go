package financials

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// A prompt only uses facts whose accounting meaning is relevant to its question.
// Catalog narrative and reading dependencies remain visible in the response.
var promptFactKeys = map[string][]string{
	"balance-sheet":                      {"total_assets", "total_liabilities", "shareholders_equity", "common_shareholders_equity"},
	"income-statement":                   {"net_earnings", "income_before_tax", "income_tax_expense", "net_income_available_to_common_shareholders"},
	"income-before-tax":                  {"income_before_tax", "income_tax_expense", "net_earnings"},
	"cash-flow-statement":                {"net_cash_from_operating_activities", "net_cash_from_investing_activities", "net_cash_from_financing_activities", "capital_expenditures"},
	"subsidiary-earnings-reconciliation": {"net_earnings", "net_income_available_to_common_shareholders"},
	"statement-of-shareholders-equity":   {"shareholders_equity", "common_shareholders_equity"},
}

func ListReviewPrompts(ctx context.Context, db *sql.DB, period string) ([]ReviewPrompt, error) {
	entries, err := listCatalog(db)
	if err != nil {
		return nil, err
	}
	evidence, err := promptFactEvidence(ctx, db, period)
	if err != nil {
		return nil, err
	}
	prompts := make([]ReviewPrompt, 0)
	for _, entry := range entries {
		if entry.Type != "concept" && entry.Type != "heuristic" {
			continue
		}
		prompt := ReviewPrompt{Entity: entry, Period: period}
		for _, key := range promptFactKeys[entry.ID] {
			if ref, ok := evidence[key]; ok {
				prompt.Evidence = append(prompt.Evidence, ref)
			}
		}
		if entry.ID == "debt-maturity-and-refinancing-risk" {
			prompt.Evidence, err = fundingMaturityEvidence(ctx, db, period)
			if err != nil {
				return nil, err
			}
		}
		if len(prompt.Evidence) > 0 {
			prompts = append(prompts, prompt)
		}
	}
	return prompts, nil
}

func promptFactEvidence(ctx context.Context, db *sql.DB, period string) (map[string]EvidenceRef, error) {
	rows, err := db.QueryContext(ctx, `SELECT f.canonical_key, d.id, d.local_path, d.source_url, f.page_number,
        f.evidence_text, f.id, COALESCE(s.source_locator,''), f.status, f.period
        FROM financial_facts f JOIN financial_documents d ON d.id = f.document_id
        LEFT JOIN financial_fact_sources s ON s.fact_id = f.id
        WHERE f.period = ? AND f.status IN ('approved','pending')
        ORDER BY f.status = 'approved' DESC, f.reporting_basis = 'consolidated' DESC,
        COALESCE(s.source_locator,'') LIKE '%/highlights/%', d.audited DESC, d.local_path, f.page_number`, period)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	evidence := make(map[string]EvidenceRef)
	for rows.Next() {
		var ref EvidenceRef
		var key string
		if err := rows.Scan(&key, &ref.DocumentID, &ref.DocumentPath, &ref.SourceURL, &ref.PageNumber, &ref.Text, &ref.FactID, &ref.SourceLocator, &ref.FactStatus, &ref.Period); err != nil {
			return nil, err
		}
		if _, exists := evidence[key]; !exists {
			evidence[key] = ref
		}
	}
	return evidence, rows.Err()
}

func fundingMaturityEvidence(ctx context.Context, db *sql.DB, period string) ([]EvidenceRef, error) {
	rows, err := db.QueryContext(ctx, `SELECT d.id, d.local_path, d.source_url, p.page_number, p.text
        FROM financial_documents d JOIN financial_pages p ON p.document_id = d.id
        WHERE d.period = ? AND (lower(p.text) LIKE '%maturity profile%' OR lower(p.text) LIKE '%profil jatuh tempo%')
        ORDER BY d.audited DESC, d.local_path, p.page_number LIMIT 5`, period)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []EvidenceRef
	for rows.Next() {
		var ref EvidenceRef
		var page string
		if err := rows.Scan(&ref.DocumentID, &ref.DocumentPath, &ref.SourceURL, &ref.PageNumber, &page); err != nil {
			return nil, err
		}
		for _, line := range strings.Split(page, "\n") {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "maturity profile") || strings.Contains(lower, "profil jatuh tempo") {
				ref.Text, ref.Period = strings.TrimSpace(line), period
				ref.SourceLocator = fmt.Sprintf("page:%d", ref.PageNumber)
				results = append(results, ref)
				break
			}
		}
	}
	return results, rows.Err()
}
