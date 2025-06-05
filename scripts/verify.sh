#!/usr/bin/env bash
# verify.sh - Run quick verification checks for linkpearl (pre-commit)
# 
# This script runs essential quality checks:
# - Code formatting
# - Go vet
# - golangci-lint (if available)
# - Quick unit tests
#
# For comprehensive checks, use: make test-all
#
# Exit codes:
#   0 - All checks passed
#   1 - One or more checks failed

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "\n🔍 Running comprehensive verification...\n"

# Track if any checks fail
FAILED=0

# Check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Run each check
run_check() {
    local name=$1
    local command=$2
    
    echo -e "==> ${name}..."
    if eval "${command}"; then
        echo -e "${GREEN}✅ ${name} passed${NC}\n"
    else
        echo -e "${RED}❌ ${name} failed${NC}\n"
        FAILED=1
    fi
}

# Run essential checks
run_check "Formatting" "test -z \"$(gofmt -l -s .)\""
run_check "Go vet" "go vet ./..."

# Run golangci-lint if available
if command_exists "golangci-lint"; then
    run_check "Golangci-lint" "golangci-lint run"
else
    echo -e "${YELLOW}⚠ golangci-lint not found, skipping${NC}\n"
fi

# Run quick tests (no race detection for speed)
run_check "Quick tests" "go test -short ./..."

# Check for common issues
echo -e "==> Checking for common issues..."
# Check for fmt.Print usage, excluding nolint lines and comments
if grep -r "fmt\.Print" --include="*.go" . | grep -v "^scripts/" | grep -v "//nolint" | grep -v "//.*fmt\.Print" > /dev/null; then
    echo -e "${YELLOW}⚠ Found fmt.Print usage (use logger instead)${NC}"
    FAILED=1
fi

echo ""

# Final result
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✅ All verification checks passed!${NC}\n"
    exit 0
else
    echo -e "${RED}❌ Some checks failed. Run 'make fix' to auto-fix issues.${NC}"
    echo -e "${YELLOW}ℹ For comprehensive testing, run: make test-all${NC}\n"
    exit 1
fi