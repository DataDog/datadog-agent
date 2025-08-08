"""
Dynamic Test Index model and helpers.

Index format:
{
  <job-name>: {
     <package-name>: ["test1", "test2", ...]
  }
}
"""

import json
import os
from collections.abc import Iterable
from typing import Self

IndexDict = dict[str, dict[str, list[str]]]


class DynamicTestIndex:
    """Represents the dynamic test reverse index.

    Structure: { job_name: { package: [tests...] } }
    """

    def __init__(self, data: IndexDict | None = None) -> None:
        self._data: IndexDict = {}
        if data:
            self._data = self._normalize(data)

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

    def get_tests_for_job(self, job_name: str) -> dict[str, list[str]]:
        return json.loads(json.dumps(self._data.get(job_name, {})))

    # --- Mutation helpers ---

    def add_tests(self, job_name: str, package: str, tests: Iterable[str]) -> None:
        if job_name not in self._data:
            self._data[job_name] = {}
        if package not in self._data[job_name]:
            self._data[job_name][package] = []
        existing = set(self._data[job_name][package])
        for t in tests:
            if t not in existing:
                self._data[job_name][package].append(t)
                existing.add(t)

    def merge(self, other: Self) -> None:
        """Merge another index into this one (in-place)."""
        for job_name, pkg_map in other._data.items():
            for package, tests in pkg_map.items():
                self.add_tests(job_name, package, tests)

    # --- Impact computation ---

    def impacted_tests(self, modified_packages: Iterable[str], job_name: str) -> list[str]:
        """Return tests impacted by modified packages.

        If job_name is provided, restrict to that job. Otherwise, collect across all jobs.
        """
        impacted: set[str] = set()

        job_map = self._data.get(job_name, {})
        for pkg in modified_packages:
            if pkg in job_map:
                impacted.update(job_map[pkg])
        return list(impacted)

    def impacted_packages_per_job(self, modified_packages: Iterable[str]) -> dict[str, list[str]]:
        """Return packages impacted by modified packages per job.

        Collect impacted test across all jobs. Return a map of job name to impacted tests.
        """
        impacted: dict[str, set[str]] = {}

        for job_name, job_map in self._data.items():
            impacted[job_name] = set()
            for pkg in modified_packages:
                if pkg in job_map:
                    impacted[job_name].update(job_map[pkg])

        return {job_name: list(tests) for job_name, tests in impacted.items()}
