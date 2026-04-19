VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"
WEB_INSTALL_STAMP := web/node_modules/.install-stamp
WEB_BUILD_STAMP := web/.build-stamp
WEB_INSTALL_INPUTS := web/package.json web/package-lock.json
WEB_BUILD_CONFIGS := web/next.config.ts web/tsconfig.json web/postcss.config.mjs web/components.json
WEB_BUILD_INPUTS := $(WEB_INSTALL_STAMP) $(WEB_BUILD_CONFIGS) performance-budgets.json scripts/validate_frontend_budget.py
WEB_SOURCE_INPUTS := $(shell find web/src -type f 2>/dev/null) $(shell find web/public -type f 2>/dev/null)

.PHONY: all build run test clean dev web web-install perf-budget

all: build

# Frontend build
web-install: $(WEB_INSTALL_STAMP)

$(WEB_INSTALL_STAMP): $(WEB_INSTALL_INPUTS)
	cd web && npm ci
	touch $@

web: $(WEB_BUILD_STAMP)

$(WEB_BUILD_STAMP): $(WEB_BUILD_INPUTS) $(WEB_SOURCE_INPUTS)
	cd web && npm run build
	python3 scripts/validate_frontend_budget.py --budget performance-budgets.json --build-dir web/.next --summary --fail-on-violation
	touch $@

perf-budget:
	python3 scripts/validate_frontend_budget.py --budget performance-budgets.json --build-dir web/.next --summary --fail-on-violation

# Go build (depends on frontend)
build: web
	go build $(LDFLAGS) -o bin/grokforge ./cmd/grokforge

run: build
	./bin/grokforge

dev:
	rm -f $(WEB_BUILD_STAMP)
	$(MAKE) web
	go run $(LDFLAGS) ./cmd/grokforge

test:
	go test -race -v ./...

clean:
	rm -rf bin/ data/ web/out/ web/.next/ web/.build-stamp web/node_modules/.install-stamp
