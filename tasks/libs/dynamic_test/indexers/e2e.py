import json
import os
from abc import abstractmethod
from collections import defaultdict
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

    def __init__(self, coverage_root: str, run_all_changes_paths: list[str] | None = None) -> None:
        """Initialize the coverage-based indexer.

        Args:
            coverage_root: Path to the root directory containing coverage output folders
        """
        self.coverage_root = coverage_root
        self.run_all_changes_paths = []
        if run_all_changes_paths is not None:
            self.run_all_changes_paths = run_all_changes_paths

    def compute_index(self, ctx: Context) -> DynamicTestIndex:
        index = self._compute_index(ctx)
        for pattern in self.run_all_changes_paths:
            for job in index.get_jobs():
                index.add_tests(job, pattern, ["*"])
        return index

    @abstractmethod
    def _compute_index(self, ctx: Context) -> DynamicTestIndex:
        raise NotImplementedError

    def read_metadata(self, suite_folder: Path) -> dict[str, str]:
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

    def extract_relative_path(self, file_path: str) -> str:
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

    def parse_coverage_line(self, line: str) -> tuple[str, str, int]:
        """Parse a single coverage line and determine if it has coverage.

        Args:
            line: Coverage line in format "file_path:ranges statements count"
            github.com/DataDog/datadog-agent/pkg/collector/corechecks/check.go:24.13,25.2 2 1

        Returns:
            tuple[str, bool]: (file_path, range, n_covered)
        """
        if not line or line.startswith("mode:"):
            return "", "", 0
        parts = line.strip().split()
        if len(parts) < 3:
            raise ValueError(f"Invalid coverage line: {line}")
        file_with_range = parts[0].split(":")
        if len(file_with_range) < 2:
            raise ValueError(f"Invalid coverage line: {line}")
        file_path, range = file_with_range[0], file_with_range[1]
        n_covered = int(parts[2])
        return file_path, range, n_covered

    def convert_coverage_to_text(self, ctx: Context, coverage_dir: Path) -> Path:
        """Convert coverage data to text format using 'go tool covdata'.

        Args:
            ctx: Invoke context for running shell commands
            coverage_dir: Path to the coverage directory
        """
        ctx.run(f"go tool covdata textfmt -i={coverage_dir} -o={coverage_dir}/coverage.txt", echo=False, warn=True)
        return coverage_dir / "coverage.txt"


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

    def _compute_index(self, ctx: Context) -> DynamicTestIndex:
        """Compute the dynamic test index from file coverage data.

        Args:
            ctx: Invoke context for running shell commands

        Returns:
            DynamicTestIndex: Index mapping files to tests per job
        """
        index = DynamicTestIndex()
        coverage_root = Path(self.coverage_root)
        for suite in sorted(coverage_root.iterdir()):
            if not suite.is_dir():
                continue
            coverage_dir = suite / "coverage"
            if not coverage_dir.exists():
                continue
            coverage_txt = self.convert_coverage_to_text(ctx, coverage_dir)
            with open(coverage_txt, encoding="utf-8") as f:
                for line in f:
                    file_path, range, has_coverage = self.parse_coverage_line(line)

                    if has_coverage == 0 or not file_path:
                        continue

                    metadata = self.read_metadata(suite)
                    job_name = metadata.get("job_name", suite.name)
                    test_name = metadata.get("test", suite.name)
                    file_path = self.extract_relative_path(file_path)

                    index.add_tests(job_name, file_path, [test_name])
        return index


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

    def _compute_index(self, ctx: Context) -> DynamicTestIndex:
        """Compute the dynamic test index from package coverage data.

        Args:
            ctx: Invoke context for running shell commands

        Returns:
            DynamicTestIndex: Index mapping packages to tests per job
        """
        index = DynamicTestIndex()
        coverage_root = Path(self.coverage_root)
        for suite in sorted(coverage_root.iterdir()):
            if not suite.is_dir():
                continue
            coverage_dir = suite / "coverage"
            if not coverage_dir.exists():
                continue
            coverage_txt = self.convert_coverage_to_text(ctx, coverage_dir)
            with open(coverage_txt, encoding="utf-8") as f:
                for line in f:
                    file_path, range, n_covered = self.parse_coverage_line(line)
                    if n_covered == 0 or not file_path:
                        continue
                    metadata = self.read_metadata(suite)
                    job_name = metadata.get("job_name", suite.name)
                    test_name = metadata.get("test", suite.name)
                    file_path = self.extract_relative_path(file_path)
                    package_path = file_path.rsplit("/", 1)[0]
                    index.add_tests(job_name, package_path, [test_name])
        return index


class DiffedPackageCoverageDynTestIndexer(CoverageDynTestIndexer):
    """Package coverage-based indexer that uses baseline comparison for differential coverage.

    This indexer extends CoverageDynTestIndexer to compute dynamic test indexes based on
    the difference between current package coverage and a baseline package coverage using
    'go tool covdata subtract' for precise differential coverage computation.

    Expected Input Structure:
        coverage_root/ (same as base class)
        baseline_coverage_root/ (same structure as coverage_root)

    Process:
    1. For each test suite, uses 'go tool covdata subtract' to compute precise diff
    2. Converts differential coverage to text format
    3. Parses differential coverage to extract affected packages
    4. Builds index only for packages with new/increased coverage
    """

    def __init__(self, coverage_root: str, baseline_coverage_root: str, is_baseline_job: bool = False) -> None:
        """Initialize the differential package coverage indexer.

        Args:
            coverage_root: Path to the current coverage data directory
            baseline_coverage_root: Path to the baseline coverage data directory
        """
        super().__init__(coverage_root)
        self.baseline_coverage_root = baseline_coverage_root
        self.is_baseline_job = is_baseline_job
        if os.getenv("CI_JOB_NAME") == "new-e2e-base-coverage":
            self.is_baseline_job = True

    def _compute_index(self, ctx: Context) -> DynamicTestIndex:
        """Compute the dynamic test index from differential package coverage data.

        Args:
            ctx: Invoke context for running shell commands

        Returns:
            DynamicTestIndex: Index mapping packages with differential coverage to tests per job
        """
        index = DynamicTestIndex()
        coverage_path = Path(self.coverage_root)
        baseline_path = Path(self.baseline_coverage_root)
        baseline_coverage_dir = baseline_path / "coverage"
        if not baseline_coverage_dir.exists():
            return index

        baseline_covered_txt = self.convert_coverage_to_text(ctx, baseline_coverage_dir)
        baseline_covered = self._parse_baseline_coverage(baseline_covered_txt)

        for suite in sorted(coverage_path.iterdir()):
            if not suite.is_dir():
                continue
            coverage_dir = suite / "coverage"
            if not coverage_dir.exists():
                continue

            if suite == baseline_path and not self.is_baseline_job:
                continue
            coverage_txt = self.convert_coverage_to_text(ctx, coverage_dir)
            with open(coverage_txt, encoding="utf-8") as f:
                for line in f:
                    file_path, range, n_covered = self.parse_coverage_line(line)
                    if n_covered == 0 or not file_path:
                        continue
                    if (
                        not self.is_baseline_job
                        and file_path in baseline_covered
                        and range in baseline_covered[file_path]
                        and baseline_covered[file_path][range]
                        > 0  # We consider it is covered by baseline test so not specific to the current test
                    ):
                        continue
                    metadata = self.read_metadata(suite)
                    job_name = metadata.get("job_name", suite.name)
                    test_name = metadata.get("test", suite.name)
                    file_path = self.extract_relative_path(file_path)
                    package_path = file_path.rsplit("/", 1)[0]
                    index.add_tests(job_name, package_path, [test_name])
        return index

    def _parse_baseline_coverage(self, baseline_covered_txt: Path) -> dict[str, dict[str, int]]:
        baseline_covered = defaultdict(lambda: defaultdict(int))  # file_path -> set of ranges covered in the baseline
        with open(baseline_covered_txt, encoding="utf-8") as f:
            for line in f:
                file_path, range, n_covered = self.parse_coverage_line(line)
                if n_covered == 0 or not file_path:
                    continue
                baseline_covered[file_path][range] = n_covered
        return baseline_covered
