#!/usr/bin/env bash
# install-tools.sh - Install required development tools for linkpearl

set -euo pipefail

# Track failures
FAILED_TOOLS=()

echo "Installing required Go development tools for linkpearl..."
echo ""

# Function to install a tool
install_tool() {
    local name=$1
    local package=$2
    
    echo -n "Installing $name... "
    if command -v "$name" &> /dev/null; then
        echo "already installed ✓"
        return 0
    fi
    
    if go install "$package" 2>/dev/null; then
        echo "✓"
    else
        echo "✗"
        FAILED_TOOLS+=("$name: $package")
        return 1
    fi
}

# Install all development tools
echo "Installing development tools:"
install_tool "golangci-lint" "github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
install_tool "goimports" "golang.org/x/tools/cmd/goimports@latest"
install_tool "misspell" "github.com/client9/misspell/cmd/misspell@latest"
install_tool "staticcheck" "honnef.co/go/tools/cmd/staticcheck@latest"
install_tool "gosec" "github.com/securego/gosec/v2/cmd/gosec@latest"
install_tool "ineffassign" "github.com/gordonklaus/ineffassign@latest"
install_tool "errcheck" "github.com/kisielk/errcheck@latest"

echo ""

# Check if PATH includes go/bin
if [[ ":$PATH:" != *":$HOME/go/bin:"* ]]; then
    echo "⚠️  Make sure $HOME/go/bin is in your PATH:"
    echo "  export PATH=\$PATH:\$HOME/go/bin"
    echo ""
fi

# Report results
if [ ${#FAILED_TOOLS[@]} -eq 0 ]; then
    echo "✅ All tools installed successfully!"
else
    echo "⚠️  Some tools failed to install:"
    for tool in "${FAILED_TOOLS[@]}"; do
        echo "  - $tool"
    done
    echo ""
    echo "You can try installing them manually or continue without them."
    exit 1
fi