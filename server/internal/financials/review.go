package financials

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var reviewCSVHeader = []string{
	"id", "status", "canonical_key", "period", "normalized_value", "currency", "unit",
	"reporting_basis", "audited_status", "review_note", "document_path", "source_url", "page_number", "source_locator",
	"raw_label", "raw_value", "evidence_text",
	"candidate_revision", "period_start", "period_end", "period_type",
}

const observationJSON = `json_object('id', f.id, 'raw_label', f.raw_label, 'raw_value', f.raw_value,
 'evidence', f.evidence_text, 'key', f.canonical_key, 'period', f.period, 'period_start', f.period_start,
 'period_end', f.period_end, 'period_type', f.period_type, 'value', f.normalized_value,
 'currency', f.currency, 'unit', f.unit, 'basis', f.reporting_basis, 'audit', f.audited_status)`

func observationRevision(observation string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(observation)))
}

type ReviewFilter struct {
	Period   string
	Document string
}

func ExportReviewCSV(ctx context.Context, db *sql.DB, filename string, filter ReviewFilter) error {
	if filter.Period != "" {
		if _, _, kind := periodBounds(filter.Period); kind == "unknown" {
			return fmt.Errorf("review period must be YYYY or YYYY-MM")
		}
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return fmt.Errorf("create review directory: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(filename), ".bbca-review-*.csv")
	if err != nil {
		return fmt.Errorf("create review file: %w", err)
	}
	defer func() { _ = file.Close(); _ = os.Remove(file.Name()) }()
	csvWriter := csv.NewWriter(file)
	if err := csvWriter.Write(reviewCSVHeader); err != nil {
		return fmt.Errorf("write review header: %w", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT f.id, f.status, f.canonical_key, f.period,
		f.normalized_value, f.currency, f.unit, f.reporting_basis, f.audited_status,
		f.review_note, d.local_path, d.source_url, f.page_number, COALESCE(s.source_locator, ''),
		f.raw_label, f.raw_value, f.evidence_text, `+observationJSON+`, f.period_start, f.period_end, f.period_type
		FROM financial_facts f JOIN financial_documents d ON d.id = f.document_id
		LEFT JOIN financial_fact_sources s ON s.fact_id = f.id
		WHERE f.status = 'pending' AND (? = '' OR f.period = ?) AND (? = '' OR d.local_path = ?)
        ORDER BY d.local_path, f.page_number, f.period, f.canonical_key, f.id`, filter.Period, filter.Period, filter.Document, filter.Document)
	if err != nil {
		return fmt.Errorf("query pending facts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var row []string
		var id, status, key, period, value, currency, unit, basis, audited, note, docPath, sourceURL, sourceLocator, label, raw, evidence string
		var observation, start, end, kind string
		var page int
		if err := rows.Scan(&id, &status, &key, &period, &value, &currency, &unit, &basis,
			&audited, &note, &docPath, &sourceURL, &page, &sourceLocator, &label, &raw, &evidence, &observation, &start, &end, &kind); err != nil {
			return fmt.Errorf("scan pending fact: %w", err)
		}
		row = []string{id, status, key, period, value, currency, unit, basis, audited, note,
			docPath, sourceURL, strconv.Itoa(page), sourceLocator, label, raw, evidence, observationRevision(observation), start, end, kind}
		if err := csvWriter.Write(row); err != nil {
			return fmt.Errorf("write review row: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate pending facts: %w", err)
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return fmt.Errorf("flush review file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close review file: %w", err)
	}
	if err := os.Rename(file.Name(), filename); err != nil {
		return fmt.Errorf("publish review CSV: %w", err)
	}
	return nil
}

func ApplyReviewCSV(ctx context.Context, db *sql.DB, filename string) (int, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, fmt.Errorf("open review file: %w", err)
	}
	defer func() { _ = file.Close() }()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return 0, fmt.Errorf("read review header: %w", err)
	}
	columns := make(map[string]int, len(header))
	for i, name := range header {
		columns[strings.ToLower(strings.TrimSpace(name))] = i
	}
	for _, required := range []string{"id", "status", "canonical_key", "period", "normalized_value", "candidate_revision", "currency", "unit", "reporting_basis", "audited_status"} {
		if _, ok := columns[required]; !ok {
			return 0, fmt.Errorf("review file is missing %q column; export a fresh review CSV", required)
		}
	}
	get := func(row []string, name string) string {
		index, ok := columns[name]
		if !ok || index >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[index])
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin review update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	count := 0
	seen := make(map[string]bool)
	line := 1
	for {
		row, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, fmt.Errorf("read review row %d: %w", line+1, err)
		}
		line++
		id := get(row, "id")
		if id == "" {
			continue
		}
		if seen[id] {
			return 0, fmt.Errorf("duplicate candidate id %q in review file", id)
		}
		seen[id] = true
		status := strings.ToLower(get(row, "status"))
		if status == "" {
			status = "pending"
		}
		if status != "approved" && status != "rejected" && status != "pending" {
			return 0, fmt.Errorf("row %d: status must be pending, approved, or rejected", line)
		}
		if status == "pending" {
			continue
		}
		var observation, extractedKind string
		err = tx.QueryRowContext(ctx, `SELECT `+observationJSON+`, f.period_type FROM financial_facts f WHERE f.id = ? AND f.status = 'pending'`, id).Scan(&observation, &extractedKind)
		if err == sql.ErrNoRows || (err == nil && observationRevision(observation) != get(row, "candidate_revision")) {
			return 0, fmt.Errorf("row %d: stale or already reviewed candidate %q; export a fresh review CSV", line, id)
		}
		if err != nil {
			return 0, fmt.Errorf("read candidate %q: %w", id, err)
		}
		if status == "approved" {
			key, period := get(row, "canonical_key"), get(row, "period")
			value, currency, unit := get(row, "normalized_value"), strings.ToUpper(get(row, "currency")), strings.ToLower(get(row, "unit"))
			basis, audit := strings.ToLower(get(row, "reporting_basis")), strings.ToLower(get(row, "audited_status"))
			if _, ok := parseCanonicalNumber(value); !ok {
				return 0, fmt.Errorf("row %d: normalized_value needs a decimal without grouping separators", line)
			}
			start, end, kind := factPeriodBounds(period, key)
			if end == "" || !knownFactKey(key) {
				return 0, fmt.Errorf("row %d: approve requires a known fact key and a valid fiscal period", line)
			}
			if basis != "consolidated" && basis != "unconsolidated" || audit != "audited" && audit != "unaudited" {
				return 0, fmt.Errorf("row %d: approve requires known reporting basis and audit status", line)
			}
			if !validFactUnit(key, currency, unit) {
				return 0, fmt.Errorf("row %d: approve requires explicit fact currency and units", line)
			}
			if get(row, "period_end") != "" && get(row, "period_end") != end {
				return 0, fmt.Errorf("row %d: period_end must match period; correct both fields", line)
			}
			if extractedKind == "interim_unknown_start" || (get(row, "period_start") != "" && get(row, "period_start") != start) {
				if kind != "interim" || get(row, "period_start") != start {
					return 0, fmt.Errorf("row %d: confirm the January 1 start date before approving a year-to-date flow", line)
				}
			}

			_, err = tx.ExecContext(ctx, `UPDATE financial_facts SET status = ?, canonical_key = ?, period = ?,
                period_start = ?, period_end = ?, period_type = ?, normalized_value = ?, currency = ?, unit = ?,
                reporting_basis = ?, audited_status = ?, review_note = ? WHERE id = ?`, status, key, period,
				start, end, kind, value, currency, unit, basis, audit, get(row, "review_note"), id)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE financial_facts SET status = 'rejected', review_note = ? WHERE id = ?`, get(row, "review_note"), id)
		}
		if err != nil {
			return 0, fmt.Errorf("review candidate %q: %w", id, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO financial_review_decisions (fact_id, observation, decision) VALUES (?, ?, ?)`, id, observation, status); err != nil {
			return 0, fmt.Errorf("record review decision for %q: %w", id, err)
		}

		count++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit review: %w", err)
	}
	return count, nil
}

func knownFactKey(key string) bool {
	switch key {
	case "restated_total_assets", "restated_common_shareholders_equity", "revenue":
		return true
	}
	for _, alias := range financialAliases {
		if alias.key == key {
			return true
		}
	}
	for _, value := range statementKeys {
		if value == key {
			return true
		}
	}
	return false
}

func factPeriodBounds(period, key string) (string, string, string) {
	start, end, kind := periodBounds(period)
	if end != "" && statementKeyAllowed("balance", strings.TrimPrefix(key, "restated_")) {
		return "", end, "instant"
	}
	return start, end, kind
}

func validFactUnit(key, currency, unit string) bool {
	switch key {
	case "weighted_average_common_shares_outstanding":
		return unit == "shares" && currency == ""
	case "reported_earnings_per_share":
		return unit == "per share" && currency == "IDR"
	default:
		return currency == "IDR" && isMoneyScale(unit)
	}
}
