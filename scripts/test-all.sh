#!/bin/bash
# Comprehensive test script for linkpearl
# This script runs all tests, linters, and cross-platform builds

set -e

echo "=== Running Comprehensive Tests for Linkpearl ==="
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✓ $2${NC}"
    else
        echo -e "${RED}✗ $2${NC}"
        exit 1
    fi
}

# 1. Check formatting
echo "=== Checking Go Formatting ==="
if [ -z "$(gofmt -s -l .)" ]; then
    print_status 0 "All files are properly formatted"
else
    echo -e "${RED}The following files need formatting:${NC}"
    gofmt -s -l .
    echo -e "${YELLOW}Run 'gofmt -s -w .' to fix${NC}"
    exit 1
fi
echo

# 2. Run go vet
echo "=== Running go vet ==="
go vet ./...
print_status $? "go vet passed"
echo

# 3. Run golangci-lint if available
echo "=== Running golangci-lint ==="
if command -v golangci-lint &> /dev/null; then
    golangci-lint run ./...
    print_status $? "golangci-lint passed"
else
    echo -e "${YELLOW}golangci-lint not installed, skipping${NC}"
fi
echo

# 4. Run unit tests
echo "=== Running Unit Tests ==="
go test ./...
print_status $? "Unit tests passed"
echo

# 5. Run tests with race detection
echo "=== Running Race Detection ==="
go test -race ./...
print_status $? "Race detection passed"
echo

# 6. Run integration tests
echo "=== Running Integration Tests ==="
go test -tags=integration ./...
print_status $? "Integration tests passed"
echo

# 7. Check test coverage
echo "=== Checking Test Coverage ==="
go test -cover ./... | grep -E "ok|FAIL" | awk '{
    if ($3 == "coverage:") {
        coverage = substr($4, 1, length($4)-1)
        if (coverage < 50) {
            printf "\033[1;33m%-50s %s\033[0m\n", $2, $4
        } else {
            printf "\033[0;32m%-50s %s\033[0m\n", $2, $4
        }
    } else {
        print $0
    }
}'
echo

# 8. Cross-platform builds and test compilation
echo "=== Testing Cross-Platform Builds ==="
platforms=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
    "linux/386"
)

# Test binary builds
echo "Testing binary builds..."
for platform in "${platforms[@]}"; do
    IFS='/' read -r os arch <<< "$platform"
    echo -n "  Building binary for $os/$arch... "
    if GOOS=$os GOARCH=$arch go build -o /dev/null ./cmd/linkpearl 2>/dev/null; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC}"
        echo -e "${RED}Build failed for $os/$arch${NC}"
        GOOS=$os GOARCH=$arch go build ./cmd/linkpearl
        exit 1
    fi
done

# Test compilation for platform-specific code
echo "Testing platform-specific test compilation..."
for platform in "${platforms[@]}"; do
    IFS='/' read -r os arch <<< "$platform"
    echo -n "  Compiling tests for $os/$arch... "
    if GOOS=$os GOARCH=$arch go test -c ./pkg/... 2>/dev/null; then
        echo -e "${GREEN}✓${NC}"
        # Clean up test binaries
        rm -f *.test
    else
        echo -e "${RED}✗${NC}"
        echo -e "${RED}Test compilation failed for $os/$arch${NC}"
        GOOS=$os GOARCH=$arch go test -c ./pkg/clipboard/...
        exit 1
    fi
done

print_status 0 "All cross-platform builds and test compilations succeeded"
echo

# 9. Nix builds (if available)
if command -v nix &> /dev/null; then
    echo "=== Testing Nix Builds ==="
    
    # Get current system
    CURRENT_SYSTEM=$(nix eval --raw --impure --expr builtins.currentSystem 2>/dev/null)
    if [ -z "$CURRENT_SYSTEM" ]; then
        echo -e "${YELLOW}Could not determine current nix system, skipping nix builds${NC}"
    else
        echo -n "Building for current system ($CURRENT_SYSTEM)... "
        if nix build .#packages.${CURRENT_SYSTEM}.default -L --no-link 2>/dev/null; then
            echo -e "${GREEN}✓${NC}"
            print_status 0 "Nix build succeeded for $CURRENT_SYSTEM"
        else
            echo -e "${RED}✗${NC}"
            echo -e "${RED}Nix build failed for $CURRENT_SYSTEM${NC}"
            exit 1
        fi
    fi
    echo
else
    echo -e "${YELLOW}=== Skipping Nix Builds (nix not installed) ===${NC}"
    echo
fi

# 10. Check for common issues
echo "=== Checking for Common Issues ==="

# Check for TODO/FIXME comments
todo_count=$(grep -r "TODO\|FIXME" --include="*.go" . 2>/dev/null | wc -l)
if [ $todo_count -gt 0 ]; then
    echo -e "${YELLOW}Found $todo_count TODO/FIXME comments${NC}"
fi

# Check for fmt.Print* statements (should use logger)
fmt_count=$(grep -r "fmt\.Print" --include="*.go" . 2>/dev/null | grep -v "_test.go" | wc -l)
if [ $fmt_count -gt 0 ]; then
    echo -e "${YELLOW}Found $fmt_count fmt.Print* statements (consider using logger)${NC}"
fi

echo
echo -e "${GREEN}=== All Checks Passed! ===${NC}"
echo "Your code is ready for commit."