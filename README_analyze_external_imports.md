# External Import Analyzer

Analyzes external GitHub dependencies in the Datadog Agent codebase, mapping them to teams (via CODEOWNERS) and measuring their binary size impact.

## Quick Start

```bash
# Analyze entire codebase (pkg, comp, cmd)
python3 analyze_external_imports.py

# Analyze specific directories
python3 analyze_external_imports.py pkg/logs comp/logs
python3 analyze_external_imports.py pkg/network

# Skip binary size analysis for faster results
python3 analyze_external_imports.py --no-binary
```

## What It Does

1. **Finds Go Files**: Scans specified directories for `.go` files (excluding `*_test.go`)
2. **Extracts Imports**: Identifies external GitHub imports (non-DataDog dependencies)
3. **Maps to Teams**: Uses `.github/CODEOWNERS` to determine which team owns each import
4. **Analyzes Binary Size**: Uses `go tool nm` to measure the binary size impact of each dependency
5. **Reports Results**: Shows which teams are responsible for the most external dependency binary bloat

## Output

The script produces three sections:

### 1. External Imports by Team
Lists all external imports grouped by owning team, showing which files use them.

### 2. Binary Size Impact Analysis
Shows the binary size contribution of each external dependency, aggregated by team.

### 3. Summary
Quick overview with top imports by size and usage statistics.

## Requirements

- Python 3.7+
- `.github/CODEOWNERS` file in repo root
- Binary built at `bin/agent/agent` (or specify with `--binary`)
- `go tool nm` available in PATH

## Options

```
usage: analyze_external_imports.py [-h] [--no-binary] [--binary BINARY]
                                   [directories ...]

positional arguments:
  directories      Directories to analyze (default: pkg, comp, cmd)

options:
  -h, --help       show this help message and exit
  --no-binary      Skip binary size analysis
  --binary BINARY  Path to binary for size analysis (default: bin/agent/agent)
```

## Examples

```bash
# Analyze entire codebase
python3 analyze_external_imports.py

# Analyze specific subsystems
python3 analyze_external_imports.py pkg/logs comp/logs
python3 analyze_external_imports.py pkg/network comp/networkpath

# Quick import check without binary analysis
python3 analyze_external_imports.py --no-binary

# Analyze only pkg directory
python3 analyze_external_imports.py pkg

# Use different binary
python3 analyze_external_imports.py --binary bin/system-probe/system-probe pkg/ebpf
```

## Use Cases

- **Dependency Audit**: Find which teams are adding external dependencies
- **Binary Size Analysis**: Identify which external deps contribute most to binary size
- **Code Review**: Understand dependency ownership before adding new imports
- **Refactoring**: Track dependency reduction efforts across teams

## Notes

- The script only finds external GitHub dependencies (excludes `github.com/DataDog/*`)
- Binary size analysis requires the binary to be built first (`dda inv agent.build`)
- Some dependencies may show 0 bytes if they're build-tag specific or optimized out
- Platform-specific imports (e.g., Linux-only) may not appear in macOS-built binaries

