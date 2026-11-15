-- +goose Up
CREATE TABLE IF NOT EXISTS financial_documents (
    id           TEXT PRIMARY KEY,
    sha256       TEXT NOT NULL UNIQUE,
    local_path   TEXT NOT NULL,
    source_url   TEXT NOT NULL DEFAULT '',
    period       TEXT NOT NULL DEFAULT '',
    document_type TEXT NOT NULL DEFAULT '',
    format       TEXT NOT NULL,
    audited      INTEGER NOT NULL DEFAULT 0,
    extraction_status TEXT NOT NULL DEFAULT 'pending'
                      CHECK (extraction_status IN ('pending', 'processed')),
    imported_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS financial_pages (
    document_id       TEXT NOT NULL REFERENCES financial_documents(id) ON DELETE CASCADE,
    page_number       INTEGER NOT NULL,
    extraction_method TEXT NOT NULL,
    text              TEXT NOT NULL,
    PRIMARY KEY (document_id, page_number)
);

CREATE TABLE IF NOT EXISTS fundamental_entities (
    id           TEXT PRIMARY KEY,
    type         TEXT NOT NULL,
    description  TEXT NOT NULL,
    formula      TEXT NOT NULL DEFAULT '',
    applies_when TEXT NOT NULL DEFAULT '',
    depends_on   TEXT NOT NULL DEFAULT '[]',
    source       TEXT NOT NULL DEFAULT '',
    catalog_hash TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS financial_facts (
    id                TEXT PRIMARY KEY,
    document_id       TEXT NOT NULL REFERENCES financial_documents(id) ON DELETE CASCADE,
    page_number       INTEGER NOT NULL,
    period            TEXT NOT NULL DEFAULT '',
    period_start      TEXT NOT NULL DEFAULT '',
    period_end        TEXT NOT NULL DEFAULT '',
    period_type       TEXT NOT NULL DEFAULT '',
    raw_label         TEXT NOT NULL,
    raw_value         TEXT NOT NULL,
    evidence_text     TEXT NOT NULL,
    canonical_key     TEXT NOT NULL DEFAULT '',
    normalized_value  TEXT NOT NULL DEFAULT '',
    currency          TEXT NOT NULL DEFAULT '',
    unit              TEXT NOT NULL DEFAULT '',
    reporting_basis   TEXT NOT NULL DEFAULT 'unknown',
    audited_status    TEXT NOT NULL DEFAULT 'unknown',
    extraction_method TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'approved', 'rejected')),
    review_note       TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_financial_facts_period_status
    ON financial_facts (period, status, canonical_key);
CREATE INDEX IF NOT EXISTS idx_financial_facts_document_page
    ON financial_facts (document_id, page_number);

CREATE TABLE IF NOT EXISTS financial_coverage_gaps (
    id          TEXT PRIMARY KEY,
    period      TEXT NOT NULL,
    description TEXT NOT NULL,
    source_url  TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'known_gap'
);

INSERT OR IGNORE INTO financial_coverage_gaps (id, period, description, source_url) VALUES
    ('fy1998-full-statements', '1998', 'Only FY2002 five-year highlights located; no full standalone FY1998 audited statements.', 'https://www.bca.co.id/-/media/Feature/Report/File/S8/Laporan-Tahunan/AR-2002.pdf'),
    ('fy1999-local-copy', '1999', 'A third-party copy of FY1999 statements was located but is not present in the local data folder.', 'https://studylibid.com/doc/1976703/pt-bank-central-asia-tbk-dan-anak-perusahaan'),
    ('fy2000-local-copy', '2000', 'A third-party copy of the split FY2000 statements was located but is not present in the local data folder.', 'https://studylibid.com/doc/1976703/pt-bank-central-asia-tbk-dan-anak-perusahaan'),
    ('fy2007-annual-report', '2007', 'Standalone FY2007 annual report links are unavailable; comparative financial statements are included in FY2008.', 'https://www.sahamok.net/annual-report/laporan-tahunan-bank-bca/laporan-tahunan-bank-bca-2007/'),
    ('fy2026-year-end', '2026', 'FY2026 year-end audited statements are not available yet; current files cover interim periods through September and October monthly publication.', '');

-- +goose Down
DROP TABLE IF EXISTS financial_coverage_gaps;
DROP TABLE IF EXISTS financial_facts;
DROP TABLE IF EXISTS fundamental_entities;
DROP TABLE IF EXISTS financial_pages;
DROP TABLE IF EXISTS financial_documents;
