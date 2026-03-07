.PHONY: build run test clean dev web web-install perf-budget

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Frontend build
web-install:
	cd web && npm ci

web: web-install
	cd web && npm run build
	python3 scripts/validate_frontend_budget.py --budget performance-budgets.json --build-dir web/.next --summary --fail-on-violation

perf-budget:
	python3 scripts/validate_frontend_budget.py --budget performance-budgets.json --build-dir web/.next --summary --fail-on-violation

# Go build (depends on frontend)
build: web
	go build $(LDFLAGS) -o bin/grokforge ./cmd/grokforge

run: build
	./bin/grokforge

dev:
	go run $(LDFLAGS) ./cmd/grokforge

test:
	go test -v ./...

clean:
	rm -rf bin/ data/ web/out/ web/.next/
