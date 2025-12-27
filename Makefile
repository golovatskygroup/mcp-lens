# MCP Proxy - Reproducible checks
# Following Gleam toolchain conventions

.PHONY: all build test fmt fmt-check lint vet clean help check-context

# Default target: run all checks
all: fmt-check lint test build

## Build: Compile the project
build:
	@echo "==> Building project..."
	gleam build

## Test: Run the test suite
test:
	@echo "==> Running tests..."
	gleam test

## Fmt: Automatically format source code
fmt:
	@echo "==> Formatting code..."
	gleam format

## Fmt-check: Check if code is formatted without modifying files
fmt-check:
	@echo "==> Checking formatting..."
	gleam format --check src test

## Lint/Vet: Perform type-checking and static analysis
lint: vet
vet:
	@echo "==> Type checking and analysis..."
	gleam check

## Clean: Remove build artifacts
clean:
	@echo "==> Cleaning build directory..."
	rm -rf build

## Check-context: Identify large files or duplicates (Phase 1 scope reduction helper)
# This helps find .worktrees or stage* files mentioned in the task context
check-context:
	@echo "==> Analyzing project structure for context reduction..."
	@echo "Top 10 largest files:"
	@find . -type f -not -path '*/.git/*' -not -path '*/build/*' -exec du -sh {} + | sort -rh | head -n 10
	@echo "\nPotential duplicates or worktrees:"
	@find . -name ".worktrees" -o -name "stage*" -type d

## Help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'