# Financial Data Directory

Source archives, extracted text, review exports, and analysis snapshots are organized per company under `data/<company>/` (for example `data/bbca/`).

## Layout

Each company directory follows the same structure:

```text
data/<company>/
├── README.md                 # Company-specific coverage notes and sources
├── manifest.csv              # Local filenames, periods, document types, source URLs
├── annual_reports/           # Annual report PDFs
├── audited_statements/       # Audited financial statement PDFs
├── 2026_interim/             # Interim / monthly disclosures (or other period folders)
├── financial_data_archives/  # Capital/risk or supplemental workbooks & PDFs
├── subsidiary_statements/    # Subsidiary statements
├── analysis/
│   ├── analysis.db           # Reviewed SQLite analysis database (gitignored)
│   ├── evidence.yaml         # Reviewed evidence & narrative snapshot
│   ├── bank-analysis.yaml    # Bank-specific evidence (optional)
│   ├── valuation-scenarios.yaml # Valuation & stress assumptions (optional)
│   ├── review.csv            # Approved fact decisions
│   └── yearly.md             # Generated Markdown yearly report
├── extracted/                # Cached PDF page text (gitignored)
└── review/                   # Pending review CSV exports (gitignored)
```

## Switching companies

Pass `COMPANY=<ticker>` to Make targets (defaults to `bbca`):

```sh
make bbca-analysis-pdf COMPANY=bbca
```

Generated PDFs are written to `output/pdf/<company>-yearly.pdf` by default.
