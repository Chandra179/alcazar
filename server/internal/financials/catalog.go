package financials

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Chandra179/lux/server/store"
	"gopkg.in/yaml.v3"
)

type catalogFile struct {
	Entries []CatalogEntry `yaml:"entries"`
}

func LoadCatalog(path string) ([]CatalogEntry, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read catalog %q: %w", path, err)
	}
	var parsed catalogFile
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, "", fmt.Errorf("parse catalog %q: %w", path, err)
	}
	if len(parsed.Entries) == 0 {
		return nil, "", fmt.Errorf("catalog %q has no entries", path)
	}
	sum := sha256.Sum256(data)
	return parsed.Entries, hex.EncodeToString(sum[:]), nil
}

// Initialize installs the BBCA schema and records the current analysis catalog
// so API reads work before the first document import.
func Initialize(ctx context.Context, db *sql.DB, path string) error {
	if err := store.EnsureBBCAFinancialSchema(ctx, db); err != nil {
		return err
	}
	entries, hash, err := LoadCatalog(path)
	if err != nil {
		return err
	}
	return storeCatalog(ctx, db, entries, hash)
}

func storeCatalog(ctx context.Context, db *sql.DB, entries []CatalogEntry, hash string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin catalog import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM fundamental_entities`); err != nil {
		return fmt.Errorf("replace catalog entries: %w", err)
	}
	for _, entry := range entries {
		depends, err := json.Marshal(entry.DependsOn)
		if err != nil {
			return fmt.Errorf("encode dependencies for %q: %w", entry.ID, err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO fundamental_entities
			(id, type, description, formula, applies_when, depends_on, source, catalog_hash)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				type = excluded.type, description = excluded.description,
				formula = excluded.formula, applies_when = excluded.applies_when,
				depends_on = excluded.depends_on, source = excluded.source,
				catalog_hash = excluded.catalog_hash`,
			entry.ID, entry.Type, entry.Description, entry.Formula, entry.AppliesWhen,
			string(depends), entry.Source, hash)
		if err != nil {
			return fmt.Errorf("store catalog entry %q: %w", entry.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit catalog import: %w", err)
	}
	return nil
}

func listCatalog(db *sql.DB) ([]CatalogEntry, error) {
	rows, err := db.Query(`SELECT id, type, description, formula, applies_when, depends_on, source
		FROM fundamental_entities ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query catalog: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var entries []CatalogEntry
	for rows.Next() {
		var entry CatalogEntry
		var depends string
		if err := rows.Scan(&entry.ID, &entry.Type, &entry.Description, &entry.Formula, &entry.AppliesWhen, &depends, &entry.Source); err != nil {
			return nil, fmt.Errorf("scan catalog: %w", err)
		}
		if err := json.Unmarshal([]byte(depends), &entry.DependsOn); err != nil {
			return nil, fmt.Errorf("decode dependencies for %q: %w", entry.ID, err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate catalog: %w", err)
	}
	return entries, nil
}
