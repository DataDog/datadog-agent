#!/usr/bin/env python3
"""
Analyze external GitHub imports, mapping them to teams and their binary size impact.

This script:
1. Finds all Go files (excluding tests) in specified directories
2. Extracts external GitHub imports (non-DataDog dependencies)
3. Maps imports to teams using .github/CODEOWNERS
4. Analyzes binary size impact using 'go tool nm'
5. Reports which teams are responsible for the most binary size from external deps

Usage:
    python3 analyze_external_imports.py [directories...]
    
    If no directories specified, analyzes entire codebase (pkg, comp, cmd)
    
Examples:
    python3 analyze_external_imports.py              # Analyze entire codebase
    python3 analyze_external_imports.py pkg/logs     # Analyze specific directory
    python3 analyze_external_imports.py pkg comp     # Analyze multiple directories
    python3 analyze_external_imports.py --no-binary  # Skip binary analysis (faster)

Requirements:
    - .github/CODEOWNERS file in repo root
    - Binary built at bin/agent/agent (or specify with --binary)
    - 'go tool nm' available
"""

import argparse
import re
import subprocess
import sys
from collections import defaultdict
from pathlib import Path
from typing import Dict, List, Set, Tuple


class CodeOwnersParser:
    """Parse CODEOWNERS file and map paths to teams."""
    
    def __init__(self, codeowners_path: Path):
        self.rules = []  # List of (pattern, teams) tuples, in order
        self._parse(codeowners_path)
    
    def _parse(self, path: Path):
        """Parse CODEOWNERS file."""
        with open(path) as f:
            for line in f:
                line = line.strip()
                # Skip comments and empty lines
                if not line or line.startswith('#'):
                    continue
                
                parts = line.split()
                if len(parts) < 2:
                    continue
                
                pattern = parts[0]
                teams = [t for t in parts[1:] if t.startswith('@')]
                
                if teams:
                    self.rules.append((pattern, teams))
    
    def get_teams(self, file_path: str) -> List[str]:
        """Get teams owning a file path. Returns last matching rule (bottom-to-top)."""
        # Normalize path
        path = file_path.lstrip('/')
        
        matched_teams = []
        # Rules are matched bottom-to-top in CODEOWNERS
        for pattern, teams in self.rules:
            if self._matches(pattern, path):
                matched_teams = teams
        
        return matched_teams if matched_teams else ['@DataDog/unowned']
    
    def _matches(self, pattern: str, path: str) -> bool:
        """Check if a CODEOWNERS pattern matches a path."""
        pattern = pattern.lstrip('/')
        
        # Exact match
        if pattern == path:
            return True
        
        # Directory match (pattern ends with /)
        if pattern.endswith('/') and path.startswith(pattern):
            return True
        
        # Wildcard patterns
        if '*' in pattern:
            regex = pattern.replace('*', '.*').replace('/', '\\/')
            if re.match(f'^{regex}', path):
                return True
        
        # Parent directory match
        if path.startswith(pattern + '/'):
            return True
        
        return False


def find_go_files(directories: List[str]) -> List[Path]:
    """Find all .go files (excluding _test.go) in given directories."""
    files = []
    for directory in directories:
        dir_path = Path(directory)
        if dir_path.exists():
            files.extend(
                f for f in dir_path.rglob('*.go')
                if not f.name.endswith('_test.go')
            )
    return files


def extract_imports(file_path: Path) -> Set[str]:
    """Extract import statements from a Go file."""
    imports = set()
    in_import_block = False
    
    with open(file_path) as f:
        for line in f:
            line = line.strip()
            
            # Single-line import
            if line.startswith('import "'):
                match = re.search(r'import "([^"]+)"', line)
                if match:
                    imports.add(match.group(1))
            
            # Multi-line import block
            elif line.startswith('import ('):
                in_import_block = True
            elif in_import_block:
                if line == ')':
                    in_import_block = False
                else:
                    match = re.search(r'"([^"]+)"', line)
                    if match:
                        imports.add(match.group(1))
    
    return imports


def is_external_github_import(pkg: str) -> bool:
    """Check if import is external GitHub package (not DataDog)."""
    return pkg.startswith('github.com/') and not pkg.startswith('github.com/DataDog/')


def parse_nm_output(binary_path: str) -> Dict[str, int]:
    """Parse `go tool nm` output and aggregate sizes by package."""
    try:
        result = subprocess.run(
            ['go', 'tool', 'nm', '-size', binary_path],
            capture_output=True,
            text=True,
            check=True
        )
    except subprocess.CalledProcessError as e:
        print(f"Error running go tool nm: {e}", file=sys.stderr)
        print(f"stderr: {e.stderr}", file=sys.stderr)
        return {}
    except FileNotFoundError:
        print(f"Binary not found: {binary_path}", file=sys.stderr)
        return {}
    
    package_sizes = defaultdict(int)
    
    # Parse nm output: address size type symbol
    for line in result.stdout.splitlines():
        parts = line.split(maxsplit=3)
        if len(parts) >= 4:
            try:
                size = int(parts[1])  # Size is in decimal
                symbol_type = parts[2]
                symbol = parts[3]
                
                # Skip undefined symbols (type 'U') and very large sizes (likely undefined)
                if symbol_type == 'U' or size > 1000000000:
                    continue
                
                # Extract package from symbol (e.g., github.com/pkg/name.Function)
                # Go symbols typically have format: package/path.Symbol
                if '.' in symbol:
                    pkg_path = symbol.rsplit('.', 1)[0]
                    # Clean up vendor prefixes
                    pkg_path = pkg_path.replace('vendor/', '')
                    package_sizes[pkg_path] += size
            except (ValueError, IndexError):
                continue
    
    return dict(package_sizes)


def get_package_prefixes(import_path: str) -> List[str]:
    """Get all possible package prefixes for matching symbols.
    
    Returns multiple prefixes to match the main package and subpackages.
    E.g., for "github.com/spf13/afero" returns:
    - github.com/spf13/afero
    - github.com/spf13/afero/
    """
    prefixes = [import_path]
    # Also match subpackages
    if not import_path.endswith('/'):
        prefixes.append(import_path + '/')
    return prefixes


def print_summary(directories, go_files, import_to_files, team_to_imports, 
                  import_sizes, import_to_teams):
    """Print summary section."""
    print("=" * 80)
    print("SUMMARY")
    print("=" * 80)
    print(f"\nDirectories analyzed: {', '.join(directories)}")
    print(f"Go files (non-test): {len(go_files)}")
    print(f"Unique external GitHub imports: {len(import_to_files)}")
    print(f"Teams involved: {len(team_to_imports)}")
    
    if import_sizes:
        total_external_size = sum(import_sizes.values())
        print(f"Total binary size from external deps: {total_external_size:,} bytes ({total_external_size / 1024 / 1024:.2f} MB)")
        
        # Top imports by size
        print(f"\nTop 10 external imports by binary size:")
        top_5 = sorted(import_sizes.items(), key=lambda x: x[1], reverse=True)[:10]
        for i, (imp, size) in enumerate(top_5, 1):
            pct = (size / total_external_size * 100) if total_external_size > 0 else 0
            teams = ', '.join(sorted(import_to_teams.get(imp, [])))
            print(f"  {i}. {imp}")
            print(f"     Size: {size:,} bytes ({size / 1024:.1f} KB, {pct:.1f}%)")
            print(f"     Teams: {teams}")
            print(f"     Used in {len(import_to_files[imp])} files")
    else:
        print("Binary size analysis: not performed")
    
    print()


def main():
    parser = argparse.ArgumentParser(
        description='Analyze external GitHub imports by team and binary size impact',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s                          # Analyze entire codebase (pkg, comp, cmd)
  %(prog)s pkg/logs                 # Analyze only pkg/logs
  %(prog)s pkg comp                 # Analyze pkg and comp directories
  %(prog)s --no-binary              # Skip binary analysis (faster)
        """
    )
    parser.add_argument(
        'directories',
        nargs='*',
        default=None,
        help='Directories to analyze (default: pkg, comp, cmd)'
    )
    parser.add_argument(
        '--no-binary',
        action='store_true',
        help='Skip binary size analysis'
    )
    parser.add_argument(
        '--binary',
        default='bin/agent/agent',
        help='Path to binary for size analysis (default: bin/agent/agent)'
    )
    
    args = parser.parse_args()
    
    repo_root = Path(__file__).parent.resolve()
    codeowners_path = repo_root / '.github' / 'CODEOWNERS'
    
    if not codeowners_path.exists():
        print(f"CODEOWNERS file not found: {codeowners_path}", file=sys.stderr)
        sys.exit(1)
    
    print("Parsing CODEOWNERS...")
    codeowners = CodeOwnersParser(codeowners_path)
    
    # Default to analyzing entire codebase if no directories specified
    directories = args.directories
    if not directories:  # Empty list or None
        directories = ['pkg', 'comp', 'cmd']
        print(f"Finding Go files in {', '.join(directories)} (entire codebase)...")
    else:
        print(f"Finding Go files in {', '.join(directories)}...")
    
    go_files = find_go_files(directories)
    print(f"Found {len(go_files)} Go files (excluding tests)")
    
    print("\nExtracting external imports...")
    # Map: import -> set of files using it -> teams
    import_to_files = defaultdict(set)
    file_to_teams = {}
    
    for go_file in go_files:
        imports = extract_imports(go_file)
        external_imports = {imp for imp in imports if is_external_github_import(imp)}
        
        # Get teams for this file
        rel_path = str(go_file.resolve().relative_to(repo_root))
        teams = codeowners.get_teams(rel_path)
        file_to_teams[rel_path] = teams
        
        # Map imports to files
        for imp in external_imports:
            import_to_files[imp].add(rel_path)
    
    # Map imports to teams
    import_to_teams = defaultdict(set)
    for imp, files in import_to_files.items():
        for file in files:
            teams = file_to_teams.get(file, [])
            import_to_teams[imp].update(teams)
    
    print(f"Found {len(import_to_files)} unique external GitHub imports")
    
    # Print import breakdown by team
    print("\n" + "=" * 80)
    print("EXTERNAL IMPORTS BY TEAM")
    print("=" * 80)
    
    team_to_imports = defaultdict(set)
    for imp, teams in import_to_teams.items():
        for team in teams:
            team_to_imports[team].add(imp)
    
    for team in sorted(team_to_imports.keys()):
        imports = sorted(team_to_imports[team])
        print(f"\n{team} ({len(imports)} imports):")
        for imp in imports:
            files = sorted(import_to_files[imp])
            print(f"  - {imp}")
            for f in files[:3]:  # Show first 3 files
                print(f"      {f}")
            if len(files) > 3:
                print(f"      ... and {len(files) - 3} more files")
    
    # Binary size analysis
    if args.no_binary:
        print("\nSkipping binary size analysis (--no-binary flag set)")
        print_summary(directories, go_files, import_to_files, team_to_imports, None, None)
        return
    
    binary_path = repo_root / args.binary
    if not binary_path.exists():
        print(f"\n⚠️  Binary not found: {binary_path}", file=sys.stderr)
        print(f"Run 'dda inv agent.build' to build the agent first.", file=sys.stderr)
        print("\nSkipping binary size analysis.")
        print_summary(directories, go_files, import_to_files, team_to_imports, None, None)
        return
    
    print("\n" + "=" * 80)
    print("BINARY SIZE IMPACT ANALYSIS")
    print("=" * 80)
    print(f"\nAnalyzing binary: {binary_path}")
    print("Running 'go tool nm' (this may take a moment)...")
    
    package_sizes = parse_nm_output(str(binary_path))
    
    if not package_sizes:
        print("⚠️  Could not parse binary symbol data", file=sys.stderr)
        return
    
    # Map external imports to their symbol sizes
    import_sizes = {}
    for imp in import_to_files.keys():
        prefixes = get_package_prefixes(imp)
        total_size = 0
        
        # Sum sizes for all symbols matching this package or subpackages
        for pkg, size in package_sizes.items():
            for prefix in prefixes:
                if pkg == prefix.rstrip('/') or pkg.startswith(prefix):
                    total_size += size
                    break  # Don't double-count
        
        if total_size > 0:
            import_sizes[imp] = total_size
    
    # Aggregate by team
    team_sizes = defaultdict(int)
    team_imports_with_sizes = defaultdict(list)
    
    for imp, size in import_sizes.items():
        teams = import_to_teams.get(imp, [])
        for team in teams:
            team_sizes[team] += size
            team_imports_with_sizes[team].append((imp, size))
    
    # Sort teams by total size impact
    sorted_teams = sorted(team_sizes.items(), key=lambda x: x[1], reverse=True)
    
    total_external_size = sum(import_sizes.values())
    
    print(f"\nTotal external dependency size: {total_external_size:,} bytes ({total_external_size / 1024 / 1024:.2f} MB)")
    print(f"\nTop teams by external dependency binary size impact:\n")
    
    for team, size in sorted_teams:
        percentage = (size / total_external_size * 100) if total_external_size > 0 else 0
        print(f"{team}")
        print(f"  Total: {size:,} bytes ({size / 1024 / 1024:.2f} MB, {percentage:.1f}%)")
        
        # Show top imports for this team
        top_imports = sorted(team_imports_with_sizes[team], key=lambda x: x[1], reverse=True)[:5]
        if top_imports:
            print(f"  Top imports:")
            for imp, imp_size in top_imports:
                imp_pct = (imp_size / size * 100) if size > 0 else 0
                print(f"    - {imp}: {imp_size:,} bytes ({imp_pct:.1f}%)")
        print()
    
    # Summary
    print_summary(directories, go_files, import_to_files, team_to_imports, 
                  import_sizes, import_to_teams)


if __name__ == '__main__':
    main()

