import os
import runpy
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

# Loading the file directly avoids initializing the Invoke task collection.
policy = runpy.run_path(str(Path(__file__).resolve().parents[1] / "libs/linter/buildifier.py"))
normalize_buildifier_path = policy["_normalize_buildifier_path"]
select_buildifier_files = policy["select_buildifier_files"]
buildifier_commands = policy["buildifier_commands"]
command_length = policy["_command_length"]


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
                self.assertEqual(normalize_buildifier_path(path), f"./{path}")

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
                self.assertIsNone(normalize_buildifier_path(path))

    def test_malformed_and_nonrelative_paths_are_rejected(self):
        paths = [
            "",
            ".",
            "./",
            "/BUILD",
            "//server/share/BUILD",
            "C:/BUILD",
            "C:BUILD",
            "../BUILD",
            "pkg/../BUILD",
            "pkg/./BUILD",
            "pkg//BUILD",
            "pkg/BUILD/",
            "././../BUILD",
            "\\\\server\\share\\BUILD",
        ]
        for path in paths:
            with self.subTest(path=path):
                self.assertIsNone(normalize_buildifier_path(path))

    def test_selection_normalizes_repeated_prefixes_and_backslashes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "nested").mkdir()
            (root / "nested" / "defs.bzl").write_text("")
            selected = select_buildifier_files(
                ["././nested/defs.bzl", ".\\.\\nested\\defs.bzl", "nested/defs.bzl"], root
            )
        self.assertEqual(selected, ["./nested/defs.bzl"])

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
                self.assertIsNone(normalize_buildifier_path(path))
        for path in included:
            with self.subTest(path=path):
                self.assertEqual(normalize_buildifier_path(path), f"./{path}")

    def test_selection_keeps_existing_regular_files_and_sorts_and_deduplicates_them(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "nested").mkdir()
            (root / "directory.bzl").mkdir()
            (root / "submodule.bzl").mkdir()
            (root / "submodule.bzl" / ".git").write_text("gitdir: ../module")
            (root / "z.bzl").write_text("")
            (root / "a b.bzl").write_text("")
            (root / "nested" / "defs.bzl").write_text("")

            selected = select_buildifier_files(
                ["z.bzl", "deleted.bzl", "a b.bzl", "z.bzl", "directory.bzl", "submodule.bzl", "nested\\defs.bzl"], root
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
    def test_native_argument_lengths_include_encoding_and_terminators(self):
        with patch.object(policy["sys"], "platform", "win32"):
            self.assertEqual(command_length(["tool.exe", "a b", 'x"y', "\U0001f600"]), 23)
        with patch.object(policy["sys"], "platform", "linux"):
            self.assertEqual(command_length(["tool", "a b", "\U0001f600"]), 14)

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
        self.assertEqual(len(buildifier_commands("buildifier", files[:100], fix=False, max_chars=100_000)), 1)

    def test_nonpositive_limits_are_rejected(self):
        for limit in ("max_files", "max_chars"):
            for value in (0, -1):
                with self.subTest(limit=limit, value=value), self.assertRaisesRegex(ValueError, "positive"):
                    buildifier_commands("buildifier", [], fix=False, **{limit: value})

    def test_command_length_accepts_the_boundary_and_spills_the_next_file(self):
        first = "./pkg/first file.bzl"
        second = "./pkg/second file.bzl"
        prefix = ["buildifier", "-mode=diff", "-lint=warn"]
        boundary = max(command_length([*prefix, path]) for path in (first, second))

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
            buildifier_commands("buildifier", ["./pkg/file.bzl"], fix=False, max_chars=command_length(command) - 1)

    def test_overlong_file_after_a_full_batch_prevents_returning_commands(self):
        short = "./a.bzl"
        long = "./a-much-longer-name.bzl"
        boundary = command_length(["buildifier", "-mode=fix", "-lint=fix", short])
        with self.assertRaisesRegex(ValueError, "exceeds"):
            buildifier_commands("buildifier", [short, long], fix=True, max_files=1, max_chars=boundary)


if __name__ == "__main__":
    unittest.main()
