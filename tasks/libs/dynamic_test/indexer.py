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
    """Abstract indexer producing a DynamicTestIndex."""

    @abstractmethod
    def compute_index(self, ctx: Context) -> DynamicTestIndex:
        """Compute and return a DynamicTestIndex."""
        raise NotImplementedError
