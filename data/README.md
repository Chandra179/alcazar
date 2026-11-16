# BBCA / BCA financial reports

Collected on 2026-11-15 from BCA and OJK pages. Ticker: BBCA (PT Bank Central Asia Tbk).

## Files

- `annual_reports/` — 23 annual-report PDFs for FY2002–FY2025, excluding a standalone FY2007 report.
- `audited_statements/` — BCA audited financial-statement PDFs for FY2018–FY2025.
- `2026_interim/` — unaudited consolidated and published financial reports for March, June, and September 2026, plus the October 2026 monthly publication.
- `financial_data_archives/` — September 2026 capital and risk exposure publication in PDF and XLSX. This XLSX covers capital/risk disclosures, not the full annual financial statements.
- `manifest.csv` — local filenames, reporting periods, document types, and direct source URLs.
- `extracted/` — generated page-by-page text from PDFs, written by the importer and ignored by Git.
- `review/` — generated review CSV exports, also ignored by Git.

The PDFs contain the primary statements, notes, auditor opinions, and/or annual-report context. Most reports through 2013 are Indonesian-language; later annual reports and statements are English or bilingual.

## Coverage and gaps

- FY1998–FY2000: no separate annual report or complete audited statement was located. BCA’s FY2002 report includes five-year financial highlights covering FY1998–FY2002.
- FY2001–FY2002: audited statements are included in the FY2002 annual report.
- FY2003–FY2006 and FY2008–FY2025: annual-report PDFs are present. A standalone FY2007 annual-report PDF was not retrievable; the FY2008 report includes comparative FY2007 statements.
- FY2026: no year-end audited report exists yet. The latest available reports collected here are Q3 (September) statements and the October monthly publication.
- OJK says its historical DPI financial data are XLS files compressed in ZIP archives. I found official archive entries for 2013–2016. Direct download attempts for the 2013 and 2016 ZIP attachments returned an HTML “Request Rejected” page from OJK, so no XLS workbooks are included; the 2014–2015 archive attachments are listed on OJK but were not retrieved in this pass. OJK’s current DPI page lists DPI through 2018; its page says 2019 onward is available through bank-specific publication reports.
- The only standalone Excel workbook downloaded is BCA’s September 2026 capital/risk exposure XLSX. I did not find a downloadable XLSX for the full annual financial statements.
- The 1999/2000 statement copy on Studylibid remains a provisional source lead and is not in this directory; it is recorded as a coverage gap until a copy is acquired and cross-checked.

## Import and review

Run the ingestion container from the repository root:

```bash
make bbca-import
make bbca-review
```

Use `REVIEW_FILE=data/review/your-queue.csv` with both `make bbca-review` and `make bbca-approve` to keep multiple exports. Set `REVIEW_PERIOD=2025` or `REVIEW_DOCUMENT=audited_statements/BCA_audited_2025.pdf` to narrow an export. Edit only the rows you want to approve or reject. Rows left `pending` are skipped. Use decimal numbers without grouping separators in `normalized_value` and keep `candidate_revision` unedited. When correcting a period, clear the derived date columns or update them to match. A flow with an unknown start date requires an explicit January 1 start to be approved as year to date. A review CSV exported before revision checking was added needs to be exported again.

```bash
make bbca-approve
make run
```

The importer keeps source evidence and generates pending facts from recognized statement tables. It records the statement, row, and column alongside the PDF page. The XLSX reader records the sheet and cell. The primary statement parser distinguishes group earnings, earnings attributable to the parent, group equity, and parent equity. A few historical PDFs provide highlights only, and questionable OCR values may still need review at the source.

The document hash and parser version control replay. The importer reuses cached PDF pages. On a parser change or `--force`, it replaces pending candidates and retains approved and rejected decisions. The CLI checks the revision before applying a decision and records the original observation in SQLite. `make bbca-catalog` updates the persisted Atlas catalog independently, and `make run` reads that catalog from SQLite. Set the same `SQLITE_DSN` for all Make targets when using a second database. For the container targets, use a path relative to the Alcazar root so the database resolves in the mounted workspace.

API routes:

- `GET /api/v1/companies/BBCA/coverage`
- `GET /api/v1/companies/BBCA/periods`
- `GET /api/v1/companies/BBCA/facts?period=2025`
- `GET /api/v1/companies/BBCA/metrics?period=2025`
- `GET /api/v1/companies/BBCA/prompts?period=2025`

Metrics use approved facts only and require compatible, known reporting basis, audit status, currency, and unit metadata. ROA and ROE average balances are derived from approved current- and prior-period balances, and the metric response includes both source facts. The capital-expenditure burden uses a trailing ten fiscal-year window for annual periods; interim requests remain `requires_multi_period`, and the response links every annual input to its source. Formula statuses distinguish `missing_inputs`, `conflict`, `not_applicable`, `requires_external_data`, `requires_multi_period`, and `requires_manual_estimate`; an unknown formula ID remains `unsupported`. Coverage reports pending and approved facts by period and identifies missing consolidated statement inputs. Review prompts use selected fact keys and statement disclosures with source references. A prompt can include pending evidence and labels its review status.

## Official source pages

- [BCA annual reports](https://www.bca.co.id/en/tentang-bca/hubungan-investor/laporan-presentasi/laporan-tahunan)
- [BCA financial reports](https://www.bca.co.id/en/tentang-bca/Hubungan-Investor/laporan-presentasi/Laporan-Keuangan)
- [OJK historical DPI archive](https://ojk.go.id/id/kanal/perbankan/data-dan-statistik/direktori-perbankan-indonesia-baru/default.aspx)
- [OJK 2013 financial-data archive entry](https://ojk.go.id/id/kanal/perbankan/data-dan-statistik/Direktori-Perbankan-Indonesia-Baru/Pages/Laporan-Keuangan-Bank-2013.aspx)
- [OJK 2014 financial-data archive entry](https://ojk.go.id/id/kanal/perbankan/data-dan-statistik/Direktori-Perbankan-Indonesia-Baru/Pages/Laporan-Keuangan-DPI-2014.aspx)
- [OJK 2015 financial-data archive entry](https://ojk.go.id/id/kanal/perbankan/data-dan-statistik/Direktori-Perbankan-Indonesia-Baru/Pages/Laporan-Keuangan-Bank-DPI-2015.aspx)
- [OJK 2016 financial-data archive entry](https://ojk.go.id/id/kanal/perbankan/data-dan-statistik/Direktori-Perbankan-Indonesia-Baru/Pages/Laporan-Keuangan-Bank-DPI-2016.aspx)
