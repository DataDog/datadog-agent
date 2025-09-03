import json
from abc import abstractmethod
from pathlib import Path

from invoke import Context

from tasks.libs.common.color import Color, color_message
from tasks.libs.dynamic_test.index import DynamicTestIndex
from tasks.libs.dynamic_test.indexer import DynTestIndexer


class CoverageDynTestIndexer(DynTestIndexer):
    """Base class for Go coverage-based implementations of DynTestIndexer.

    Provides common functionality for processing Go coverage data and building
    dynamic test indexes. Subclasses implement the specific granularity (file vs package)
    for coverage analysis.

    Expected Input Structure:
        coverage_root/
          suite1/
            metadata.json       # Contains job_name and test name (optional)
            coverage/           # Go covdata directory from test execution
          suite2/
            metadata.json
            coverage/
          ...

    Common Process:
    1. Scans coverage root directory for test suite folders
    2. Reads metadata to determine job and test names
    3. Converts Go coverage data to text format using 'go tool covdata'
    4. Parses coverage text to extract covered items (delegated to subclasses)
    5. Builds reverse index mapping items to tests per job

    The indexer handles:
    - Missing or malformed metadata (falls back to folder names)
    - Coverage conversion failures (skips with warnings)
    - Path extraction and normalization (subclass-specific)
    - Deduplication across multiple test suites
    """

    def __init__(self, coverage_root: str) -> None:
        """Initialize the coverage-based indexer.

        Args:
            coverage_root: Path to the root directory containing coverage output folders
        """
        self.coverage_root = coverage_root

    def compute_index(self, ctx: Context) -> DynamicTestIndex:
        """Compute the dynamic test index from coverage data.

        Args:
            ctx: Invoke context for running shell commands

        Returns:
            DynamicTestIndex: Index mapping coverage items to tests per job
        """
        root = Path(self.coverage_root)
        if not root.exists() or not root.is_dir():
            raise FileNotFoundError(f"Coverage root not found or not a directory: {self.coverage_root}")

        job_to_item_tests: dict[str, dict[str, list[str]]] = {}

        for entry in sorted(root.iterdir()):
            if not entry.is_dir():
                continue

            suite_folder = entry
            metadata = self._read_metadata(suite_folder)
            job_name = metadata.get("job_name", suite_folder.name)
            test_name = metadata.get("test", suite_folder.name)

            coverage_dir = suite_folder / "coverage"
            if not coverage_dir.exists() or not coverage_dir.is_dir():
                print(color_message(f"No coverage/ folder in {suite_folder}", Color.ORANGE))
                continue

            coverage_txt = suite_folder / "coverage.txt"
            # Convert to textfmt using go tool covdata
            ctx.run(
                f"go tool covdata textfmt -i={coverage_dir} -o={coverage_txt}",
                echo=False,
                warn=True,
            )

            if not coverage_txt.exists():
                print(color_message(f"Failed to generate {coverage_txt}", Color.ORANGE))
                continue

            covered_items = self._parse_coverage_file(coverage_txt)
            if not covered_items:
                continue

            job_entry = job_to_item_tests.setdefault(job_name, {})
            for item in covered_items:
                tests = job_entry.setdefault(item, [])
                if test_name not in tests:
                    tests.append(test_name)

        index = DynamicTestIndex(job_to_item_tests)
        return index

    def _read_metadata(self, suite_folder: Path) -> dict[str, str]:
        """Read metadata.json file from a test suite folder.

        Args:
            suite_folder: Path to the test suite folder

        Returns:
            dict[str, str]: Metadata dictionary, empty if file doesn't exist or is malformed
        """
        metadata_path = suite_folder / "metadata.json"
        if not metadata_path.exists():
            return {}
        try:
            with open(metadata_path, encoding="utf-8") as f:
                return json.load(f)
        except Exception as e:
            print(color_message(f"Error reading {metadata_path}: {e}", Color.ORANGE))
            return {}

    def _extract_relative_path(self, file_path: str) -> str:
        """Extract relative path from full Go module path.

        Args:
            file_path: Full path from Go coverage (e.g., github.com/DataDog/datadog-agent/pkg/util/log.go)

        Returns:
            str: Relative path from module root (e.g., pkg/util/log.go)
        """
        segments = file_path.split("/")
        # Expect at least github.com DataDog datadog-agent filename
        if len(segments) < 4:
            return ""
        # Build path starting from after github.com/DataDog/datadog-agent, i.e., join from index 3
        return "/".join(segments[3:])

    def _parse_coverage_line(self, line: str) -> tuple[str, bool]:
        """Parse a single coverage line and determine if it has coverage.

        Args:
            line: Coverage line in format "file_path:ranges statements count"

        Returns:
            tuple[str, bool]: (file_path, has_coverage)
        """
        if not line or line.startswith("mode:"):
            return "", False

        # Split at ':' first time to separate file path from the rest
        parts = line.strip().split(":", 1)
        if len(parts) < 2:
            return "", False
        file_path, rest = parts[0], parts[1]

        # Determine if the line indicates any coverage > 0
        # The rest contains positions and two integers, e.g. "24.13,25.2 2 1"
        try:
            tail_numbers = rest.strip().split()
            if not tail_numbers:
                return "", False
            covered_count = int(tail_numbers[-1])
            has_coverage = covered_count > 0
        except Exception:
            # If parsing fails, skip the line
            return "", False

        return file_path, has_coverage

    @abstractmethod
    def _parse_coverage_file(self, coverage_txt: Path) -> set[str]:
        """Parse Go coverage text file and extract covered items.

        This method must be implemented by subclasses to define the granularity
        of coverage analysis (files, packages, etc.).

        Args:
            coverage_txt: Path to the coverage text file

        Returns:
            set[str]: Set of covered items (files, packages, etc.)
        """
        pass


class FileCoverageDynTestIndexer(CoverageDynTestIndexer):
    """Go coverage-based implementation of DynTestIndexer at file granularity.

    Builds dynamic test indexes by analyzing Go coverage data generated during test execution.
    The indexer processes coverage output directories, extracts file coverage information,
    and creates reverse mappings from files to the tests that exercise them.

    Expected Input Structure:
        coverage_root/
          suite1/
            metadata.json       # Contains job_name and test name (optional)
            coverage/           # Go covdata directory from test execution
          suite2/
            metadata.json
            coverage/
          ...

    Process:
    1. Scans coverage root directory for test suite folders
    2. Reads metadata to determine job and test names
    3. Converts Go coverage data to text format using 'go tool covdata'
    4. Parses coverage text to extract covered files
    5. Builds reverse index mapping files to tests per job

    The indexer handles:
    - Missing or malformed metadata (falls back to folder names)
    - Coverage conversion failures (skips with warnings)
    - File path extraction and normalization
    - Deduplication across multiple test suites
    """

    def _parse_coverage_file(self, coverage_txt: Path) -> set[str]:
        """Parse Go coverage text file and extract covered files.

        Processes Go coverage text format to identify which files were exercised
        during test execution. Only considers lines with non-zero coverage counts.

        Coverage line format:
            github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe.go:24.13,25.2 2 1
            - File path, line ranges, statement count, execution count
            - We consider a file covered when execution count (last number) > 0

        Args:
            coverage_txt: Path to the coverage text file

        Returns:
            set[str]: Set of file paths that have coverage, relative to module root
                     (e.g., "pkg/collector/corechecks/ebpf/probe.go", "pkg/util/log/logger.go")
        """
        covered: set[str] = set()
        try:
            with open(coverage_txt, encoding="utf-8") as f:
                for line in f:
                    file_path, has_coverage = self._parse_coverage_line(line)
                    if not has_coverage or not file_path:
                        continue

                    # Extract relative file path and keep .go extension
                    relative_file_path = self._extract_relative_path(file_path)
                    if relative_file_path:
                        covered.add(relative_file_path)
        except Exception as e:
            print(color_message(f"Error parsing coverage file {coverage_txt}: {e}", Color.ORANGE))
            return set()
        return covered


class PackageCoverageDynTestIndexer(CoverageDynTestIndexer):
    """Go coverage-based implementation of DynTestIndexer.

    Builds dynamic test indexes by analyzing Go coverage data generated during test execution.
    The indexer processes coverage output directories, extracts package coverage information,
    and creates reverse mappings from packages to the tests that exercise them.

    Expected Input Structure:
        coverage_root/
          suite1/
            metadata.json       # Contains job_name and test name (optional)
            coverage/           # Go covdata directory from test execution
          suite2/
            metadata.json
            coverage/
          ...

    Process:
    1. Scans coverage root directory for test suite folders
    2. Reads metadata to determine job and test names
    3. Converts Go coverage data to text format using 'go tool covdata'
    4. Parses coverage text to extract covered packages
    5. Builds reverse index mapping packages to tests per job

    The indexer handles:
    - Missing or malformed metadata (falls back to folder names)
    - Coverage conversion failures (skips with warnings)
    - Package path extraction and normalization
    - Deduplication across multiple test suites
    """

    def _parse_coverage_file(self, coverage_txt: Path) -> set[str]:
        """Parse Go coverage text file and extract covered packages.

        Processes Go coverage text format to identify which packages were exercised
        during test execution. Only considers lines with non-zero coverage counts.

        Coverage line format:
            github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe.go:24.13,25.2 2 1
            - File path, line ranges, statement count, execution count
            - We consider a file covered when execution count (last number) > 0
            - A package is covered if any file in it has coverage > 0

        Args:
            coverage_txt: Path to the coverage text file

        Returns:
            set[str]: Set of package paths that have coverage, relative to module root
                     (e.g., "pkg/collector/corechecks", "pkg/util/log")
        """
        covered: set[str] = set()
        try:
            with open(coverage_txt, encoding="utf-8") as f:
                for line in f:
                    file_path, has_coverage = self._parse_coverage_line(line)
                    if not has_coverage or not file_path:
                        continue

                    # Extract package path from file path
                    # Remove .go suffix if present and take directory components after module root
                    if file_path.endswith(".go"):
                        file_path = file_path[:-3]

                    relative_path = self._extract_relative_path(file_path)
                    if not relative_path:
                        continue

                    # Convert to package directory (drop filename)
                    if "/" in relative_path:
                        package_path = "/".join(relative_path.split("/")[:-1])
                    else:
                        package_path = relative_path

                    if package_path:
                        covered.add(package_path)
        except Exception as e:
            print(color_message(f"Error parsing coverage file {coverage_txt}: {e}", Color.ORANGE))
            return set()
        return covered
