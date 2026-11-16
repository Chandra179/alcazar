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

//go:embed migrations/sqlite/00003_create_financial_fact_sources.sql
var bbcaFinancialFactSourcesSchema string

//go:embed migrations/sqlite/00004_version_financial_review.sql
var bbcaFinancialReviewSchema string

// EnsureBBCAFinancialSchema installs the BBCA tables. The migration is
// idempotent so both the importer and API can start against a fresh SQLite DB.
func EnsureBBCAFinancialSchema(ctx context.Context, db *sql.DB) error {
	for _, migration := range []string{bbcaFinancialSchema, bbcaFinancialFactSourcesSchema, bbcaFinancialReviewSchema} {
		up := strings.SplitN(migration, "-- +goose Down", 2)[0]
		if _, err := db.ExecContext(ctx, up); err != nil {
			return fmt.Errorf("ensure BBCA financial schema: %w", err)
		}
	}
	return nil
}
