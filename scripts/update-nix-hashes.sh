#!/usr/bin/env bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Parse command line arguments
FORCE=false
while [[ $# -gt 0 ]]; do
    case $1 in
        -f|--force)
            FORCE=true
            shift
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            echo "Usage: $0 [-f|--force]"
            exit 1
            ;;
    esac
done

echo -e "${YELLOW}Updating Nix hashes for linkpearl...${NC}"

# Check if we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo -e "${RED}Error: Not in a git repository${NC}" >&2
    exit 1
fi

# Check for required tools
for tool in nix-prefetch-url nix-hash nix go sed; do
    if ! command -v "$tool" &> /dev/null; then
        echo -e "${RED}Error: Required tool '$tool' not found${NC}" >&2
        exit 1
    fi
done

# Check for uncommitted changes
if ! git diff-index --quiet HEAD --; then
    echo -e "${YELLOW}Warning: You have uncommitted changes. The hashes will be calculated for the current HEAD.${NC}"
    echo -e "${YELLOW}Consider committing your changes first.${NC}"
    if [[ "$FORCE" != "true" ]]; then
        read -p "Continue anyway? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            echo -e "${RED}Aborted.${NC}"
            exit 1
        fi
    fi
fi

# Get the current git commit SHA
CURRENT_SHA=$(git rev-parse HEAD)
echo -e "Current commit: ${GREEN}${CURRENT_SHA}${NC}"

# Function to calculate nix hash for a GitHub archive
calculate_github_hash() {
    local sha=$1
    local hash=$(nix-prefetch-url --unpack https://github.com/Veraticus/linkpearl/archive/${sha}.tar.gz 2>/dev/null)
    if [ -z "$hash" ]; then
        echo -e "${RED}Failed to calculate GitHub archive hash${NC}" >&2
        exit 1
    fi
    echo "$hash"
}

# Function to calculate vendor hash using nix to get SRI format for flake.nix
calculate_vendor_hash_flake() {
    # Try to build with a dummy hash to get the correct one
    local temp_flake=$(mktemp -d)
    trap "rm -rf $temp_flake" EXIT
    
    # Create a minimal flake.nix with dummy vendor hash
    cat > "$temp_flake/flake.nix" <<EOF
{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";
  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system: {
      packages.default = nixpkgs.legacyPackages.\${system}.buildGoModule {
        pname = "linkpearl";
        version = "test";
        src = ./.;
        vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
      };
    });
}
EOF
    
    # Copy go files
    cp go.mod go.sum "$temp_flake/" 2>/dev/null || {
        echo -e "${RED}Failed to copy go.mod/go.sum files${NC}" >&2
        exit 1
    }
    
    # Try to build and extract the correct hash from error message
    local output=$(cd "$temp_flake" && nix build .#packages.x86_64-linux.default -L 2>&1 || true)
    local vendor_hash=$(echo "$output" | grep -A1 "got:" | grep -oE 'sha256-[A-Za-z0-9+/=]+' | head -1)
    
    if [ -z "$vendor_hash" ]; then
        echo -e "${RED}Failed to calculate vendor hash${NC}" >&2
        exit 1
    fi
    
    echo "$vendor_hash"
}

# Function to calculate vendor hash for default.nix (base32 format)
calculate_vendor_hash_base32() {
    # Create a temporary directory
    local temp_dir=$(mktemp -d)
    trap "rm -rf $temp_dir" EXIT
    
    # Copy go.mod and go.sum to temp directory
    cp go.mod go.sum "$temp_dir/" 2>/dev/null || {
        echo -e "${RED}Failed to copy go.mod/go.sum files${NC}" >&2
        exit 1
    }
    
    # Run go mod vendor in temp directory (redirect stderr to avoid "no packages" warning)
    (cd "$temp_dir" && go mod vendor 2>/dev/null) || {
        echo -e "${RED}Failed to create vendor directory${NC}" >&2
        exit 1
    }
    
    # Calculate the vendor hash
    local vendor_hash=$(nix-hash --type sha256 --base32 "$temp_dir/vendor" 2>/dev/null)
    if [ -z "$vendor_hash" ]; then
        echo -e "${RED}Failed to calculate vendor hash${NC}" >&2
        exit 1
    fi
    
    echo "$vendor_hash"
}

# Calculate hashes
echo -e "${YELLOW}Calculating GitHub archive hash...${NC}"
GITHUB_HASH=$(calculate_github_hash "$CURRENT_SHA")
echo -e "GitHub archive hash: ${GREEN}${GITHUB_HASH}${NC}"

echo -e "${YELLOW}Calculating vendor hash...${NC}"
VENDOR_HASH_BASE32=$(calculate_vendor_hash_base32)
echo -e "Vendor hash (base32): ${GREEN}${VENDOR_HASH_BASE32}${NC}"

echo -e "${YELLOW}Calculating vendor hash for flake.nix...${NC}"
VENDOR_HASH_FLAKE=$(calculate_vendor_hash_flake)
echo -e "Vendor hash (SRI): ${GREEN}${VENDOR_HASH_FLAKE}${NC}"

# Update default.nix
echo -e "${YELLOW}Updating default.nix...${NC}"
sed -i.bak \
    -e "s/rev = \"[^\"]*\";/rev = \"${CURRENT_SHA}\";/" \
    -e "s/sha256 = \"sha256-[^\"]*\";/sha256 = \"sha256-${GITHUB_HASH}\";/" \
    -e "s/vendorHash = \"sha256-[^\"]*\";/vendorHash = \"sha256-${VENDOR_HASH_BASE32}\";/" \
    default.nix

# Update flake.nix vendor hash
echo -e "${YELLOW}Updating flake.nix...${NC}"
sed -i.bak \
    -e "s/vendorHash = \"sha256-[^\"]*\";/vendorHash = \"${VENDOR_HASH_FLAKE}\";/" \
    flake.nix

# Update README.md
echo -e "${YELLOW}Updating README.md...${NC}"
sed -i.bak \
    -e "s/rev = \"[a-f0-9]\{40\}\";/rev = \"${CURRENT_SHA}\";/" \
    -e "s/sha256 = \"sha256-[^\"]*\";/sha256 = \"sha256-${GITHUB_HASH}\";/" \
    README.md

# Clean up backup files
rm -f default.nix.bak flake.nix.bak README.md.bak

echo -e "${GREEN}✓ All Nix hashes updated successfully!${NC}"
echo -e "${YELLOW}Summary of changes:${NC}"
echo -e "  - Git revision: ${GREEN}${CURRENT_SHA}${NC}"
echo -e "  - GitHub hash: ${GREEN}sha256-${GITHUB_HASH}${NC}"
echo -e "  - Vendor hash (base32): ${GREEN}sha256-${VENDOR_HASH_BASE32}${NC}"
echo -e "  - Vendor hash (SRI): ${GREEN}${VENDOR_HASH_FLAKE}${NC}"