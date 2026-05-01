import unittest
from unittest.mock import MagicMock, patch

from tasks.gotest import (
    _minimize_bazel_patterns,
    _parse_bazel_test_line,
    _target_to_bazel_pattern,
    find_impacted_packages,
    should_run_all_tests,
)


class TestUtils(unittest.TestCase):
    def test_impacted_packages_1(self):
        dependencies = {
            "pkg1": [
                "pkg2",
            ],
            "pkg2": ["pkg3"],
            "pkg3": [],
        }
        changed_files = {"pkg1"}
        expected_impacted_packages = {"pkg1", "pkg2", "pkg3"}
        self.assertEqual(find_impacted_packages(dependencies, changed_files), expected_impacted_packages)

    def test_impacted_packages_2(self):
        dependencies = {
            "pkg1": ["pkg2", "pkg3"],
            "pkg2": ["pkg4"],
            "pkg3": ["pkg4"],
            "pkg4": [],
        }
        changed_files = {"pkg1"}
        expected_impacted_packages = {"pkg1", "pkg2", "pkg3", "pkg4"}
        self.assertEqual(find_impacted_packages(dependencies, changed_files), expected_impacted_packages)

    def test_impacted_packages_3(self):
        dependencies = {
            "pkg1": ["pkg2"],
            "pkg2": ["pkg1"],
        }
        changed_files = {"pkg1"}
        expected_impacted_packages = {"pkg1", "pkg2"}
        self.assertEqual(find_impacted_packages(dependencies, changed_files), expected_impacted_packages)

    def test_impacted_packages_4(self):
        dependencies = {
            "pkg1": ["pkg2"],
            "pkg2": ["pkg3"],
            "pkg3": [],
        }
        changed_files = {"pkg3"}
        expected_impacted_packages = {"pkg3"}
        self.assertEqual(find_impacted_packages(dependencies, changed_files), expected_impacted_packages)

    @patch("tasks.gotest._get_release_json_value", new=MagicMock())
    @patch("tasks.gotest.get_modified_files", new=MagicMock(return_value=["pkg/foo.go", "pkg/bar.go"]))
    def test_should_run_all_tests_1(self):
        trigger_files = ["pkg/foo.go"]

        self.assertTrue(should_run_all_tests(None, trigger_files))

    @patch("tasks.gotest._get_release_json_value", new=MagicMock())
    @patch("tasks.gotest.get_modified_files", new=MagicMock(return_value=["pkg/toto/bar.go"]))
    def test_should_run_all_tests_2(self):
        trigger_files = ["pkg/*"]

        self.assertTrue(should_run_all_tests(None, trigger_files))

    @patch("tasks.gotest._get_release_json_value", new=MagicMock())
    @patch("tasks.gotest.get_modified_files", new=MagicMock(return_value=["pkg/foo.go", "pkg/bar.go"]))
    def test_should_run_all_tests_3(self):
        trigger_files = ["pkg/toto/bar.go"]

        self.assertFalse(should_run_all_tests(None, trigger_files))

    @patch("tasks.gotest._get_release_json_value", new=MagicMock())
    @patch("tasks.gotest.get_modified_files", new=MagicMock(return_value=["pkg/foo.go", "pkg/bar.go"]))
    def test_should_run_all_tests_4(self):
        trigger_files = ["pkgs/*"]

        self.assertFalse(should_run_all_tests(None, trigger_files))


class TestTargetToBazelPattern(unittest.TestCase):
    def test_dot(self):
        self.assertEqual(_target_to_bazel_pattern('.'), '//...')

    def test_dot_slash(self):
        self.assertEqual(_target_to_bazel_pattern('./'), '//...')

    def test_simple_path(self):
        self.assertEqual(_target_to_bazel_pattern('./pkg/util'), '//pkg/util/...')

    def test_trailing_slash(self):
        self.assertEqual(_target_to_bazel_pattern('./pkg/util/'), '//pkg/util/...')

    def test_multiple_trailing_slashes(self):
        self.assertEqual(_target_to_bazel_pattern('./pkg/util//'), '//pkg/util/...')

    def test_already_recursive(self):
        self.assertEqual(_target_to_bazel_pattern('./pkg/...'), '//pkg/...')

    def test_no_leading_dot_slash(self):
        self.assertEqual(_target_to_bazel_pattern('pkg/util'), '//pkg/util/...')

    def test_nested_path(self):
        self.assertEqual(_target_to_bazel_pattern('./comp/core/config'), '//comp/core/config/...')


class TestMinimizeBazelPatterns(unittest.TestCase):
    def test_empty(self):
        self.assertEqual(_minimize_bazel_patterns([]), [])

    def test_no_overlap(self):
        result = _minimize_bazel_patterns(['//pkg/...', '//cmd/...'])
        self.assertEqual(sorted(result), ['//cmd/...', '//pkg/...'])

    def test_child_removed(self):
        result = _minimize_bazel_patterns(['//comp/...', '//comp/core/...'])
        self.assertEqual(result, ['//comp/...'])

    def test_deeply_nested_removed(self):
        result = _minimize_bazel_patterns(['//pkg/...', '//pkg/util/log/...', '//pkg/util/...'])
        self.assertEqual(result, ['//pkg/...'])

    def test_sibling_patterns_kept(self):
        result = _minimize_bazel_patterns(['//comp/...', '//pkg/...'])
        self.assertEqual(sorted(result), ['//comp/...', '//pkg/...'])

    def test_global_wildcard_subsumes_all(self):
        result = _minimize_bazel_patterns(['//...', '//pkg/...', '//comp/core/...'])
        self.assertEqual(result, ['//...'])

    def test_duplicates_removed(self):
        result = _minimize_bazel_patterns(['//pkg/...', '//pkg/...', '//comp/...'])
        self.assertEqual(sorted(result), ['//comp/...', '//pkg/...'])

    def test_mixed_overlap_and_siblings(self):
        patterns = ['//comp/...', '//comp/core/...', '//pkg/...', '//pkg/util/log/...', '//cmd/...']
        result = _minimize_bazel_patterns(patterns)
        self.assertEqual(sorted(result), ['//cmd/...', '//comp/...', '//pkg/...'])


class TestParseBazelTestLine(unittest.TestCase):
    def test_passed_with_timing(self):
        line = '//pkg/util/log:log_test                                     PASSED in 0.521s'
        label, status, timing, cached = _parse_bazel_test_line(line)
        self.assertEqual(label, '//pkg/util/log:log_test')
        self.assertEqual(status, 'PASSED')
        self.assertEqual(timing, '0.521s')
        self.assertFalse(cached)

    def test_failed_with_timing(self):
        line = '//pkg/process/util:util_test                                FAILED in 0.345s'
        label, status, timing, cached = _parse_bazel_test_line(line)
        self.assertEqual(label, '//pkg/process/util:util_test')
        self.assertEqual(status, 'FAILED')
        self.assertEqual(timing, '0.345s')
        self.assertFalse(cached)

    def test_skipped_no_timing(self):
        line = '//pkg/api/security:security_test                            SKIPPED'
        label, status, timing, cached = _parse_bazel_test_line(line)
        self.assertEqual(label, '//pkg/api/security:security_test')
        self.assertEqual(status, 'SKIPPED')
        self.assertIsNone(timing)
        self.assertFalse(cached)

    def test_cached_passed(self):
        line = '//pkg/aggregator/ckey:ckey_test                             (cached) PASSED in 1.234s'
        label, status, timing, cached = _parse_bazel_test_line(line)
        self.assertEqual(label, '//pkg/aggregator/ckey:ckey_test')
        self.assertEqual(status, 'PASSED')
        self.assertEqual(timing, '1.234s')
        self.assertTrue(cached)

    def test_leading_whitespace_ignored(self):
        line = '   //pkg/util/log:log_test PASSED in 0.1s'
        result = _parse_bazel_test_line(line)
        self.assertIsNotNone(result)
        self.assertEqual(result[1], 'PASSED')

    def test_info_line_returns_none(self):
        self.assertIsNone(_parse_bazel_test_line('INFO: Build completed successfully, 1 total action'))

    def test_analyzing_line_returns_none(self):
        self.assertIsNone(_parse_bazel_test_line('Analyzing: 8 targets (0 packages loaded, 0 targets configured)'))

    def test_empty_line_returns_none(self):
        self.assertIsNone(_parse_bazel_test_line(''))

    def test_non_label_line_returns_none(self):
        self.assertIsNone(_parse_bazel_test_line('Computing main repo mapping:'))
