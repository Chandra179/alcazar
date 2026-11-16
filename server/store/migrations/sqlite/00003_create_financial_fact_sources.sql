-- +goose Up
CREATE TABLE IF NOT EXISTS financial_fact_sources (
    fact_id        TEXT PRIMARY KEY REFERENCES financial_facts(id) ON DELETE CASCADE,
    source_locator TEXT NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE IF EXISTS financial_fact_sources;
