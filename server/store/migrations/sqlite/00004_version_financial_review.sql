-- +goose Up
CREATE TABLE IF NOT EXISTS financial_document_parsers (
    document_id TEXT PRIMARY KEY REFERENCES financial_documents(id) ON DELETE CASCADE,
    version TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS financial_review_decisions (
    fact_id TEXT PRIMARY KEY REFERENCES financial_facts(id),
    observation TEXT NOT NULL,
    decision TEXT NOT NULL CHECK (decision IN ('approved', 'rejected')),
    reviewed_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- +goose Down
DROP TABLE IF EXISTS financial_review_decisions;
DROP TABLE IF EXISTS financial_document_parsers;
