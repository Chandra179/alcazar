# BBCA / BCA financial reports

PT Bank Central Asia Tbk (BBCA) source archive. The reviewed snapshot includes disclosures available through 18 November 2026.

## Files

- `annual_reports/` — 23 annual-report PDFs for FY2002–FY2025, excluding a standalone FY2007 report.
- `audited_statements/` — BCA audited financial-statement PDFs for FY2018–FY2025.
- `2026_interim/` — unaudited consolidated and published financial reports for March, June, and September 2026, plus the October 2025 and 2026 bank-only publications and archived 2025 quarterly comparatives.
- `financial_data_archives/` — September 2026 capital and risk exposure publication in PDF and XLSX. This XLSX covers capital/risk disclosures, not the full annual financial statements.
- `subsidiary_statements/` — BCA Life and BCA Insurance standalone audited FY2025 summary statements with restated FY2024 comparatives.
- `manifest.csv` — local filenames, reporting periods, document types, and direct source URLs.
- `extracted/` — generated page-by-page text from PDFs, written by the importer and ignored by Git.
- `review/` — generated review CSV exports, also ignored by Git.

The PDFs contain the primary statements, notes, auditor opinions, and/or annual-report context. Most reports through 2013 are Indonesian-language; later annual reports and statements are English or bilingual.

## Coverage and gaps

- BCA’s FY2002 highlights cover FY1998–FY2002. The reviewed FY1999 and FY2000 archive supplements the highlights with audited balances, cash flows, tax and share data. The two FY2000 periods are recorded separately around the October quasi-reorganization. A standalone audited FY1998 statement and FY1997 opening balances remain unverified.
- FY2001–FY2002: audited statements are included in the FY2002 annual report.
- FY2003–FY2006 and FY2008–FY2025: annual-report PDFs are present. The FY2008 report includes FY2007 comparatives. The reviewed FY2007 archival copy supplies the as-filed parent equity for FY2007 ROE, and FY2008 ROE uses its restated opening equity.
- FY2026: no year-end audited report exists yet. The latest available reports collected here are Q3 (September) statements and the October monthly publication.
- OJK says its historical DPI financial data are XLS files compressed in ZIP archives. I found official archive entries for 2013–2016. Direct download attempts for the 2013 and 2016 ZIP attachments returned an HTML “Request Rejected” page from OJK, so no XLS workbooks are included; the 2014–2015 archive attachments are listed on OJK but were not retrieved in this pass. OJK’s current DPI page lists DPI through 2018; its page says 2019 onward is available through bank-specific publication reports.
- The only standalone Excel workbook downloaded is BCA’s September 2026 capital/risk exposure XLSX. I did not find a downloadable XLSX for the full annual financial statements.
- Historical quotes, corporate actions, national inflation and archived statements are linked from the evidence register. Market series share the October 2021 split basis. Cash yield uses the calendar payment date, and the holding-period return uses dividend entitlement.

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

The document hash and parser version control replay. The importer reuses cached PDF pages. On a parser change or `--force`, it replaces pending candidates and retains approved and rejected decisions. The CLI checks the revision before applying a decision and records the original observation in SQLite. `make bbca-catalog` updates the persisted catalog independently, and `make run` reads that catalog from SQLite. Set the same `SQLITE_DSN` for all Make targets when using a second database. For the container targets, use a path relative to the Alcazar root so the database resolves in the mounted workspace.

API routes:

- `GET /api/v1/companies/BBCA/coverage`
- `GET /api/v1/companies/BBCA/periods`
- `GET /api/v1/companies/BBCA/facts?period=2025`
- `GET /api/v1/companies/BBCA/metrics?period=2025`
- `GET /api/v1/companies/BBCA/prompts?period=2025`

Metrics use approved facts only and require compatible, known reporting basis, audit status, currency, and unit metadata. ROA and ROE average balances are derived from approved current- and prior-period balances, and the metric response includes both source facts. The capital-expenditure burden uses a trailing ten fiscal-year window for annual periods; interim requests remain `requires_multi_period`, and the response links every annual input to its source. Formula statuses distinguish `missing_inputs`, `conflict`, `not_applicable`, `requires_external_data`, `requires_multi_period`, and `requires_manual_estimate`; an unknown formula ID remains `unsupported`. Coverage reports pending and approved facts by period and identifies missing consolidated statement inputs. Review prompts use selected fact keys and statement disclosures with source references. A prompt can include pending evidence and labels its review status.

## FY2025 analysis report

Open [`analysis/bbca-fy2025.md`](analysis/bbca-fy2025.md) for the brief and [`analysis/bbca-fy2025-evidence.md`](analysis/bbca-fy2025-evidence.md) for full history, source coverage, the 101 rule assessments, calculations and the grouped review queue. [`analysis/bbca-fy2025.yaml`](analysis/bbca-fy2025.yaml) records the qualitative assessments, supplemental observations, and the catalog and approved-fact hashes. The decision CSV records approved extracted statement facts. The evidence YAML includes source-located statement inputs for pages or rows the parser missed. The report uses those reviewed inputs only when an approved candidate for that scope is absent. Review decisions use the original candidate revisions, so regenerating candidates with a different parser or OCR result may require a fresh review.

Create the dedicated database once from the repository root. Run the approval step against a fresh analysis database. Reapplying the same decisions to an already reviewed database reports stale candidate revisions.

```sh
make bbca-import SQLITE_DSN=data/analysis/bbca-analysis.db
make bbca-approve SQLITE_DSN=data/analysis/bbca-analysis.db \
  REVIEW_FILE=data/analysis/bbca-fy2025-review.csv
```

If `lux.db` already contains the matching initial 40-document import and has no reviewed facts, it can supply the initial candidates. With `sqlite3` installed, run `sqlite3 -readonly lux.db '.backup data/analysis/bbca-analysis.db'` to seed a fresh analysis database. Run the import against that database to pick up the additional manifest files, then apply the decision CSV. For a cached import with only text-layer additions, the native import accepts `--skip-ocr` when Tesseract is unavailable.

```sh
make bbca-analysis
make bbca-analysis ANALYSIS_VIEW=evidence
make bbca-analysis ANALYSIS_VIEW=yearly
make bbca-analysis ANALYSIS_FORMAT=json > /tmp/bbca-fy2025.json
```

The Markdown and JSON commands write to stdout and open the database read-only. The default catalog is the repository-root `fundamental-analysis.yaml`. Native CLI invocations also accept `--catalog` or `BBCA_CATALOG`. The report requires the catalog in SQLite, the supplied YAML, and its reviewed evidence hash to agree. A change to a supported formula requires a matching code change. Numeric narrative and rule assessments are pinned to the approved fact set, including its source references. To update the snapshot, recheck assessments and source observations against the new facts, then record the output of `make bbca-analysis ANALYSIS_FORMAT=facts-hash` as `facts_hash` in the evidence file. `ANALYSIS_DB` and `ANALYSIS_EVIDENCE` override the report inputs.

To upgrade a previously reviewed analysis database, rebuild the dedicated snapshot from the unreviewed import and reapply the updated decision CSV. Keep the previous database as a backup until the new report succeeds. The restated opening cells are stored as alternate facts for the audited comparison so the as-filed historical totals retain their own source basis. The consolidated KM1 table links reviewed values to both the capital PDF and the matching workbook cells. The JSON carries all available report views alongside the existing facts, observations, rule assessments and metric fields. The evidence view combines equivalent candidate values within compatible period, basis, audit status and units. Source and status remain visible for each candidate. The grouping does not approve pending facts.

The yearly report summarizes each year in ordinary language, with business changes, stock performance, and what each meant for shareholders. The English [yearly report](analysis/bbca-fy2025-yearly.md) uses narrative and source references from the reviewed evidence YAML. The report calculates price changes from split-adjusted closes and reuses the reviewed holding-period return. Year-to-date returns require a reviewed period-end close and dividends earned through the same date on that share basis.

### PDF export

The PDF uses the same report sections as Markdown and has clickable source links. Install ReportLab once in a Python environment if needed.

```sh
python3 -m venv .venv-bbca-pdf
.venv-bbca-pdf/bin/python -m pip install reportlab==4.4.9
make bbca-analysis-pdf PYTHON=.venv-bbca-pdf/bin/python
make bbca-analysis-pdf ANALYSIS_VIEW=evidence PYTHON=.venv-bbca-pdf/bin/python
make bbca-analysis-pdf ANALYSIS_VIEW=yearly PYTHON=.venv-bbca-pdf/bin/python
```

If `python3` already has ReportLab, run `make bbca-analysis-pdf` or `make bbca-analysis ANALYSIS_FORMAT=pdf`. The default destination for the brief is `output/pdf/bbca-fy2025.pdf`. The evidence and yearly views add `-evidence` and `-yearly` to the filename. Set `ANALYSIS_PDF=/path/to/report.pdf` to choose another file. Use `PYTHON` to select an existing interpreter with ReportLab. PDF export uses the same dedicated analysis database and evidence as Markdown and JSON.

## Official source pages

- [BCA annual reports](https://www.bca.co.id/en/tentang-bca/hubungan-investor/laporan-presentasi/laporan-tahunan)
- [BCA financial reports](https://www.bca.co.id/en/tentang-bca/Hubungan-Investor/laporan-presentasi/Laporan-Keuangan)
- [OJK historical DPI archive](https://ojk.go.id/id/kanal/perbankan/data-dan-statistik/direktori-perbankan-indonesia-baru/default.aspx)
- [OJK 2013 financial-data archive entry](https://ojk.go.id/id/kanal/perbankan/data-dan-statistik/Direktori-Perbankan-Indonesia-Baru/Pages/Laporan-Keuangan-Bank-2013.aspx)
- [OJK 2014 financial-data archive entry](https://ojk.go.id/id/kanal/perbankan/data-dan-statistik/Direktori-Perbankan-Indonesia-Baru/Pages/Laporan-Keuangan-DPI-2014.aspx)
- [OJK 2015 financial-data archive entry](https://ojk.go.id/id/kanal/perbankan/data-dan-statistik/Direktori-Perbankan-Indonesia-Baru/Pages/Laporan-Keuangan-Bank-DPI-2015.aspx)
- [OJK 2016 financial-data archive entry](https://ojk.go.id/id/kanal/perbankan/data-dan-statistik/Direktori-Perbankan-Indonesia-Baru/Pages/Laporan-Keuangan-Bank-DPI-2016.aspx)
