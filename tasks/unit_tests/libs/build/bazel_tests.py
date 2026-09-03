import os
import subprocess
import unittest
from unittest.mock import patch

from tasks.libs.build.bazel import bazel, package_from_path, split_label
from tasks.libs.common.utils import get_repo_root


class TestBazel(unittest.TestCase):
    def test_bazel_call(self):
        self.assertEqual(bazel("info", "release"), "")

    def test_bazel_output(self):
        expected_version = (get_repo_root() / ".bazelversion").read_text().strip()
        actual_output = bazel("info", "release", capture_output=True).strip()
        self.assertEqual(actual_output, f"release {expected_version}")

    @patch.dict(os.environ, {"PATH": os.devnull})
    def test_bazel_not_found(self):
        with self.assertRaises(SystemExit) as cm:
            bazel("info")
        self.assertIn("Please run `inv install-tools` for `bazel` support!", str(cm.exception.code))

    @patch("tasks.libs.build.bazel._run_command")
    @patch("tasks.libs.build.bazel.shutil.which", return_value="/bzlx")
    def test_capture_output(self, _, run_command):
        run_command.return_value = subprocess.CompletedProcess("/bzlx info", 0, "out\n", "")
        self.assertEqual(bazel("info", capture_output=True), "out\n")

    @patch("tasks.libs.build.bazel._run_command")
    @patch("tasks.libs.build.bazel.shutil.which", return_value="/bzlx")
    def test_ignore_errors_returns_completed_process(self, _, run_command):
        run_command.return_value = subprocess.CompletedProcess("/bzlx info", 1, "out\n", "err\n")
        res = bazel("info", ignore_errors=True, capture_output=True)
        self.assertEqual(res.returncode, 1)
        self.assertEqual(res.stdout, "out\n")
        self.assertEqual(res.stderr, "err\n")

    @patch("tasks.libs.build.bazel._run_command")
    @patch("tasks.libs.build.bazel.shutil.which", return_value="/bzlx")
    def test_input_disabled_by_default(self, _, run_command):
        run_command.return_value = subprocess.CompletedProcess("/bzlx info", 0, "", "")
        bazel("info")
        self.assertIsNone(run_command.call_args.kwargs["input"])

    @patch("tasks.libs.build.bazel._run_command")
    @patch("tasks.libs.build.bazel.shutil.which", return_value="/bzlx")
    def test_input_forwarding(self, _, run_command):
        run_command.return_value = subprocess.CompletedProcess("/bzlx info", 0, "", "")
        bazel("info", input="some input")
        self.assertEqual(run_command.call_args.kwargs["input"], "some input")

    @patch("tasks.libs.build.bazel._run_command")
    @patch("tasks.libs.build.bazel.shutil.which", return_value="/bzlx")
    @patch.dict(os.environ, {}, clear=True)
    def test_no_omnibazel_flag_to_insert(self, _, run_command):
        run_command.return_value = subprocess.CompletedProcess("/bzlx run //:go", 0, "", "")
        bazel("run", "//:go")
        self.assertEqual(run_command.call_args.args[0], ("/bzlx", "run", "//:go"))

    @patch("tasks.libs.build.bazel._run_command")
    @patch("tasks.libs.build.bazel.shutil.which", return_value="/bzlx")
    @patch.dict(os.environ, {"AGENT_FLAVOR": "fips", "INSTALL_DIR": "/opt"})
    def test_inserted_omnibazel_flags(self, _, run_command):
        run_command.return_value = subprocess.CompletedProcess(
            "/bzlx --batch run --//packages/agent:flavor=fips --//:install_dir=/opt --//:output_config_dir= //:go",
            0,
            "",
            "",
        )
        with patch("tasks.libs.build.bazel.sys.platform", "linux"):
            bazel("--batch", "run", "//:go")
        self.assertEqual(
            run_command.call_args.args[0],
            (
                "/bzlx",
                "--batch",
                "run",
                "--//packages/agent:flavor=fips",
                "--//:install_dir=/opt",
                "--//:output_config_dir=",
                "//:go",
            ),
        )


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
