BIN := bin/outpost
SPEC ?= workload.yaml

.PHONY: all build test test-operator vet fmt fmt-check demo validate clean tools

all: build

build: ## Build the outpost CLI
	go build -o $(BIN) .

test: ## Run unit, golden, and validator tests (both modules)
	go test ./...
	cd operator && go test ./...

test-operator: ## Run only the operator module tests
	cd operator && go test ./...

vet: ## go vet both modules
	go vet ./...
	cd operator && go vet ./...

fmt: ## Format both modules
	gofmt -w .
	cd operator && gofmt -w .

fmt-check: ## Fail if any Go file is not gofmt-clean
	@test -z "$$(gofmt -l . operator)" || { echo "gofmt needed:"; gofmt -l . operator; exit 1; }

demo: build ## Render all three targets from the example spec into _demo/
	rm -rf _demo
	$(BIN) render --spec $(SPEC) --out _demo
	@echo
	@echo "Rendered targets under _demo/:"
	@find _demo -type f | sort

validate: ## Render and run every per-target validator (via the test suite)
	go test ./internal/render/ -run 'TestOnPremHelm|TestCloudTerraform|TestAirGap' -v

clean: ## Remove build and demo output
	rm -rf bin _demo out

tools: ## Print the external validators the test suite uses
	@echo "Required for full validation: go, helm, kubeconform, terraform, shellcheck"
	@command -v go helm kubeconform terraform shellcheck 2>/dev/null || true
