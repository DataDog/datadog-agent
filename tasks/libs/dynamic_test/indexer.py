"""
Dynamic test indexers.

- DynTestIndexer: abstract interface to produce a DynamicTestIndex
- CoverageDynTestIndexer: builds index from Go coverage output folders

Expected coverage folder layout:
coverage_root/
  <suite_or_folder_name>/
    metadata.json           # contains at least job_name and test (optional)
    coverage/               # go tool covdata directory

Resulting index format (see DynamicTestIndex):
{
  <job-name>: {
    <package-name>: ["test1", "test2", ...]
  }
}
"""

from __future__ import annotations

from abc import ABC, abstractmethod

from invoke import Context

from tasks.libs.dynamic_test.index import DynamicTestIndex


class DynTestIndexer(ABC):
    """Abstract base class for building dynamic test indexes.

    Defines the interface for generating DynamicTestIndex objects from various
    data sources such as test coverage, execution traces, or static analysis.

    An indexer's responsibility is to:
    1. Analyze available data sources (coverage files, execution logs, etc.)
    2. Build reverse mappings from code components to tests that exercise them
    3. Return a properly structured DynamicTestIndex

    Implementations should:
    - Handle missing or malformed input data gracefully
    - Generate consistent test and package identifiers
    - Provide progress feedback for long-running operations
    - Support different test frameworks and languages as needed

    Common indexer types:
    - Coverage-based: Uses test coverage data to map packages to tests
    - Execution-based: Uses test execution logs to build mappings
    - Static analysis-based: Uses code analysis to predict test relevance
    """

    @abstractmethod
    def compute_index(self, ctx: Context) -> DynamicTestIndex:
        """Build and return a DynamicTestIndex from the configured data source.

        This method should:
        1. Process the configured data source (files, directories, APIs)
        2. Extract relationships between code components and tests
        3. Build a reverse index mapping packages/components to tests
        4. Return a properly structured DynamicTestIndex

        Args:
            ctx: Invoke context for running shell commands and operations

        Returns:
            DynamicTestIndex: The computed index with job->package->tests mappings

        Raises:
            FileNotFoundError: If required input data sources are missing
            ValueError: If input data is malformed or cannot be processed
            RuntimeError: If index computation fails due to system issues

        Note:
            Implementations should log progress and warnings for debugging.
            The returned index should be immediately usable by DynTestExecutor.
        """
        raise NotImplementedError
