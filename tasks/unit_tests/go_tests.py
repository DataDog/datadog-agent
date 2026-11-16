import unittest
from unittest.mock import MagicMock, patch

from invoke.context import Context

from tasks.gotest import (
    _minimize_bazel_patterns,
    _target_to_bazel_pattern,
    find_impacted_packages,
    get_go_modified_files,
    get_modified_packages,
    should_run_all_tests,
)
from tasks.modules import GoModule


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


class TestGetModifiedPackages(unittest.TestCase):
    """Tests for the package selection used by `linter.go -o` and `test -o`."""

    @patch(
        "tasks.gotest.get_go_modified_files",
        new=MagicMock(return_value=["comp/api/api/def/foo.go", "comp/api/api/def/bar.go"]),
    )
    @patch("tasks.gotest.get_go_module", new=MagicMock(return_value="comp/api/api/def"))
    @patch("tasks.gotest.get_module_by_path", new=MagicMock(return_value=GoModule("comp/api/api/def")))
    def test_no_duplicate_target_for_files_in_same_package(self):
        # Several files in the same package of a nested module must yield a single
        # target, in both `test_targets` and `lint_targets`.
        modules = get_modified_packages(Context())

        self.assertEqual(len(modules), 1)
        self.assertEqual(modules[0].test_targets, ["./."])
        self.assertEqual(modules[0].lint_targets, ["./."])

    @patch("tasks.gotest.get_go_modified_files", new=MagicMock(return_value=["pkg/aggregator/foo.go"]))
    @patch("tasks.gotest.get_go_module", new=MagicMock(return_value="."))
    @patch(
        "tasks.gotest.get_module_by_path",
        new=MagicMock(return_value=GoModule(".", test_targets=["./pkg", "./cmd", "./comp"])),
    )
    def test_file_in_package_under_target_dir(self):
        modules = get_modified_packages(Context())

        self.assertEqual(len(modules), 1)
        self.assertEqual(modules[0].test_targets, ["./pkg/aggregator"])

    @patch("tasks.gotest.get_go_modified_files", new=MagicMock(return_value=["internal/somepkg/foo.go"]))
    @patch("tasks.gotest.get_go_module", new=MagicMock(return_value="."))
    @patch(
        "tasks.gotest.get_module_by_path",
        new=MagicMock(return_value=GoModule(".", test_targets=["./pkg", "./cmd", "./comp"])),
    )
    def test_not_targeted_when_directory_name_contains_target(self):
        # `internal/somepkg` must not match the `./pkg` target.
        modules = get_modified_packages(Context())

        self.assertEqual(modules, [])


class TestGetGoModifiedFiles(unittest.TestCase):
    @patch("tasks.gotest._get_release_json_value", new=MagicMock(return_value="main"))
    @patch("tasks.gotest.get_modified_files", new=MagicMock(return_value=["pkg/existing/foo.go", "docs/readme.md"]))
    @patch(
        "tasks.gotest.get_untracked_files",
        new=MagicMock(return_value=["pkg/newpkg/foo.go", "pkg/existing/foo.go", "pkg/newpkg/notes.txt"]),
    )
    def test_untracked_files_are_included(self):
        files = get_go_modified_files(Context())

        self.assertEqual(files, ["pkg/existing/foo.go", "pkg/newpkg/foo.go"])


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
