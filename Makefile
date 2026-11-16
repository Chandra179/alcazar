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

.PHONY: all build run test lint fmt tidy verify migrate-up podman-build bbca-import bbca-catalog bbca-review bbca-approve \
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
