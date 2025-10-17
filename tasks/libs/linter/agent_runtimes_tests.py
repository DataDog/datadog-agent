"""
Linter for agent-runtimes e2e tests.

This module validates:
1. Test naming follows the convention: Test(Linux|Windows)<Feature>Suite
2. All tests are registered in .gitlab/e2e/e2e.yml
3. No duplicate test names exist
"""

import re
import sys
from pathlib import Path

from tasks.libs.common.color import Color, color_message

# Expected test name pattern
TEST_NAME_PATTERN = re.compile(r'^func (Test(Linux|Windows)\w+Suite)\(t \*testing\.T\)')
VALID_OS_PREFIX = ['Linux', 'Windows']

# YAML patterns to extract test filters
YAML_EXTRA_PARAMS_PATTERN = re.compile(r'EXTRA_PARAMS:\s*--run\s+"([^"]+)"')


def find_test_files(base_dir: Path) -> list[Path]:
    """Find all *_test.go files in the agent-runtimes directory"""
    test_files = []
    for pattern in ['*_test.go']:
        test_files.extend(base_dir.rglob(pattern))
    return sorted(test_files)


def extract_test_functions(file_path: Path) -> list[tuple[str, int]]:
    """Extract test function names and their line numbers from a Go test file"""
    tests = []
    with open(file_path, encoding='utf-8') as f:
        for line_num, line in enumerate(f, 1):
            match = TEST_NAME_PATTERN.match(line.strip())
            if match:
                test_name = match.group(1)
                tests.append((test_name, line_num))
    return tests


def validate_test_name(test_name: str, file_path: Path, line_num: int) -> list[str]:
    """Validate test name follows the naming convention"""
    errors = []

    # Check if it starts with Test
    if not test_name.startswith('Test'):
        errors.append(f"{file_path}:{line_num}: Test name '{test_name}' must start with 'Test'")
        return errors

    # Check if it ends with Suite
    if not test_name.endswith('Suite'):
        errors.append(
            f"{file_path}:{line_num}: Test name '{test_name}' must end with 'Suite'\n"
            f"  Expected format: Test(Linux|Windows)<Feature>Suite\n"
            f"  Example: TestLinuxDiskSuite, TestWindowsNetworkSuite"
        )
        return errors

    # Check if it has correct OS prefix
    has_valid_prefix = False
    for os_prefix in VALID_OS_PREFIX:
        if test_name.startswith(f'Test{os_prefix}'):
            has_valid_prefix = True
            break

    if not has_valid_prefix:
        errors.append(
            f"{file_path}:{line_num}: Test name '{test_name}' must start with 'TestLinux' or 'TestWindows'\n"
            f"  Current: {test_name}\n"
            f"  Expected format: Test(Linux|Windows)<Feature>Suite\n"
            f"  Examples:\n"
            f"    - TestLinuxDiskSuite\n"
            f"    - TestWindowsNetworkSuite\n"
            f"    - TestLinuxAuthArtifactSuite\n"
            f"    - TestWindowsIPCSuite"
        )

    return errors


def extract_yaml_test_patterns(yaml_path: Path) -> set[str]:
    """Extract test name patterns from the YAML configuration"""
    patterns = set()

    with open(yaml_path, encoding='utf-8') as f:
        content = f.read()

    # Find the agent-runtimes section
    agent_runtimes_section = re.search(r'new-e2e-agent-runtimes:.*?(?=\nnew-e2e-|\Z)', content, re.DOTALL)

    if not agent_runtimes_section:
        return patterns

    section_content = agent_runtimes_section.group(0)

    # Extract all EXTRA_PARAMS patterns
    for match in YAML_EXTRA_PARAMS_PATTERN.finditer(section_content):
        pattern = match.group(1)
        patterns.add(pattern)

    return patterns


def test_matches_pattern(test_name: str, pattern: str) -> bool:
    """Check if a test name matches a regex pattern from YAML"""
    try:
        # Convert the pattern to a full regex
        regex = re.compile(pattern)
        return regex.search(test_name) is not None
    except re.error:
        return False


def check_test_in_yaml(test_name: str, yaml_patterns: set[str]) -> bool:
    """Check if test is covered by any pattern in YAML"""
    for pattern in yaml_patterns:
        if test_matches_pattern(test_name, pattern):
            return True
    return False


def suggest_yaml_pattern(test_name: str) -> str:
    """Suggest a YAML pattern for a test name"""
    # Extract the feature name
    for os_prefix in VALID_OS_PREFIX:
        if test_name.startswith(f'Test{os_prefix}'):
            feature = test_name[len(f'Test{os_prefix}') : -len('Suite')]
            return f'--run "Test(Linux|Windows){feature}Suite"'

    return f'--run "{test_name}"'


def lint_agent_runtimes_tests(repo_root: Path, check_yaml_only: bool = False) -> bool:
    """
    Lint agent-runtimes e2e tests.

    Args:
        repo_root: Path to the repository root
        check_yaml_only: Only check if tests are registered in YAML

    Returns:
        True if linting passed, False otherwise
    """
    agent_runtimes_dir = repo_root / 'test' / 'new-e2e' / 'tests' / 'agent-runtimes'
    yaml_path = repo_root / '.gitlab' / 'e2e' / 'e2e.yml'

    if not agent_runtimes_dir.exists():
        print(
            f"{color_message('Error', Color.RED)}: agent-runtimes directory not found: {agent_runtimes_dir}",
            file=sys.stderr,
        )
        return False

    if not yaml_path.exists():
        print(f"{color_message('Error', Color.RED)}: YAML file not found: {yaml_path}", file=sys.stderr)
        return False

    # Find all test files
    test_files = find_test_files(agent_runtimes_dir)

    if not test_files:
        print(
            f"{color_message('Warning', Color.ORANGE)}: No test files found in {agent_runtimes_dir}",
            file=sys.stderr,
        )
        return True

    # Extract YAML patterns
    yaml_patterns = extract_yaml_test_patterns(yaml_path)

    # Collect all tests and validate
    all_tests: dict[str, list[tuple[Path, int]]] = {}
    errors = []
    missing_in_yaml = []

    for test_file in test_files:
        # Skip common test files (they don't contain top-level test functions)
        if '_common_test.go' in test_file.name:
            continue

        tests = extract_test_functions(test_file)

        for test_name, line_num in tests:
            # Track duplicate test names
            if test_name not in all_tests:
                all_tests[test_name] = []
            all_tests[test_name].append((test_file, line_num))

            # Validate test name format
            if not check_yaml_only:
                validation_errors = validate_test_name(test_name, test_file, line_num)
                errors.extend(validation_errors)

            # Check if test is in YAML
            if not check_test_in_yaml(test_name, yaml_patterns):
                missing_in_yaml.append((test_name, test_file, line_num))

    # Check for duplicate test names
    if not check_yaml_only:
        for test_name, locations in all_tests.items():
            if len(locations) > 1:
                error_msg = f"Duplicate test name '{test_name}' found in multiple files:"
                for file_path, line_num in locations:
                    error_msg += f"\n  - {file_path}:{line_num}"
                errors.append(error_msg)

    # Report errors
    has_errors = False

    if errors:
        has_errors = True
        print(f"\n{color_message('❌ Test naming validation errors:', Color.RED)}\n", file=sys.stderr)
        for error in errors:
            print(error, file=sys.stderr)
            print(file=sys.stderr)

    if missing_in_yaml:
        has_errors = True
        print(f"\n{color_message('❌ Tests not registered in .gitlab/e2e/e2e.yml:', Color.RED)}\n", file=sys.stderr)
        for test_name, file_path, line_num in missing_in_yaml:
            rel_path = file_path.relative_to(repo_root)
            print(f"  {rel_path}:{line_num}: {test_name}", file=sys.stderr)
            print("    Suggested YAML entry:", file=sys.stderr)
            print(f"      - EXTRA_PARAMS: {suggest_yaml_pattern(test_name)}", file=sys.stderr)
            print(file=sys.stderr)

    if not has_errors:
        print(color_message("✅ All tests are properly named and registered!", Color.GREEN))
        return True

    print("\n" + "=" * 80, file=sys.stderr)
    print(color_message("SUMMARY:", Color.ORANGE), file=sys.stderr)
    print(f"  Total test files scanned: {len(test_files)}", file=sys.stderr)
    print(f"  Total tests found: {len(all_tests)}", file=sys.stderr)
    print(f"  Naming errors: {len(errors)}", file=sys.stderr)
    print(f"  Missing in YAML: {len(missing_in_yaml)}", file=sys.stderr)
    print("=" * 80, file=sys.stderr)

    return False
