import json
import os
import tempfile
import unittest

from tasks.libs.dynamic_test.index import DynamicTestIndex


class TestDynamicTestIndex(unittest.TestCase):
    def test_normalize_deduplicates_preserves_order(self):
        data = {"job": {"pkg": ["t1", "t2", "t1", "t3", "t2"]}}
        idx = DynamicTestIndex.from_dict(data)
        self.assertEqual(idx.to_dict(), {"job": {"pkg": ["t1", "t2", "t3"]}})

    def test_normalize_handles_none_tests(self):
        data = {"job": {"pkg": None}}
        idx = DynamicTestIndex.from_dict(data)
        self.assertEqual(idx.to_dict(), {"job": {"pkg": []}})

    def test_get_tests_for_job_returns_deepcopy(self):
        idx = DynamicTestIndex.from_dict({"job": {"pkg": ["t1"]}})
        view = idx.get_tests_for_job("job")
        # mutate the returned value
        view["pkg"].append("tX")
        # ensure internal state isn't affected
        self.assertEqual(idx.get_tests_for_job("job"), {"pkg": ["t1"]})

    def test_get_tests_for_unknown_job(self):
        idx = DynamicTestIndex()
        self.assertEqual(idx.get_tests_for_job("nope"), {})

    def test_add_tests_creates_job_and_package_and_dedups(self):
        idx = DynamicTestIndex()
        idx.add_tests("job", "pkg", ["t2", "t1", "t2"])  # dedup within call
        idx.add_tests("job", "pkg", ["t3", "t1"])  # dedup across calls, preserve order
        self.assertEqual(idx.to_dict(), {"job": {"pkg": ["t2", "t1", "t3"]}})

    def test_merge_combines_indexes(self):
        a = DynamicTestIndex.from_dict({"job": {"pkg1": ["a"], "pkg2": ["b"]}})
        b = DynamicTestIndex.from_dict({"job": {"pkg1": ["c", "a"]}, "job2": {"pkgX": ["z"]}})
        a.merge(b)
        self.assertEqual(
            a.to_dict(),
            {"job": {"pkg1": ["a", "c"], "pkg2": ["b"]}, "job2": {"pkgX": ["z"]}},
        )

    def test_impacted_tests_specific_job(self):
        idx = DynamicTestIndex.from_dict(
            {
                "job": {"pkgA": ["t1", "t2"], "pkgB": ["t3"]},
                "job2": {"pkgA": ["t9"]},
            }
        )
        impacted = idx.impacted_tests(["pkgA", "pkgC"], "job")
        self.assertEqual(set(impacted), {"t1", "t2"})

    def test_impacted_packages_per_job(self):
        idx = DynamicTestIndex.from_dict(
            {
                "job": {"pkgA": ["t1", "t2"], "pkgB": ["t3"]},
                "job2": {"pkgA": ["t9"]},
            }
        )
        per_job = idx.impacted_tests_per_job(["pkgA", "pkgB"])
        # order is not guaranteed, compare as sets
        self.assertEqual(set(per_job.keys()), {"job", "job2"})
        self.assertEqual(set(per_job["job"]), {"t1", "t2", "t3"})
        self.assertEqual(set(per_job["job2"]), {"t9"})

    def test_skipped_tests_specific_job(self):
        idx = DynamicTestIndex.from_dict(
            {
                "job": {"pkgA": ["t1", "t2"], "pkgB": ["t3"], "pkgC": ["t4", "t5"]},
                "job2": {"pkgA": ["t9"], "pkgD": ["t10"]},
            }
        )
        # Modify pkgA - should skip tests from pkgB and pkgC for job
        skipped = idx.skipped_tests(["pkgA"], "job")
        self.assertEqual(skipped, {"t3", "t4", "t5"})

        # Modify pkgA and pkgB - should skip tests from pkgC only for job
        skipped = idx.skipped_tests(["pkgA", "pkgB"], "job")
        self.assertEqual(skipped, {"t4", "t5"})

        # Modify all packages - no tests should be skipped
        skipped = idx.skipped_tests(["pkgA", "pkgB", "pkgC"], "job")
        self.assertEqual(skipped, set())

        # Unknown job should return empty set
        skipped = idx.skipped_tests(["pkgA"], "unknown")
        self.assertEqual(skipped, set())

    def test_skipped_tests_per_job(self):
        idx = DynamicTestIndex.from_dict(
            {
                "job": {"pkgA": ["t1", "t2"], "pkgB": ["t3"], "pkgC": ["t4"]},
                "job2": {"pkgA": ["t9"], "pkgD": ["t10"]},
            }
        )
        per_job = idx.skipped_tests_per_job(["pkgA"])

        # Check that we get results for both jobs
        self.assertEqual(set(per_job.keys()), {"job", "job2"})

        # For job: pkgA is modified, so tests from pkgB and pkgC should be skipped
        self.assertEqual(per_job["job"], {"t3", "t4"})

        # For job2: pkgA is modified, so test from pkgD should be skipped
        self.assertEqual(per_job["job2"], {"t10"})

    def test_skipped_tests_complement_impacted_tests(self):
        """Verify that skipped + impacted = all indexed tests for each job."""
        idx = DynamicTestIndex.from_dict(
            {
                "job": {"pkgA": ["t1", "t2"], "pkgB": ["t3"], "pkgC": ["t4", "t5"]},
                "job2": {"pkgA": ["t9"], "pkgD": ["t10", "t11"]},
            }
        )
        changes = ["pkgA", "pkgC"]

        for job_name in ["job", "job2"]:
            all_tests = idx.get_indexed_tests_for_job(job_name)
            impacted = idx.impacted_tests(changes, job_name)
            skipped = idx.skipped_tests(changes, job_name)

            # Verify no overlap between impacted and skipped
            self.assertEqual(impacted & skipped, set())

            # Verify impacted + skipped = all tests
            self.assertEqual(impacted | skipped, all_tests)

    def test_dump_json_writes_file_and_parent_dirs(self):
        idx = DynamicTestIndex.from_dict({"job": {"pkg": ["t1", "t2"]}})
        with tempfile.TemporaryDirectory() as tmp:
            nested_dir = os.path.join(tmp, "nested", "dir")
            target = os.path.join(nested_dir, "index.json")
            idx.dump_json(target)
            self.assertTrue(os.path.isfile(target))
            with open(target, encoding="utf-8") as f:
                loaded = json.load(f)
            self.assertEqual(loaded, idx.to_dict())


if __name__ == "__main__":
    unittest.main()
