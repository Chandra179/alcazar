GO ?= go
GOLANGCI_LINT ?= golangci-lint
NPM ?= npm
PODMAN ?= podman

BINARY ?= bin/lux-server
APP_ENVIRONMENT ?= dev
SQLITE_DSN ?= lux.db
BADGER_DIR ?= lux
REVIEW_FILE ?= data/review/bbca-review.csv
REVIEW_PERIOD ?=
REVIEW_DOCUMENT ?=
ANALYSIS_DB ?= data/analysis/bbca-analysis.db
ANALYSIS_FORMAT ?= markdown
ANALYSIS_EVIDENCE ?= data/analysis/bbca-fy2025.yaml
ANALYSIS_BANK_CATALOG ?= bank-analysis.yaml
ANALYSIS_BANK_EVIDENCE ?= $(if $(ANALYSIS_BANK_CATALOG),data/analysis/bbca-bank-analysis.yaml)
ANALYSIS_BANK_SCENARIOS ?= $(if $(ANALYSIS_BANK_CATALOG),data/analysis/bbca-valuation-scenarios.yaml)
ANALYSIS_PDF ?= output/pdf/bbca-fy2025-yearly.pdf
PYTHON ?= python3

.PHONY: all build run test lint fmt tidy verify migrate-up podman-build bbca-import bbca-catalog bbca-review bbca-approve bbca-analysis bbca-analysis-pdf \
	web-install web-dev web-build web-lint web-typecheck

all: verify lint test build

build:
	@mkdir -p "$(dir $(BINARY))"
	CGO_ENABLED=0 $(GO) build -trimpath -o "$(BINARY)" ./server/cmd/example

run:
	APP_ENVIRONMENT="$(APP_ENVIRONMENT)" SQLITE_DSN="$(SQLITE_DSN)" BADGER_DIR="$(BADGER_DIR)" $(GO) run ./server/cmd/example

test:
	$(GO) test -short -race -count=1 ./...

lint:
	$(GOLANGCI_LINT) run ./...

fmt:
	gofmt -w $$(find server -type f -name '*.go' -print)

tidy:
	$(GO) mod tidy

verify:
	$(GO) mod verify

migrate-up:
	$(GO) run github.com/pressly/goose/v3/cmd/goose@latest -dir server/store/migrations/sqlite sqlite3 "$(SQLITE_DSN)" up

podman-build:
	$(PODMAN) build -t lux-server .

bbca-import:
	PODMAN="$(PODMAN)" ./scripts/bbca.sh import --db "$(SQLITE_DSN)"

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
		input=$$(mktemp); \
		trap 'rm -f "$$input"' EXIT; \
		$(GO) run ./server/cmd/bbca analysis "$$@" --format json > "$$input"; \
		"$(PYTHON)" scripts/bbca_analysis_pdf.py "$$input" "$(ANALYSIS_PDF)"; \
	else \
		$(GO) run ./server/cmd/bbca analysis "$$@" --format "$(ANALYSIS_FORMAT)"; \
	fi

web-install:
	$(NPM) --prefix web ci

web-dev:
	$(NPM) --prefix web run dev

web-build:
	$(NPM) --prefix web run build

web-lint:
	$(NPM) --prefix web run lint

web-typecheck:
	$(NPM) --prefix web run typecheck
