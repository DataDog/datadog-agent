"""
Dynamic Test Index model and helpers.

Index format:
{
  <job-name>: {
     <package-name>: ["test1", "test2", ...]
  }
}
"""

import fnmatch
import json
import os
from collections.abc import Iterable
from enum import Enum
from typing import Self

IndexDict = dict[str, dict[str, list[str]]]


class IndexKind(Enum):
    """Enumeration of different index types supported by the dynamic test system.

    Each kind represents a different strategy for mapping code changes to tests:
    - PACKAGE: Maps Go packages to tests that exercise code in those packages

    Additional kinds can be added to support different granularities or languages:
    - FILE: Map individual files to tests (finer granularity)
    - MODULE: Map language-specific modules to tests
    - FUNCTION: Map individual functions to tests (finest granularity)
    """

    PACKAGE = "package"
    FILE = "file"
    DIFFED_PACKAGE = "diffed_package"


class DynamicTestIndex:
    """Core data structure for dynamic test selection.

    Maintains a reverse index that maps code components (packages, files, etc.) to
    the tests that exercise them, organized by CI job. This enables efficient lookup
    of which tests should be executed when specific code changes are made.

    Index Structure:


          "job_name": {
            "package_name": ["test1", "test2", ...],
            "other_package": ["test3", ...]
          },
          "other_job": {...}
        }

    Key Features:
    - Automatic deduplication of tests while preserving order
    - Deep copy semantics for safe access to internal data
    - Efficient merging of multiple indexes
    - JSON serialization for persistence
    - Impact analysis for determining affected tests
    - * can be used as target to indicate that all the indexed tests should be triggered

    Thread Safety:
        This class is not thread-safe. External synchronization is required
        for concurrent access.
    """

    def __init__(self, data: IndexDict | None = None) -> None:
        self._data: IndexDict = {}
        self._exact_keys: dict[str, dict[str, list[str]]] = {}  # job -> {exact_key -> tests}
        self._glob_keys: dict[str, dict[str, list[str]]] = {}  # job -> {glob_pattern -> tests}
        if data:
            self._data = self._normalize(data)
            self._split_keys()

    @staticmethod
    def _is_blob_pattern(key: str) -> bool:
        """Check if a key is a blob pattern (contains wildcards)."""
        return '*' in key or '?' in key or '[' in key

    @staticmethod
    def _matches_blob_pattern(pattern: str, package: str) -> bool:
        """Check if a package matches a blob pattern."""
        return fnmatch.fnmatch(package, pattern)

    def _split_keys(self) -> None:
        """Split the index keys into exact and glob pattern categories for efficient lookup."""
        self._exact_keys = {}
        self._glob_keys = {}

        for job_name, pkg_map in self._data.items():
            self._exact_keys[job_name] = {}
            self._glob_keys[job_name] = {}

            for key, tests in pkg_map.items():
                if self._is_blob_pattern(key):
                    self._glob_keys[job_name][key] = tests
                else:
                    self._exact_keys[job_name][key] = tests

    @staticmethod
    def _normalize(data: IndexDict) -> IndexDict:
        normalized: IndexDict = {}
        for job_name, pkg_map in data.items():
            if job_name not in normalized:
                normalized[job_name] = {}
            for package, tests in pkg_map.items():
                # Ensure list of strings and deduplicate while preserving order
                seen = set()
                deduped: list[str] = []
                for t in tests or []:
                    if t not in seen:
                        seen.add(t)
                        deduped.append(t)
                normalized[job_name][package] = deduped
        return normalized

    def to_dict(self) -> IndexDict:
        return json.loads(json.dumps(self._data))

    @classmethod
    def from_dict(cls, data: IndexDict) -> Self:
        return cls(data)

    def dump_json(self, path: str) -> None:
        parent = os.path.dirname(path)
        if parent:
            os.makedirs(parent, exist_ok=True)
        with open(path, "w", encoding="utf-8") as f:
            json.dump(self._data, f, indent=2, sort_keys=True)

    # --- Query helpers ---

    def get_jobs(self) -> list[str]:
        """Get all job names in the index."""
        return list(self._data.keys())

    def get_tests_for_job(self, job_name: str) -> dict[str, list[str]]:
        """Get all package-to-tests mappings for a specific job.

        Returns a deep copy of the job's data to prevent accidental mutation
        of the internal index structure.

        Args:
            job_name: Name of the CI job to get mappings for

        Returns:
            dict[str, list[str]]: Mapping of package names to lists of test names.
                                Returns empty dict if job doesn't exist.
        """
        return json.loads(json.dumps(self._data.get(job_name, {})))

    def get_indexed_tests_for_job(self, job_name: str) -> set[str]:
        """Get all test names that are indexed for a specific job.

        Flattens the package-to-tests mapping to return a set of all unique
        test names across all packages for the given job.

        Args:
            job_name: Name of the CI job to get test names for

        Returns:
            set[str]: Set of all unique test names indexed for the job.
                     Returns empty set if job doesn't exist.
        """
        indexed_tests = set()
        for _, tests in self._data.get(job_name, {}).items():
            for test in tests:
                if test == "*":
                    continue
                indexed_tests.add(test)
        return indexed_tests

    # --- Mutation helpers ---

    def add_tests(self, job_name: str, package: str, tests: Iterable[str]) -> None:
        """Add tests to the index for a specific job and package.

        Creates job and package entries if they don't exist. Automatically
        deduplicates tests while preserving order of first occurrence.
        Efficiently adds to the appropriate exact/glob dictionaries.

        Args:
            job_name: Name of the CI job
            package: Name of the code package/component or blob pattern
            tests: Iterable of test names to add
        """
        # Add to main data structure
        if job_name not in self._data:
            self._data[job_name] = {}
        if package not in self._data[job_name]:
            self._data[job_name][package] = []
        existing = set(self._data[job_name][package])
        for t in tests:
            if t not in existing:
                self._data[job_name][package].append(t)
                existing.add(t)

        # Add to appropriate processed dictionaries
        if job_name not in self._exact_keys:
            self._exact_keys[job_name] = {}
        if job_name not in self._glob_keys:
            self._glob_keys[job_name] = {}

        if self._is_blob_pattern(package):
            self._glob_keys[job_name][package] = self._data[job_name][package]
        else:
            self._exact_keys[job_name][package] = self._data[job_name][package]

    def merge(self, other: Self) -> None:
        """Merge another index into this one (in-place).

        Combines all job/package/test mappings from the other index into this one.
        Uses add_tests() internally to ensure proper deduplication.

        Args:
            other: Another DynamicTestIndex to merge into this one
        """
        for job_name, pkg_map in other._data.items():
            for package, tests in pkg_map.items():
                self.add_tests(job_name, package, tests)

    # --- Impact computation ---

    def impacted_tests(self, modified_packages: Iterable[str], job_name: str) -> set[str]:
        """Determine which tests are impacted by changes to specific packages.

        This is the core functionality for dynamic test selection - given a list
        of modified packages, return the set of tests that should be executed.

        Supports both exact package matching and blob pattern matching (e.g., cmd/*).
        Uses pre-split exact/glob keys for efficient lookup.

        Args:
            modified_packages: Iterable of package names that have been modified
            job_name: CI job name to restrict the search to

        Returns:
            set[str]: Set of test names that should be executed due to the changes.
                     Empty set if no tests are impacted or job doesn't exist.
        """
        impacted: set[str] = set()

        exact_keys = self._exact_keys.get(job_name, {})
        glob_keys = self._glob_keys.get(job_name, {})

        for pkg in modified_packages:
            # Fast exact match lookup using hash
            if pkg in exact_keys:
                impacted.update(exact_keys[pkg])

            # Check glob patterns (only iterate through glob keys)
            for glob_pattern, tests in glob_keys.items():
                if self._matches_blob_pattern(glob_pattern, pkg):
                    impacted.update(tests)

        if "*" in impacted:
            impacted.remove("*")
            impacted.update(self.get_indexed_tests_for_job(job_name))
        return impacted

    def impacted_tests_per_job(self, modified_packages: Iterable[str]) -> dict[str, set[str]]:
        """Determine impacted tests across all jobs for given package changes.

        Applies impact analysis to all jobs in the index, returning a comprehensive
        mapping of which tests should be executed in each job.

        Args:
            modified_packages: Iterable of package names that have been modified

        Returns:
            dict[str, set[str]]: Mapping of job names to sets of impacted test names.
                                Jobs with no impacted tests will have empty sets.
        """
        impacted: dict[str, set[str]] = {}

        for job_name in self._data:
            impacted[job_name] = self.impacted_tests(modified_packages, job_name)

        return impacted

    def skipped_tests(self, modified_packages: Iterable[str], job_name: str) -> set[str]:
        """Determine which indexed tests should be skipped for a specific job.

        Returns tests that are present in the index but are NOT impacted by the
        given package changes. These are tests that can be safely skipped since
        the changes don't affect the code they exercise.

        Args:
            modified_packages: Iterable of package names that have been modified
            job_name: CI job name to get skipped tests for

        Returns:
            set[str]: Set of test names that can be skipped.
                     Empty set if no tests can be skipped or job doesn't exist.
        """
        all_indexed_tests = self.get_indexed_tests_for_job(job_name)
        impacted_tests = self.impacted_tests(modified_packages, job_name)
        return all_indexed_tests - impacted_tests

    def skipped_tests_per_job(self, modified_packages: Iterable[str]) -> dict[str, set[str]]:
        """Determine which indexed tests should be skipped across all jobs.

        For each job, returns tests that are indexed but not impacted by the
        given package changes. This is the complement of impacted_tests_per_job().

        Args:
            modified_packages: Iterable of package names that have been modified

        Returns:
            dict[str, set[str]]: Mapping of job names to sets of tests that can be skipped.
                                Jobs with no skippable tests will have empty sets.
        """
        skipped: dict[str, set[str]] = {}

        for job_name in self._data:
            skipped[job_name] = self.skipped_tests(modified_packages, job_name)

        return skipped

    def triggering_paths(self, job_name: str, test_name: str) -> list[str]:
        """Determine the triggering path for a specific test.

        Args:
            job_name: Name of the CI containing the test being queried
            test_name: Name of the test to get the triggering path for
        """
        return list({pkg for pkg, tests in self._data.get(job_name, {}).items() if test_name in tests})
