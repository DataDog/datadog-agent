import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import MagicMock, call, patch

from invoke.exceptions import Exit

import tasks.linter as linter
from tasks.libs.common.utils import join_command
from tasks.libs.linter.buildifier import buildifier_commands, is_buildifier_path, select_buildifier_files


class TestBuildifierPaths(unittest.TestCase):
    def test_supported_filenames(self):
        paths = [
            "BUILD",
            "pkg/BUILD.bazel",
            "pkg/MODULE.bazel",
            "pkg/REPO.bazel",
            "pkg/WORKSPACE",
            "pkg/WORKSPACE.bazel",
            "pkg/WORKSPACE.bzlmod",
            "pkg/WORKSPACE.oss",
            "pkg/defs.bzl",
            "pkg/defs.sky",
            "pkg/defs.star",
            "pkg/platform.BUILD",
            "pkg/platform.BUILD.bazel",
            "pkg/BUILD.windows.bazel",
            "pkg/BUILD.windows.oss",
            "pkg/platform.MODULE.bazel",
            "pkg/WORKSPACE.windows.bazel",
            "pkg/WORKSPACE.windows.oss",
        ]

        for path in paths:
            with self.subTest(path=path):
                self.assertTrue(is_buildifier_path(path))

    def test_near_misses_and_case_changes_are_not_supported(self):
        paths = [
            "build",
            "pkg/module.bazel",
            "pkg/MODULE",
            "pkg/foo.build",
            "pkg/WORKSPACE.windows",
            "pkg/README.bzl.txt",
        ]

        for path in paths:
            with self.subTest(path=path):
                self.assertFalse(is_buildifier_path(path))

    def test_repository_exclusions_are_applied_case_sensitively(self):
        excluded = [
            ".hidden.bzl",
            ".github/BUILD.bazel",
            "third_party/project/BUILD",
            "vendor/project/defs.bzl",
            "deps/project/overlay/defs.bzl",
            "deps/a/b/overlay/defs.bzl",
        ]
        included = [
            "pkg/.hidden/defs.bzl",
            "deps/overlay/defs.bzl",
            "deps/project/overlay.bzl",
            "Vendor/project/defs.bzl",
        ]

        for path in excluded:
            with self.subTest(path=path):
                self.assertFalse(is_buildifier_path(path))
        for path in included:
            with self.subTest(path=path):
                self.assertTrue(is_buildifier_path(path))

    def test_selection_keeps_existing_regular_files_and_sorts_and_deduplicates_them(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "nested").mkdir()
            (root / "directory.bzl").mkdir()
            (root / "z.bzl").write_text("")
            (root / "a b.bzl").write_text("")
            (root / "nested" / "defs.bzl").write_text("")

            selected = select_buildifier_files(
                ["z.bzl", "deleted.bzl", "a b.bzl", "z.bzl", "directory.bzl", "nested\\defs.bzl"], root
            )

        self.assertEqual(selected, ["./a b.bzl", "./nested/defs.bzl", "./z.bzl"])

    @unittest.skipIf(os.name == "nt", "Creating symbolic links is not reliably available on Windows.")
    def test_selection_skips_symbolic_links(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "target.bzl").write_text("")
            (root / "link.bzl").symlink_to(root / "target.bzl")

            selected = select_buildifier_files(["link.bzl"], root)

        self.assertEqual(selected, [])


class TestBuildifierCommands(unittest.TestCase):
    def test_check_and_fix_flags_use_the_public_cli(self):
        self.assertEqual(
            buildifier_commands("buildifier", ["./BUILD.bazel"], fix=False, diff_command="diff --unified"),
            [["buildifier", "-mode=diff", "-lint=warn", "-diff_command=diff --unified", "./BUILD.bazel"]],
        )
        self.assertEqual(
            buildifier_commands("buildifier", ["./BUILD.bazel"], fix=True),
            [["buildifier", "-mode=fix", "-lint=fix", "./BUILD.bazel"]],
        )

    def test_empty_file_list_produces_no_command(self):
        self.assertEqual(buildifier_commands("buildifier", [], fix=False), [])

    def test_file_count_bounds_batches(self):
        files = [f"./pkg/file-{index}.bzl" for index in range(101)]

        commands = buildifier_commands("buildifier", files, fix=False, max_chars=100_000)

        self.assertEqual([len(command) - 3 for command in commands], [100, 1])

    def test_command_length_accepts_the_boundary_and_spills_the_next_file(self):
        first = "./pkg/first file.bzl"
        second = "./pkg/second file.bzl"
        prefix = ["buildifier", "-mode=diff", "-lint=warn"]
        boundary = max(len(join_command([*prefix, path])) for path in (first, second))

        self.assertEqual(
            buildifier_commands("buildifier", [first], fix=False, max_chars=boundary),
            [[*prefix, first]],
        )
        self.assertEqual(
            buildifier_commands("buildifier", [first, second], fix=False, max_chars=boundary),
            [[*prefix, first], [*prefix, second]],
        )

    def test_overlong_single_file_is_rejected_before_commands_are_returned(self):
        command = ["buildifier", "-mode=diff", "-lint=warn", "./pkg/file.bzl"]

        with self.assertRaisesRegex(ValueError, "exceeds"):
            buildifier_commands("buildifier", ["./pkg/file.bzl"], fix=False, max_chars=len(join_command(command)) - 1)

    @patch("tasks.libs.common.utils.is_windows", return_value=True)
    def test_windows_quoting_is_included_in_the_command_length(self, _is_windows):
        path = "./directory with spaces/file&name^.bzl"
        command = ["buildifier", "-mode=diff", "-lint=warn", path]

        with self.assertRaises(ValueError):
            buildifier_commands("buildifier", [path], fix=False, max_chars=len(join_command(command)) - 1)


class TestBuildifierTask(unittest.TestCase):
    @staticmethod
    def _result(*, stdout="", stderr="", exited=0):
        result = MagicMock()
        result.stdout = stdout
        result.stderr = stderr
        result.exited = exited
        return result

    def test_empty_selection_fails_closed(self):
        ctx = MagicMock()
        ctx.run.return_value = self._result(stdout="README.md\0")

        with (
            tempfile.TemporaryDirectory() as directory,
            patch.object(linter, "get_repo_root", return_value=Path(directory)),
            self.assertRaises(Exit) as raised,
        ):
            linter.buildifier.body(ctx)

        self.assertEqual(raised.exception.code, 2)
        ctx.run.assert_called_once()
        self.assertIn("git -C", ctx.run.call_args.args[0])

    def test_windows_clone_path_containing_percent_fails_before_the_shell(self):
        ctx = MagicMock()

        with (
            patch("tasks.libs.common.utils.is_windows", return_value=True),
            patch.object(linter, "get_repo_root", return_value=Path("C:/repo%PATH%")),
            self.assertRaises(Exit) as raised,
        ):
            linter.buildifier.body(ctx)

        self.assertEqual(raised.exception.code, 2)
        ctx.run.assert_not_called()

    def test_windows_file_path_containing_percent_fails_before_buildifier(self):
        ctx = MagicMock()
        ctx.run.return_value = self._result(stdout="pkg/file%PATH%.bzl\0")

        with (
            tempfile.TemporaryDirectory() as directory,
            patch("tasks.libs.common.utils.is_windows", return_value=True),
            patch.object(linter, "get_repo_root", return_value=Path(directory)),
            patch.object(linter, "select_buildifier_files", return_value=["./pkg/file%PATH%.bzl"]),
            self.assertRaises(Exit) as raised,
        ):
            linter.buildifier.body(ctx)

        self.assertEqual(raised.exception.code, 2)
        ctx.run.assert_called_once()

    def test_cli_contract_rejects_a_no_op_executable(self):
        ctx = MagicMock()
        ctx.run.side_effect = [
            self._result(stdout="BUILD.bazel\0"),
            self._result(exited=0),
            self._result(exited=0),
        ]

        with (
            tempfile.TemporaryDirectory() as directory,
            patch.object(linter, "get_repo_root", return_value=Path(directory)),
            patch.object(linter, "select_buildifier_files", return_value=["./BUILD.bazel"]),
            patch.object(linter, "buildifier_commands", return_value=[["buildifier", "./BUILD.bazel"]]),
            self.assertRaises(Exit) as raised,
        ):
            linter.buildifier.body(ctx)

        self.assertEqual(raised.exception.code, 1)
        self.assertEqual(ctx.run.call_count, 3)

    def test_cli_contract_surfaces_the_failing_probe_diagnostic(self):
        ctx = MagicMock()
        ctx.run.side_effect = [
            self._result(stdout="BUILD.bazel\0"),
            self._result(stderr="dotslash: executable was not found", exited=1),
            self._result(exited=4),
        ]

        with (
            tempfile.TemporaryDirectory() as directory,
            patch.object(linter, "get_repo_root", return_value=Path(directory)),
            patch.object(linter, "select_buildifier_files", return_value=["./BUILD.bazel"]),
            patch.object(linter, "buildifier_commands", return_value=[["buildifier", "./BUILD.bazel"]]),
            self.assertRaises(Exit) as raised,
        ):
            linter.buildifier.body(ctx)

        self.assertEqual(raised.exception.code, 1)
        self.assertIn("dotslash: executable was not found", str(raised.exception))

    def test_cli_contract_rejects_fix_output_that_does_not_change_the_input(self):
        ctx = MagicMock()
        ctx.run.side_effect = [
            self._result(stdout="BUILD.bazel\0"),
            self._result(exited=0),
            self._result(exited=4),
            self._result(stdout="value=[1,2]\n", exited=0),
        ]

        with (
            tempfile.TemporaryDirectory() as directory,
            patch.object(linter, "get_repo_root", return_value=Path(directory)),
            patch.object(linter, "select_buildifier_files", return_value=["./BUILD.bazel"]),
            patch.object(linter, "buildifier_commands", return_value=[["buildifier", "./BUILD.bazel"]]),
            self.assertRaises(Exit) as raised,
        ):
            linter.buildifier.body(ctx)

        self.assertEqual(raised.exception.code, 1)
        self.assertEqual(ctx.run.call_count, 4)

    def test_every_batch_runs_and_any_nonzero_exit_fails_the_task(self):
        ctx = MagicMock()
        ctx.run.side_effect = [
            self._result(stdout="BUILD.bazel\0"),
            self._result(exited=0),
            self._result(exited=4),
            self._result(stdout="value = [1, 2]\n", exited=0),
            self._result(exited=0),
            self._result(exited=4),
            self._result(exited=0),
            self._result(exited=1),
        ]
        commands = [["buildifier", f"./file-{index}.bzl"] for index in range(3)]

        with (
            tempfile.TemporaryDirectory() as directory,
            patch.object(linter, "get_repo_root", return_value=Path(directory)),
            patch.object(linter, "select_buildifier_files", return_value=["./BUILD.bazel"]),
            patch.object(linter, "buildifier_commands", return_value=commands) as command_builder,
            self.assertRaises(Exit) as raised,
        ):
            linter.buildifier.body(ctx, fix=True)

        self.assertEqual(raised.exception.code, 1)
        command_builder.assert_called_once_with(
            str(Path(directory) / "tools" / "bin" / ("buildifier.exe" if os.name == "nt" else "buildifier")),
            [".\\BUILD.bazel"] if os.name == "nt" else ["./BUILD.bazel"],
            fix=True,
            diff_command="FC" if os.name == "nt" else "diff --unified",
        )
        self.assertEqual(ctx.run.call_args_list[1].kwargs["in_stream"].getvalue(), "value = [1, 2]\n")
        self.assertEqual(ctx.run.call_args_list[2].kwargs["in_stream"].getvalue(), "value=[1,2]\n")
        self.assertEqual(ctx.run.call_args_list[3].kwargs["in_stream"].getvalue(), "value=[1,2]\n")
        self.assertEqual(ctx.run.call_args_list[4].kwargs["in_stream"].getvalue(), "value = [1, 2]\n")
        self.assertEqual(
            ctx.run.call_args_list[5:],
            [call(join_command(command), warn=True, encoding="utf-8") for command in commands],
        )


if __name__ == "__main__":
    unittest.main()
