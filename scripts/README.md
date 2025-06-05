# Linkpearl Scripts

This directory contains utility scripts for the linkpearl project.

## update-nix-hashes.sh

This script automatically updates all Nix-related hashes in the project to match the current Git HEAD commit. It updates:

1. **Git revision**: Updates the commit SHA in `default.nix` and `README.md`
2. **GitHub archive hash**: Calculates and updates the sha256 hash for the GitHub archive
3. **Vendor hash**: Calculates and updates the Go vendor dependencies hash

### Usage

```bash
# Run interactively (will prompt if there are uncommitted changes)
make update-nix

# Force update without prompts
make update-nix ARGS=-f

# Or run directly
./scripts/update-nix-hashes.sh
./scripts/update-nix-hashes.sh --force
```

### What it does

1. Checks for required tools (`nix-prefetch-url`, `nix-hash`, `go`, `sed`)
2. Gets the current Git commit SHA
3. Calculates the GitHub archive hash using `nix-prefetch-url`
4. Creates a temporary vendor directory and calculates its hash
5. Updates all relevant files:
   - `default.nix`: Git revision, GitHub hash, and vendor hash
   - `flake.nix`: Vendor hash
   - `README.md`: Git revision and GitHub hash in examples

### When to use

Run this script after:
- Making changes to Go dependencies (go.mod/go.sum)
- Before creating a release
- When updating the Nix packaging to point to a specific commit

### Requirements

- Nix (with `nix-prefetch-url` and `nix-hash`)
- Go
- Git
- sed

The script will verify all requirements are available before running.