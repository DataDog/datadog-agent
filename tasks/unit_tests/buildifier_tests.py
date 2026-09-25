import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import call, patch

from invoke.exceptions import Exit

import tasks.linter as linter


class TestBuildifierTask(unittest.TestCase):
    @staticmethod
    def result(*, stdout="", returncode=0):
        return subprocess.CompletedProcess([], returncode, stdout, "")

    def test_empty_selection_fails_closed(self):
        with (
            tempfile.TemporaryDirectory() as directory,
            patch.object(linter, "get_repo_root", return_value=Path(directory)),
            patch.object(linter.subprocess, "run", return_value=self.result(stdout="README.md\0")) as run,
            self.assertRaises(Exit) as raised,
        ):
            linter.buildifier.body(None)
        self.assertEqual(raised.exception.code, 2)
        run.assert_called_once()

    def test_late_batching_error_prevents_all_buildifier_execution(self):
        files = [f"./file-{index}.bzl" for index in range(100)] + ["./" + "z" * 8_000 + ".bzl"]
        with (
            tempfile.TemporaryDirectory() as directory,
            patch.object(linter, "get_repo_root", return_value=Path(directory)),
            patch.object(linter, "select_buildifier_files", return_value=files),
            patch.object(linter.subprocess, "run", return_value=self.result(stdout="a.bzl\0")) as run,
            self.assertRaises(Exit) as raised,
        ):
            linter.buildifier.body(None, fix=True)
        self.assertEqual(raised.exception.code, 2)
        self.assertIn("exceeds", str(raised.exception))
        run.assert_called_once()

    def test_platform_commands_preserve_literal_paths_and_use_the_checkout_launcher(self):
        original_cwd = Path.cwd()
        for on_windows in (False, True):
            for fix in (False, True):
                with (
                    self.subTest(on_windows=on_windows, fix=fix),
                    tempfile.TemporaryDirectory(prefix="buildifier %PATH% & ") as directory,
                    patch("tasks.libs.common.utils.is_windows", return_value=on_windows),
                    patch.object(linter, "get_repo_root", return_value=Path(directory)),
                    patch.object(linter, "select_buildifier_files", return_value=["./nested/a %PATH% & file.bzl"]),
                    patch.object(linter.subprocess, "run", return_value=self.result(stdout="file.bzl\0")) as run,
                ):
                    linter.buildifier.body(None, fix=fix)
                    executable = Path(directory) / "tools/bin" / ("buildifier.exe" if on_windows else "buildifier")
                    expected = [
                        str(executable),
                        "-mode=fix" if fix else "-mode=diff",
                        "-lint=fix" if fix else "-lint=warn",
                    ]
                    if not fix:
                        expected.append("-diff_command=FC" if on_windows else "-diff_command=diff --unified")
                    expected.append(".\\nested\\a %PATH% & file.bzl" if on_windows else "./nested/a %PATH% & file.bzl")
                    self.assertEqual(
                        run.call_args_list,
                        [
                            call(
                                [
                                    "git",
                                    "-C",
                                    directory,
                                    "ls-files",
                                    "-z",
                                    "--cached",
                                    "--others",
                                    "--exclude-standard",
                                ],
                                capture_output=True,
                                encoding="utf-8",
                                check=True,
                            ),
                            call(expected, cwd=Path(directory), stdin=subprocess.DEVNULL, check=False),
                        ],
                    )
                    self.assertEqual(Path.cwd(), original_cwd)

    def test_every_batch_runs_and_any_nonzero_exit_fails_the_task(self):
        commands = [["buildifier", f"./file-{index}.bzl"] for index in range(3)]
        with (
            tempfile.TemporaryDirectory() as directory,
            patch.object(linter, "get_repo_root", return_value=Path(directory)),
            patch.object(linter, "select_buildifier_files", return_value=["./BUILD.bazel"]),
            patch.object(linter, "buildifier_commands", return_value=commands),
            patch.object(
                linter.subprocess,
                "run",
                side_effect=[
                    self.result(stdout="BUILD.bazel\0"),
                    *[self.result(returncode=code) for code in (4, 0, 1)],
                ],
            ) as run,
            self.assertRaises(Exit) as raised,
        ):
            linter.buildifier.body(None, fix=True)
        self.assertEqual(raised.exception.code, 1)
        self.assertEqual(
            run.call_args_list[1:],
            [call(command, cwd=Path(directory), stdin=subprocess.DEVNULL, check=False) for command in commands],
        )

    def test_git_failures_stop_before_buildifier_and_preserve_diagnostics(self):
        for error in (
            FileNotFoundError("git unavailable"),
            subprocess.CalledProcessError(1, ["git"], stderr="repository unavailable"),
        ):
            with (
                self.subTest(error=error),
                patch.object(linter.subprocess, "run", side_effect=error) as run,
                self.assertRaisesRegex(Exit, "unavailable") as raised,
            ):
                linter.buildifier.body(None)
            self.assertEqual(raised.exception.code, 1)
            run.assert_called_once()

    def test_unavailable_launcher_preserves_diagnostics(self):
        with (
            patch.object(linter, "select_buildifier_files", return_value=["./BUILD.bazel"]),
            patch.object(
                linter.subprocess,
                "run",
                side_effect=[self.result(stdout="BUILD.bazel\0"), FileNotFoundError("launcher unavailable")],
            ),
            self.assertRaisesRegex(Exit, "launcher unavailable") as raised,
        ):
            linter.buildifier.body(None)
        self.assertEqual(raised.exception.code, 1)


if __name__ == "__main__":
    unittest.main()
