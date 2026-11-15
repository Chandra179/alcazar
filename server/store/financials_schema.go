package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
)

//go:embed migrations/sqlite/00002_create_bbca_financials.sql
var bbcaFinancialSchema string

// EnsureBBCAFinancialSchema installs the BBCA tables. The migration is
// idempotent so both the importer and API can start against a fresh SQLite DB.
func EnsureBBCAFinancialSchema(ctx context.Context, db *sql.DB) error {
	upMigration := strings.SplitN(bbcaFinancialSchema, "-- +goose Down", 2)[0]
	if _, err := db.ExecContext(ctx, upMigration); err != nil {
		return fmt.Errorf("ensure BBCA financial schema: %w", err)
	}
	return nil
}
