import os
import unittest
from unittest.mock import patch

from invoke import Context, Exit, MockContext

from tasks.libs.build.bazel import bazel, package_from_path, split_label
from tasks.libs.common.utils import get_repo_root


class TestBazel(unittest.TestCase):
    def test_bazel_call(self):
        self.assertIsNone(bazel(Context(), "info", "release"))

    def test_bazel_output(self):
        expected_version = (get_repo_root() / ".bazelversion").read_text().strip()
        actual_output = bazel(Context(), "info", "release", capture_output=True).strip()
        self.assertEqual(actual_output, f"release {expected_version}")

    @patch.dict(os.environ, {"PATH": os.devnull})
    def test_bazel_not_found(self):
        with self.assertRaises(Exit) as cm:
            bazel(MockContext(), "info")
        self.assertIn("Please run `inv install-tools` for `bazel` support!", cm.exception.message)


class TestSplitLabel(unittest.TestCase):
    def test_no_repo(self):
        parts = split_label("//pkg/foo:bar")
        self.assertIsNone(parts.repo)
        self.assertEqual(parts.package, "pkg/foo")
        self.assertEqual(parts.name, "bar")

    def test_single_at_repo(self):
        parts = split_label("@myrepo//pkg/foo:bar")
        self.assertEqual(parts.repo, "myrepo")
        self.assertEqual(parts.package, "pkg/foo")
        self.assertEqual(parts.name, "bar")

    def test_double_at_repo(self):
        parts = split_label("@@myrepo//pkg/foo:bar")
        self.assertEqual(parts.repo, "myrepo")
        self.assertEqual(parts.package, "pkg/foo")
        self.assertEqual(parts.name, "bar")

    def test_root_package(self):
        parts = split_label("//:foo")
        self.assertIsNone(parts.repo)
        self.assertEqual(parts.package, "")
        self.assertEqual(parts.name, "foo")

    def test_root_package_with_repo(self):
        parts = split_label("@myrepo//:foo")
        self.assertEqual(parts.repo, "myrepo")
        self.assertEqual(parts.package, "")
        self.assertEqual(parts.name, "foo")

    def test_no_explicit_name(self):
        parts = split_label("//pkg/foo")
        self.assertIsNone(parts.repo)
        self.assertEqual(parts.package, "pkg/foo")
        self.assertIsNone(parts.name)

    def test_at_main_workspace(self):
        # "@//" and "@@//" both mean the main workspace — repo should be None
        parts_single = split_label("@//pkg:target")
        self.assertIsNone(parts_single.repo)
        parts_double = split_label("@@//pkg:target")
        self.assertIsNone(parts_double.repo)

    def test_deep_package(self):
        parts = split_label("@@com_github_foo//a/b/c/d:e")
        self.assertEqual(parts.repo, "com_github_foo")
        self.assertEqual(parts.package, "a/b/c/d")
        self.assertEqual(parts.name, "e")


class TestPackageFromPath(unittest.TestCase):
    def test_relative_path(self):
        self.assertEqual(package_from_path("pkg/foo"), "pkg/foo")

    def test_relative_path_dot_slash(self):
        self.assertEqual(package_from_path("./pkg/foo"), "pkg/foo")

    def test_relative_path_backslash(self):
        self.assertEqual(package_from_path("pkg\\foo"), "pkg/foo")

    def test_dot_is_root_package(self):
        # A bare "." represents the repo root — the Bazel root package is ""
        self.assertEqual(package_from_path("."), "")

    def test_absolute_path(self):
        repo_root = get_repo_root()
        abs_path = os.path.join(str(repo_root), "pkg", "foo")
        self.assertEqual(package_from_path(abs_path), "pkg/foo")

    def test_absolute_path_nested(self):
        repo_root = get_repo_root()
        abs_path = os.path.join(str(repo_root), "comp", "core", "config")
        self.assertEqual(package_from_path(abs_path), "comp/core/config")


if __name__ == "__main__":
    unittest.main()
