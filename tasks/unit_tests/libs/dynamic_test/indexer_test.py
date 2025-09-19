import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import MagicMock

from tasks.libs.dynamic_test.indexers.e2e import (
    DiffedPackageCoverageDynTestIndexer,
    FileCoverageDynTestIndexer,
    PackageCoverageDynTestIndexer,
)


class TestPackageCoverageDynTestIndexer(unittest.TestCase):
    def test_compute_index_raises_on_missing_root(self):
        idxr = PackageCoverageDynTestIndexer(coverage_root="/path/that/does/not/exist")
        with self.assertRaises(FileNotFoundError):
            idxr.compute_index(ctx=MagicMock())

    def test_compute_index_builds_reverse_index(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)

            # Suite 1 with metadata specifying job and test names
            suite1 = root / "suite1"
            (suite1 / "coverage").mkdir(parents=True)
            with open(suite1 / "metadata.json", "w", encoding="utf-8") as f:
                json.dump({"job_name": "job1", "test": "TestA"}, f)

            # Suite 2 without metadata: should fall back to folder name
            suite2 = root / "suite2"
            (suite2 / "coverage").mkdir(parents=True)

            # Mock ctx.run to generate coverage.txt files
            def fake_run(cmd, echo=False, warn=True):  # noqa: U100
                # retrieving out and in path from go tool covdata textfmt -i <input> -o <output> command
                out_path = None
                in_path = None
                for token in cmd.split():
                    if token.startswith("-o="):
                        out_path = token[len("-o=") :]
                    if token.startswith("-i="):
                        in_path = token[len("-i=") :]
                self.assertIsNotNone(out_path)
                # Choose content based on which suite coverage dir was used
                if in_path and "suite1" in in_path:
                    content = "\n".join(
                        [
                            # covered (>0)
                            "github.com/DataDog/datadog-agent/pkg/collector/corechecks/check.go:24.13,25.2 2 1",
                            # not covered (=0)
                            "github.com/DataDog/datadog-agent/pkg/collector/corechecks/other.go:10.1,12.2 1 0",
                            # header-like line ignored
                            "mode: set",
                        ]
                    )
                else:
                    content = "\n".join(
                        [
                            "github.com/DataDog/datadog-agent/pkg/util/log/log.go:5.1,6.2 1 3",
                            "github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe.go:1.1,2.2 1 2",
                        ]
                    )
                # Write output file
                os.makedirs(os.path.dirname(out_path), exist_ok=True)
                with open(out_path, "w", encoding="utf-8") as f:
                    f.write(content)
                # Return an object with .ok-like behavior if needed; not used here
                return MagicMock()

            ctx = MagicMock()
            ctx.run.side_effect = fake_run

            idxr = PackageCoverageDynTestIndexer(coverage_root=str(root))
            index = idxr.compute_index(ctx)

            result = index.to_dict()

            # Expected packages extracted from the content above
            # Package paths are derived starting from after the module root and dropping filename
            expected = {
                "job1": {"pkg/collector/corechecks": ["TestA"]},
                "suite2": {
                    "pkg/util/log": ["suite2"],
                    "pkg/collector/corechecks/ebpf": ["suite2"],
                },
            }

            # Compare sets as order isn't strictly guaranteed for lists built from sets
            self.assertEqual(set(result.keys()), set(expected.keys()))
            for job in expected:
                self.assertIn(job, result)
                self.assertEqual(set(result[job].keys()), set(expected[job].keys()))
                for pkg in expected[job]:
                    self.assertIn(pkg, result[job])
                    self.assertEqual(result[job][pkg], expected[job][pkg])


class TestFileCoverageDynTestIndexer(unittest.TestCase):
    def test_compute_index_raises_on_missing_root(self):
        idxr = FileCoverageDynTestIndexer(coverage_root="/path/that/does/not/exist")
        with self.assertRaises(FileNotFoundError):
            idxr.compute_index(ctx=MagicMock())

    def test_compute_index_builds_reverse_index(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)

            # Suite 1 with metadata specifying job and test names
            suite1 = root / "suite1"
            (suite1 / "coverage").mkdir(parents=True)
            with open(suite1 / "metadata.json", "w", encoding="utf-8") as f:
                json.dump({"job_name": "job1", "test": "TestA"}, f)

            # Suite 2 without metadata: should fall back to folder name
            suite2 = root / "suite2"
            (suite2 / "coverage").mkdir(parents=True)

            # Mock ctx.run to generate coverage.txt files
            def fake_run(cmd, echo=False, warn=True):  # noqa: U100
                # retrieving out and in path from go tool covdata textfmt -i <input> -o <output> command
                out_path = None
                in_path = None
                for token in cmd.split():
                    if token.startswith("-o="):
                        out_path = token[len("-o=") :]
                    if token.startswith("-i="):
                        in_path = token[len("-i=") :]
                self.assertIsNotNone(out_path)
                # Choose content based on which suite coverage dir was used
                if in_path and "suite1" in in_path:
                    content = "\n".join(
                        [
                            # covered (>0)
                            "github.com/DataDog/datadog-agent/pkg/collector/corechecks/check.go:24.13,25.2 2 1",
                            # not covered (=0)
                            "github.com/DataDog/datadog-agent/pkg/collector/corechecks/other.go:10.1,12.2 1 0",
                            # header-like line ignored
                            "mode: set",
                        ]
                    )
                else:
                    content = "\n".join(
                        [
                            "github.com/DataDog/datadog-agent/pkg/util/log/log.go:5.1,6.2 1 3",
                            "github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe.go:1.1,2.2 1 2",
                        ]
                    )
                # Write output file
                os.makedirs(os.path.dirname(out_path), exist_ok=True)
                with open(out_path, "w", encoding="utf-8") as f:
                    f.write(content)
                # Return an object with .ok-like behavior if needed; not used here
                return MagicMock()

            ctx = MagicMock()
            ctx.run.side_effect = fake_run

            idxr = FileCoverageDynTestIndexer(coverage_root=str(root))
            index = idxr.compute_index(ctx)

            result = index.to_dict()

            # Expected files extracted from the content above
            # File paths are derived starting from after the module root and keeping .go extension
            expected = {
                "job1": {"pkg/collector/corechecks/check.go": ["TestA"]},
                "suite2": {
                    "pkg/util/log/log.go": ["suite2"],
                    "pkg/collector/corechecks/ebpf/probe.go": ["suite2"],
                },
            }

            # Compare sets as order isn't strictly guaranteed for lists built from sets
            self.assertEqual(set(result.keys()), set(expected.keys()))
            for job in expected:
                self.assertIn(job, result)
                self.assertEqual(set(result[job].keys()), set(expected[job].keys()))
                for file_path in expected[job]:
                    self.assertIn(file_path, result[job])
                    self.assertEqual(result[job][file_path], expected[job][file_path])


class TestDiffedPackageCoverageDynTestIndexer(unittest.TestCase):
    def test_compute_index_builds_differential_index(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            baseline_root = Path(tmp) / "baseline"

            # Create baseline coverage
            baseline_suite = baseline_root / "coverage"
            baseline_suite.mkdir(parents=True)

            # Create baseline coverage.txt with some coverage
            baseline_coverage_txt = baseline_suite / "coverage.txt"
            with open(baseline_coverage_txt, "w", encoding="utf-8") as f:
                f.write("github.com/DataDog/datadog-agent/pkg/collector/corechecks/check.go:24.13,25.2 2 1\n")

            # Create current coverage suite
            suite1 = root / "suite1"
            (suite1 / "coverage").mkdir(parents=True)
            with open(suite1 / "metadata.json", "w", encoding="utf-8") as f:
                json.dump({"job_name": "job1", "test": "TestA"}, f)

            # Mock ctx.run to generate coverage.txt files
            def fake_run(cmd, echo=False, warn=True):  # noqa: U100
                if "baseline" in cmd:
                    content = "\n".join(
                        [
                            "github.com/DataDog/datadog-agent/pkg/collector/coretoto/check.go:24.13,25.2 2 1",
                            "github.com/DataDog/datadog-agent/pkg/collector/corechecks/check.go:26.13,27.2 1 0",
                        ]
                    )
                    out_path = baseline_suite / "coverage.txt"
                    with open(out_path, "w", encoding="utf-8") as f:
                        f.write(content)
                    return MagicMock()
                if "suite1" in cmd:
                    # Current coverage has additional lines not in baseline
                    content = "\n".join(
                        [
                            "github.com/DataDog/datadog-agent/pkg/collector/coretoto/check.go:24.13,25.2 2 1",  # Same as baseline
                            "github.com/DataDog/datadog-agent/pkg/collector/corechecks/check.go:26.13,27.2 2 1",  # New coverage
                            "github.com/DataDog/datadog-agent/pkg/util/log/log.go:5.1,6.2 1 1",  # New file
                        ]
                    )
                    out_path = suite1 / "coverage" / "coverage.txt"
                    with open(out_path, "w", encoding="utf-8") as f:
                        f.write(content)
                return MagicMock()

            ctx = MagicMock()
            ctx.run.side_effect = fake_run

            idxr = DiffedPackageCoverageDynTestIndexer(
                coverage_root=str(root), baseline_coverage_root=str(baseline_root)
            )
            index = idxr.compute_index(ctx)

            result = index.to_dict()

            # Expected: only packages with new coverage ranges should be included
            expected = {
                "job1": {
                    "pkg/collector/corechecks": ["TestA"],  # Has new coverage range 26.13,27.2
                    "pkg/util/log": ["TestA"],  # Completely new file
                },
            }

            self.assertEqual(set(result.keys()), set(expected.keys()))
            for job in expected:
                self.assertIn(job, result)
                self.assertEqual(set(result[job].keys()), set(expected[job].keys()))
                for pkg in expected[job]:
                    self.assertIn(pkg, result[job])
                    self.assertEqual(result[job][pkg], expected[job][pkg])


if __name__ == "__main__":
    unittest.main()
