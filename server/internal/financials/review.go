package financials

import (
	"context"
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
	"reporting_basis", "audited_status", "review_note", "document_path", "source_url", "page_number",
	"raw_label", "raw_value", "evidence_text",
}

func ExportReviewCSV(ctx context.Context, db *sql.DB, filename string) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return fmt.Errorf("create review directory: %w", err)
	}
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create review file: %w", err)
	}
	defer func() { _ = file.Close() }()
	csvWriter := csv.NewWriter(file)
	if err := csvWriter.Write(reviewCSVHeader); err != nil {
		return fmt.Errorf("write review header: %w", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT f.id, f.status, f.canonical_key, f.period,
		f.normalized_value, f.currency, f.unit, f.reporting_basis, f.audited_status,
		f.review_note, d.local_path, d.source_url, f.page_number, f.raw_label, f.raw_value, f.evidence_text
		FROM financial_facts f JOIN financial_documents d ON d.id = f.document_id
		WHERE f.status = 'pending' ORDER BY d.local_path, f.page_number, f.id`)
	if err != nil {
		return fmt.Errorf("query pending facts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var row []string
		var id, status, key, period, value, currency, unit, basis, audited, note, docPath, sourceURL, label, raw, evidence string
		var page int
		if err := rows.Scan(&id, &status, &key, &period, &value, &currency, &unit, &basis,
			&audited, &note, &docPath, &sourceURL, &page, &label, &raw, &evidence); err != nil {
			return fmt.Errorf("scan pending fact: %w", err)
		}
		row = []string{id, status, key, period, value, currency, unit, basis, audited, note,
			docPath, sourceURL, strconv.Itoa(page), label, raw, evidence}
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
	for _, required := range []string{"id", "status", "canonical_key", "period", "normalized_value"} {
		if _, ok := columns[required]; !ok {
			return 0, fmt.Errorf("review file is missing %q column", required)
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
		value := get(row, "normalized_value")
		if status == "approved" {
			number, ok := ParseNumber(value)
			if !ok {
				return 0, fmt.Errorf("row %d: approved facts need a numeric normalized_value", line)
			}
			value = strconv.FormatFloat(number, 'f', -1, 64)
			if get(row, "canonical_key") == "" || get(row, "period") == "" {
				return 0, fmt.Errorf("row %d: approved facts need canonical_key and period", line)
			}
		}
		result, err := tx.ExecContext(ctx, `UPDATE financial_facts SET status = ?, canonical_key = ?, period = ?,
			normalized_value = ?, currency = ?, unit = ?, reporting_basis = ?, audited_status = ?, review_note = ?
			WHERE id = ?`, status, get(row, "canonical_key"), get(row, "period"), value,
			get(row, "currency"), get(row, "unit"), nonempty(get(row, "reporting_basis"), "unknown"),
			nonempty(get(row, "audited_status"), "unknown"), get(row, "review_note"), id)
		if err != nil {
			return 0, fmt.Errorf("update candidate %q: %w", id, err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("check candidate %q update: %w", id, err)
		}
		if changed == 0 {
			return 0, fmt.Errorf("candidate %q was not found", id)
		}
		count++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit review: %w", err)
	}
	return count, nil
}

func nonempty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
