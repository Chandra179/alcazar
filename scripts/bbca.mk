COMPANY ?= bbca
DATA_DIR ?= data/$(COMPANY)
REVIEW_FILE ?= $(DATA_DIR)/review/review.csv
REVIEW_PERIOD ?=
REVIEW_DOCUMENT ?=
ANALYSIS_DB ?= $(DATA_DIR)/analysis/analysis.db
ANALYSIS_FORMAT ?= markdown
ANALYSIS_EVIDENCE ?= $(DATA_DIR)/analysis/evidence.yaml
ANALYSIS_BANK_CATALOG ?= bank-analysis.yaml
ANALYSIS_BANK_EVIDENCE ?= $(if $(ANALYSIS_BANK_CATALOG),$(DATA_DIR)/analysis/bank-analysis.yaml)
ANALYSIS_BANK_SCENARIOS ?= $(if $(ANALYSIS_BANK_CATALOG),$(DATA_DIR)/analysis/valuation-scenarios.yaml)
ANALYSIS_PDF ?= output/pdf/$(COMPANY)-yearly.pdf
PYTHON ?= $(or $(wildcard .venv-bbca-pdf/bin/python),python3)

.PHONY: bbca-import bbca-catalog bbca-review bbca-approve bbca-analysis bbca-analysis-pdf

bbca-import:
	PODMAN="$(PODMAN)" ./scripts/bbca.sh import --data "$(DATA_DIR)" --db "$(SQLITE_DSN)"

bbca-catalog:
	PODMAN="$(PODMAN)" ./scripts/bbca.sh catalog --db "$(SQLITE_DSN)"

bbca-review:
	PODMAN="$(PODMAN)" ./scripts/bbca.sh review --db "$(SQLITE_DSN)" --out "$(REVIEW_FILE)" --period "$(REVIEW_PERIOD)" --document "$(REVIEW_DOCUMENT)"

bbca-approve:
	PODMAN="$(PODMAN)" ./scripts/bbca.sh approve --db "$(SQLITE_DSN)" --file "$(REVIEW_FILE)"

bbca-analysis-pdf: override ANALYSIS_FORMAT = pdf
bbca-analysis bbca-analysis-pdf:
	@set -eu; \
	set -- --db "$(ANALYSIS_DB)" --evidence "$(ANALYSIS_EVIDENCE)" \
		$(if $(ANALYSIS_BANK_CATALOG),--bank-catalog "$(ANALYSIS_BANK_CATALOG)") \
		$(if $(ANALYSIS_BANK_EVIDENCE),--bank-evidence "$(ANALYSIS_BANK_EVIDENCE)") \
		$(if $(ANALYSIS_BANK_SCENARIOS),--bank-scenarios "$(ANALYSIS_BANK_SCENARIOS)"); \
	if [ "$(ANALYSIS_FORMAT)" = pdf ]; then \
		input=$$(mktemp --suffix=.html); \
		trap 'rm -f "$$input"' EXIT; \
		$(GO) run ./server/cmd/bbca analysis "$$@" --format html > "$$input"; \
		"$(PYTHON)" scripts/bbca_analysis_pdf.py "$$input" "$(ANALYSIS_PDF)"; \
	else \
		$(GO) run ./server/cmd/bbca analysis "$$@" --format "$(ANALYSIS_FORMAT)"; \
	fi
