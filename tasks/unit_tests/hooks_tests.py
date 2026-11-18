"""Exercise conservative selection and read-only validation failures."""

import importlib.util
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

REPO = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("hooks", REPO / "tasks/hooks.py")
hooks = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(hooks)


class TestHooks(unittest.TestCase):
    def setUp(self):
        git_environment = patch.dict(os.environ, {"GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_NOSYSTEM": "1"})
        git_environment.start()
        self.addCleanup(git_environment.stop)
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.patch = patch.object(hooks, "ROOT", self.root)
        self.patch.start()
        self.addCleanup(self.patch.stop)
        self.git("init", "--initial-branch=feature")
        self.git("config", "user.name", "Hook tests")
        self.git("config", "user.email", "hooks@example.com")
        self.git("config", "core.autocrlf", "false")

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root, stderr=subprocess.STDOUT)

    def write(self, path, contents):
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(contents, encoding="utf-8", newline="\n")
        return target

    def test_deleted_settings_select_all_remaining_files(self):
        self.write(".golangci.yml", "")
        self.write("a.go", "x = 1\n")
        self.write("deleted.go", "")
        self.write("untracked.go", "")
        self.git("add", ".golangci.yml", "a.go", "deleted.go")
        self.git("commit", "-qm", "Initial files")
        self.git("rm", ".golangci.yml", "deleted.go")
        changed = hooks.git_paths("diff", "--cached", "--name-only", "--diff-filter=ACDMRT")
        self.assertEqual(hooks.selected_files("go-fmt", changed), ["a.go"])

    def test_rename_spaces_and_deleted_files(self):
        self.write("old.go", "x = 1\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Initial file")
        self.git("mv", "old.go", "new name.go")
        changed = hooks.git_paths("diff", "--cached", "--name-only", "--diff-filter=ACDMRT")
        self.assertEqual(hooks.selected_files("go-fmt", changed), ["new name.go"])

    def test_full_scope_omits_missing_files_and_type_changes(self):
        self.write("present.go", "x = 1\n")
        self.write("missing.go", "x = 1\n")
        self.write("directory.go", "x = 1\n")
        self.git("add", ".")
        (self.root / "missing.go").unlink()
        (self.root / "directory.go").unlink()
        (self.root / "directory.go").mkdir()
        self.assertEqual(hooks.selected_files("go-fmt", [], all_files=True), ["present.go"])

    def test_full_scope_omits_staged_and_unstaged_symlinks(self):
        self.write("plain.go", "x = 1\n")
        self.write("changed.go", "x = 1\n")
        try:
            os.symlink("plain.go", self.root / "staged.go")
        except OSError as error:
            self.skipTest(f"Symlinks are unavailable: {error}")
        self.git("add", ".")
        (self.root / "changed.go").unlink()
        os.symlink("plain.go", self.root / "changed.go")
        self.assertEqual(hooks.selected_files("go-fmt", [], all_files=True), ["plain.go"])

    def test_full_scope_rejects_unmerged_index(self):
        self.write("conflict.go", "x = 1\n")
        oid = self.git("hash-object", "-w", "conflict.go").decode().strip()
        subprocess.run(
            ["git", "update-index", "--index-info"],
            cwd=self.root,
            input=f"100644 {oid} 1\tconflict.go\n100644 {oid} 2\tconflict.go\n".encode(),
            check=True,
        )
        with self.assertRaisesRegex(ValueError, "Resolve the index conflict in conflict.go"):
            hooks.selected_files("go-fmt", [], all_files=True)

    def test_renamed_settings_select_the_full_scope(self):
        self.write(".golangci.yml", "")
        self.write("a.go", "x = 1\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Initial files")
        self.git("mv", ".golangci.yml", "old-settings.txt")
        changed = hooks.git_paths("diff", "--cached", "--no-renames", "--name-only", "--diff-filter=ACDMRT")
        self.assertEqual(hooks.selected_files("go-fmt", changed), ["a.go"])

    def test_configuration_triggers(self):
        for check, path in [
            ("go-fmt", ".go-version"),
            ("shellcheck", "scripts/.shellcheckrc"),
            ("actionlint", ".github/actions/local/action.yml"),
        ]:
            with self.subTest(check=check, path=path):
                self.assertTrue(hooks.settings_changed(check, [path]))
                self.assertFalse(hooks.settings_changed(check, ["README.md"]))

    def test_shell_dialects_and_extensionless_scripts(self):
        self.write("preinst", "#!/bin/sh\ntrue\n")
        self.write("template.in", "#!/usr/bin/env bash\ntrue\n")
        self.write("program.py", "#!/usr/bin/env python3\n")
        self.assertEqual(
            hooks.selected_files("shellcheck", ["preinst", "template.in", "program.py"]), ["preinst", "template.in"]
        )
        with patch.object(hooks, "parallel_files", return_value=0) as run:
            hooks.check_files("shellcheck", ["preinst"])
        self.assertNotIn("--shell=bash", run.call_args.args[0])

    def test_gofmt_output_is_a_failure_without_rewriting(self):
        self.write(".golangci.yml", "formatters:\n  exclusions:\n    paths: []\n")
        target = self.write("a.go", "package p\nvar x=1\n")
        before = target.read_bytes()
        with (
            patch.object(hooks, "gofmt_command", return_value="gofmt"),
            patch.object(
                hooks.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, b"a.go\n", b"")
            ) as run,
        ):
            self.assertEqual(hooks.check_files("go-fmt", ["a.go"]), 1)
        self.assertNotIn("-w", run.call_args.args[0])
        self.assertEqual(target.read_bytes(), before)

    def test_prohibited_tracing_failure_preserves_files(self):
        target = self.write("a.sh", "#!/bin/bash\nset -eux\n")
        before = target.read_bytes()
        self.assertEqual(hooks.check_files("shell-check-no-set-x", ["a.sh"]), 1)
        self.assertEqual(target.read_bytes(), before)

    def test_final_newline(self):
        self.write("release.json", "{}")
        self.assertEqual(hooks.check_files("release-json-newline", ["release.json"]), 1)
        self.write("release.json", "{}\n")
        self.assertEqual(hooks.check_files("release-json-newline", ["release.json"]), 0)

    @unittest.skipUnless(os.name != "nt" and shutil.which("bash"), "CI uses a native POSIX shell")
    def test_ci_comparison_base_handles_mainline_and_diverged_feature_pushes(self):
        import yaml

        config = yaml.load(
            (REPO / ".gitlab/build/lint/technical_linters.yml").read_text("utf-8"), Loader=yaml.BaseLoader
        )
        root_config = yaml.load((REPO / ".gitlab-ci.yml").read_text("utf-8"), Loader=yaml.BaseLoader)
        self.assertTrue(
            any(
                ".gitlab/build/lint/technical_linters.yml" in rule.get("changes", {}).get("paths", [])
                for rule in root_config[".on_invoke_tasks_changes"]
                if isinstance(rule, dict)
            )
        )
        script = config[".lint_changed_files"]["before_script"][1]
        self.write("target.go", "package original\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Initial mainline")
        base = self.git("rev-parse", "HEAD").decode().strip()
        self.git("branch", "main")
        self.write("target.go", "package changed\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Feature source change")
        previous = self.git("rev-parse", "HEAD").decode().strip()
        self.write("README.md", "Feature documentation\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Feature documentation change")
        self.git("checkout", "main")
        self.write("main.go", "package mainline\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Independent mainline change")
        target = self.git("rev-parse", "HEAD").decode().strip()
        self.git("checkout", "feature")
        self.git("fetch", ".", "main")
        self.assertEqual(len({base, previous, target}), 3)
        cases = [
            ("feature", previous, base),
            ("main", previous, previous),
            ("main", "0" * 40, "HEAD^"),
            ("main", "", "HEAD^"),
            ("", "", base),
            ("feature", "", base),
            ("feature", "0" * 40, base),
        ]

        def comparison(branch, before):
            return subprocess.check_output(
                ["bash", "-e", "-c", script + '\nprintf "%s" "$CHANGED_FILES_BASE"'],
                cwd=self.root,
                env={
                    **os.environ,
                    "CI_COMMIT_BRANCH": branch,
                    "CI_COMMIT_BEFORE_SHA": before,
                    "COMPARE_TO_BRANCH": "main",
                },
                text=True,
            )

        for branch, before, expected in cases:
            with self.subTest(branch=branch, before=before):
                self.assertEqual(comparison(branch, before), expected)
        unrelated = self.git("commit-tree", "HEAD^{tree}", "-m", "Unrelated target history").decode().strip()
        self.git("fetch", ".", unrelated)
        for before, expected in [(previous, previous), ("0" * 40, "HEAD^"), ("", "HEAD^")]:
            with self.subTest(history="unrelated", before=before):
                self.assertEqual(comparison("main", before), expected)

    def test_explicit_ci_base_compares_push_endpoints_after_a_rewind(self):
        self.write("target.go", "package original\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Initial mainline")
        base = self.git("rev-parse", "HEAD").decode().strip()
        self.write("target.go", "package changed\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Previous push tip")
        previous = self.git("rev-parse", "HEAD").decode().strip()
        self.git("checkout", "--detach", base)
        previous_directory = Path.cwd()
        self.addCleanup(os.chdir, previous_directory)
        with (
            patch.object(sys, "argv", ["hooks.py", "go-fmt", "--base", previous]),
            patch.object(hooks, "check_files", return_value=0) as check,
        ):
            self.assertEqual(hooks.main(), 0)
            check.assert_called_once_with("go-fmt", ["target.go"])


if __name__ == "__main__":
    unittest.main()
