"""Exercise selection, read-only failures, and pre-commit's index isolation."""

import importlib.util
import io
import os
import py_compile
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

    def test_hook_cli_accepts_interspersed_options_and_dash_prefixed_paths(self):
        self.write("selected.py", "value = 1\n")
        self.write("-selected.py", "value = 1\n")
        self.write("--all.py", "value = 1\n")
        previous_directory = Path.cwd()
        self.addCleanup(os.chdir, previous_directory)
        for arguments, expected in [
            (["python-ruff", "--hook", "--", "selected.py"], "selected.py"),
            (["python-ruff", "--hook", "--", "-selected.py"], "-selected.py"),
            (["python-ruff", "--hook", "--", "--all.py"], "--all.py"),
            (["python-ruff", "selected.py", "--hook"], "selected.py"),
        ]:
            with (
                self.subTest(arguments=arguments),
                patch.object(sys, "argv", ["hooks.py", *arguments]),
                patch.dict(os.environ, {"PRE_COMMIT_FROM_REF": "", "PRE_COMMIT_TO_REF": ""}),
                patch.object(hooks, "check_files", return_value=0) as check,
            ):
                self.assertEqual(hooks.main(), 0)
                check.assert_called_once_with("python-ruff", [expected])

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
            ("copyright", "tasks/libs/types/copyright.py"),
            ("shellcheck", "scripts/.shellcheckrc"),
            ("actionlint", ".github/actions/local/action.yml"),
            ("python-ruff", "nested/pyproject.toml"),
            ("buildifier", "bazel/prebuilt_buildtools.json"),
            ("rustfmt", "nested/Cargo.toml"),
            ("rustfmt", "nested/.rustfmt.toml"),
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
        with patch.object(hooks.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, b"", b"")) as run:
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

    def test_protected_and_detached_branches(self):
        self.write("file", "content\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Initial file")
        self.assertEqual(hooks.protected_branch(), 0)
        for branch in ("main", "7.81.x"):
            self.git("checkout", "-qb", branch)
            self.assertEqual(hooks.protected_branch(), 1)
        self.git("checkout", "--detach")
        self.assertEqual(hooks.protected_branch(), 0)

    def test_copyright_bsd_exception_accepts_string_paths(self):
        spec = importlib.util.spec_from_file_location("copyright_linter", REPO / "tasks/libs/types/copyright.py")
        copyright = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(copyright)

        target = self.write(
            "cmd/secret-generic-connector/main.go",
            "// Unless explicitly stated otherwise all files in this repository are licensed\n"
            "// under the BSD 3-Clause License.\n"
            "// This product includes software developed at Datadog (https://www.datadoghq.com/).\n"
            "// Copyright 2026-present Datadog, Inc.\n",
        )
        linter = copyright.CopyrightLinter()
        self.assertTrue(linter._has_copyright(str(target)))
        invalid = self.write("invalid.go", "package example\n")
        excluded = self.write("vendor/example.go", "package example\n")
        self.assertEqual(linter._assert_copyrights([invalid, excluded]), [invalid])

    def test_copyright_rules_use_current_source(self):
        self.write("tasks/hooks.py", (REPO / "tasks/hooks.py").read_text("utf-8"))
        self.write("example.go", "package example\n")
        rules = self.write(
            "tasks/libs/types/copyright.py",
            "class LintFailure(Exception): pass\n"
            "class CopyrightLinter:\n"
            "    def assert_compliance(self, files):\n"
            "        if 1: raise LintFailure()\n",
        )
        # A same-size replacement in the same second leaves timestamp-based
        # bytecode valid, as can happen when pre-commit hides unstaged rules.
        timestamp = 1_700_000_000
        os.utime(rules, (timestamp, timestamp))
        py_compile.compile(str(rules), doraise=True)
        rules.write_text(rules.read_text().replace("if 1:", "if 0:"), encoding="utf-8", newline="\n")
        os.utime(rules, (timestamp, timestamp))
        result = subprocess.run(
            [sys.executable, "tasks/hooks.py", "copyright", "example.go"], cwd=self.root, capture_output=True
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    @unittest.skipUnless(shutil.which("pre-commit"), "pre-commit is required")
    def test_partially_staged_success_and_failure_preserve_index_and_worktree(self):
        self.write("tasks/hooks.py", (REPO / "tasks/hooks.py").read_text("utf-8"))
        self.write(
            ".pre-commit-config.yaml",
            "repos:\n- repo: local\n  hooks:\n  - id: tracing\n    name: tracing\n"
            f'    entry: \'"{Path(sys.executable).as_posix()}" tasks/hooks.py shell-check-no-set-x --hook --\'\n'
            "    language: system\n    always_run: true\n    require_serial: true\n",
        )
        target = self.write("space name.sh", "#!/bin/bash\ntrue\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Initial files")
        for staged, unstaged, expected in [("false", "set -x", 0), ("set -x", "true", 1)]:
            with self.subTest(staged=staged):
                target.write_text(f"#!/bin/bash\n{staged}\n", encoding="utf-8", newline="\n")
                self.git("add", "space name.sh")
                target.write_text(f"#!/bin/bash\n{unstaged}\n", encoding="utf-8", newline="\n")
                index = self.git("write-tree")
                before = target.read_bytes()
                result = subprocess.run(["pre-commit", "run"], cwd=self.root, capture_output=True)
                self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
                self.assertEqual(self.git("write-tree"), index)
                self.assertEqual(target.read_bytes(), before)
        self.git("reset", "--hard", "HEAD")
        target.write_text("#!/bin/bash\nset -x\n", encoding="utf-8", newline="\n")
        for scope in (["--all-files"], ["--files", "space name.sh"]):
            with self.subTest(scope=scope):
                result = subprocess.run(["pre-commit", "run", *scope], cwd=self.root, capture_output=True)
                self.assertEqual(result.returncode, 1, result.stdout + result.stderr)

    @unittest.skipUnless(shutil.which("pre-commit"), "pre-commit is required")
    def test_individual_hook_selection_skipping_and_failure_reporting(self):
        import yaml

        self.write("tasks/hooks.py", (REPO / "tasks/hooks.py").read_text("utf-8"))
        config = yaml.safe_load((REPO / ".pre-commit-config.yaml").read_text("utf-8"))
        configured = config["repos"][0]["hooks"]
        expected = [
            "protected-branches",
            "go-fmt",
            "shellcheck",
            "actionlint",
            "copyright",
            "shell-check-no-set-x",
            "python-ruff",
            "buildifier",
            "rustfmt",
        ]
        self.assertEqual([hook["id"] for hook in configured], expected)
        self.assertEqual(config["default_install_hook_types"], ["pre-commit"])
        self.assertEqual(config["default_stages"], ["pre-commit"])
        dependencies = configured[0]["additional_dependencies"]
        for hook in configured:
            self.assertEqual(hook["language"], "python")
            self.assertEqual(hook["additional_dependencies"], dependencies)
            hook["language"] = "system"
            hook.pop("additional_dependencies")
            hook["entry"] = hook["entry"].replace("python ", f'"{Path(sys.executable).as_posix()}" ', 1)
        self.write(".pre-commit-config.yaml", yaml.safe_dump(config))
        target = self.write("tracing.yaml", "script: set -x\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Initial files")
        environment = {key: value for key, value in os.environ.items() if key != "SKIP"}

        def run(*args, skip=None):
            return subprocess.run(
                ["pre-commit", "run", *args, "--files", "tracing.yaml"],
                cwd=self.root,
                env={**environment, **({"SKIP": skip} if skip else {})},
                capture_output=True,
                text=True,
            )

        index, contents = self.git("write-tree"), target.read_bytes()
        failure = run("shell-check-no-set-x")
        self.assertEqual(failure.returncode, 1, failure.stdout + failure.stderr)
        self.assertNotIn("protected-branches", failure.stdout)
        selected = run("python-ruff")
        self.assertEqual(selected.returncode, 0, selected.stdout + selected.stderr)
        self.assertNotIn("shell-check-no-set-x", selected.stdout)
        skipped = run(skip="shell-check-no-set-x")
        self.assertEqual(skipped.returncode, 0, skipped.stdout + skipped.stderr)
        self.assertIn("Skipped", skipped.stdout)
        all_checks = run()
        self.assertEqual(all_checks.returncode, 1, all_checks.stdout + all_checks.stderr)
        for check in expected:
            self.assertIn(check, all_checks.stdout)
        self.assertGreater(all_checks.stdout.index(expected[-1]), all_checks.stdout.index("shell-check-no-set-x"))
        self.assertEqual(self.git("write-tree"), index)
        self.assertEqual(target.read_bytes(), contents)

    def test_hook_scope_includes_deleted_configuration_for_index_and_refs(self):
        self.write(".golangci.yml", "")
        self.write("selected.go", "package selected\n")
        self.write("other.go", "package other\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Initial files")
        original = self.git("rev-parse", "HEAD").decode().strip()
        self.git("rm", ".golangci.yml")
        previous_directory = Path.cwd()
        self.addCleanup(os.chdir, previous_directory)
        with (
            patch.object(sys, "argv", ["hooks.py", "go-fmt", "--hook", "--", "selected.go"]),
            patch.dict(os.environ, {"PRE_COMMIT_FROM_REF": "", "PRE_COMMIT_TO_REF": ""}),
            patch.object(hooks, "check_files", return_value=0) as check,
        ):
            self.assertEqual(hooks.main(), 0)
            check.assert_called_once_with("go-fmt", ["other.go", "selected.go"])
        self.git("commit", "-qm", "Delete formatter settings")
        current = self.git("rev-parse", "HEAD").decode().strip()
        with (
            patch.object(sys, "argv", ["hooks.py", "go-fmt", "--hook", "--", "selected.go"]),
            patch.dict(os.environ, {"PRE_COMMIT_FROM_REF": original, "PRE_COMMIT_TO_REF": current}),
            patch.object(hooks, "check_files", return_value=0) as check,
        ):
            self.assertEqual(hooks.main(), 0)
            check.assert_called_once_with("go-fmt", ["other.go", "selected.go"])

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

    def test_deleted_untracked_sources_are_not_validated(self):
        target = self.write("removed.sh", "#!/bin/sh\nset -x\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Initial source")
        self.git("rm", "--cached", "removed.sh")
        previous_directory = Path.cwd()
        self.addCleanup(os.chdir, previous_directory)
        for scope in ["--hook", "--staged"]:
            with (
                self.subTest(scope=scope),
                patch.object(sys, "argv", ["hooks.py", "shell-check-no-set-x", scope]),
                patch.dict(os.environ, {"PRE_COMMIT_FROM_REF": "", "PRE_COMMIT_TO_REF": ""}),
                patch.object(hooks, "check_files", return_value=0) as check,
            ):
                self.assertEqual(hooks.main(), 0)
                check.assert_called_once_with("shell-check-no-set-x", [])
        self.assertTrue(target.is_file())
        from_ref = self.git("rev-parse", "HEAD").decode().strip()
        self.git("commit", "-qm", "Remove source from the index")
        to_ref = self.git("rev-parse", "HEAD").decode().strip()
        with (
            patch.object(sys, "argv", ["hooks.py", "shell-check-no-set-x", "--hook", "--", "removed.sh"]),
            patch.dict(os.environ, {"PRE_COMMIT_FROM_REF": from_ref, "PRE_COMMIT_TO_REF": to_ref}),
            patch.object(hooks, "check_files", return_value=0) as check,
        ):
            self.assertEqual(hooks.main(), 0)
            check.assert_called_once_with("shell-check-no-set-x", [])

    def test_ref_deletion_recovery_uses_the_requested_merge_base(self):
        self.write(".golangci.yml", "")
        self.write("selected.go", "package selected\n")
        self.write("other.go", "package other\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Common ancestor")
        self.git("branch", "comparison")
        self.git("rm", ".golangci.yml")
        self.git("commit", "-qm", "Delete feature settings")
        to_ref = self.git("rev-parse", "HEAD").decode().strip()
        self.git("checkout", "comparison")
        self.git("rm", ".golangci.yml")
        self.git("commit", "-qm", "Delete comparison settings independently")
        from_ref = self.git("rev-parse", "HEAD").decode().strip()
        self.git("checkout", "feature")
        previous_directory = Path.cwd()
        self.addCleanup(os.chdir, previous_directory)
        with (
            patch.object(sys, "argv", ["hooks.py", "go-fmt", "--hook", "--", "selected.go"]),
            patch.dict(os.environ, {"PRE_COMMIT_FROM_REF": from_ref, "PRE_COMMIT_TO_REF": to_ref}),
            patch.object(hooks, "check_files", return_value=0) as check,
        ):
            self.assertEqual(hooks.main(), 0)
            check.assert_called_once_with("go-fmt", ["other.go", "selected.go"])

    @unittest.skipUnless(shutil.which("gofmt"), "gofmt is required")
    def test_gofmt_checks_lf_index_contents_without_rewriting_crlf_worktree(self):
        self.write(".gitattributes", "* text eol=lf\n")
        self.write(".golangci.yml", "formatters:\n  exclusions:\n    paths: []\n")
        target = self.write("a.go", "package example\n\nvar value = 1\n")
        self.git("add", ".")
        target.write_bytes(target.read_bytes().replace(b"\n", b"\r\n"))
        self.assertEqual(subprocess.run(["git", "diff", "--quiet"], cwd=self.root, capture_output=True).returncode, 0)
        index, contents = self.git("write-tree"), target.read_bytes()
        with patch.object(hooks, "gofmt_command", return_value=shutil.which("gofmt")):
            self.assertEqual(hooks.check_files("go-fmt", ["a.go"]), 0)
            self.assertEqual(target.read_bytes(), contents)
            target.write_bytes(b"package example\r\nvar value=1\r\n")
            invalid = target.read_bytes()
            self.assertEqual(hooks.check_files("go-fmt", ["a.go"]), 1)
            self.assertEqual(target.read_bytes(), invalid)
            target.write_bytes(b"var value = 1\r\n")
            fragment = target.read_bytes()
            self.assertEqual(hooks.check_files("go-fmt", ["a.go"]), 1)
            self.assertEqual(target.read_bytes(), fragment)
        self.assertEqual(self.git("write-tree"), index)

    @unittest.skipUnless(shutil.which("shellcheck"), "ShellCheck is required")
    def test_shellcheck_normalizes_crlf_and_retains_directory_configuration(self):
        self.write(".gitattributes", "* text eol=lf\n")
        self.write("nested/.shellcheckrc", "disable=SC2034\n")
        target = self.write("nested/a.sh", "#!/bin/sh\nunused=1\nprintf '%s\\n' ready\n")
        target.write_bytes(target.read_bytes().replace(b"\n", b"\r\n"))
        contents = target.read_bytes()
        self.assertEqual(hooks.check_files("shellcheck", ["nested/a.sh"]), 0)
        self.assertEqual(target.read_bytes(), contents)
        target.write_bytes(b"#!/bin/sh\r\nprintf '%s\\n' \"$missing\"\r\n")
        invalid = target.read_bytes()
        self.assertEqual(hooks.check_files("shellcheck", ["nested/a.sh"]), 1)
        self.assertEqual(target.read_bytes(), invalid)
        self.assertEqual(list((self.root / "nested").glob(".agent-hooks-*")), [])

    @unittest.skipUnless(shutil.which("shellcheck"), "ShellCheck is required")
    def test_shellcheck_retains_selected_source_inputs_with_crlf(self):
        self.write(".gitattributes", "* text eol=lf\n")
        self.write(".shellcheckrc", "disable=SC2034\n")
        main = self.write("main.sh", '#!/bin/sh\n. ./helper.sh\nprintf "%s\\n" "$value"\n')
        helper = self.write("helper.sh", "#!/bin/sh\nvalue=ready\n")
        main_lf, helper_lf = main.read_bytes(), helper.read_bytes()
        for main_crlf, helper_crlf in [(True, False), (False, True), (True, True)]:
            with self.subTest(main_crlf=main_crlf, helper_crlf=helper_crlf):
                main.write_bytes(main_lf.replace(b"\n", b"\r\n") if main_crlf else main_lf)
                helper.write_bytes(helper_lf.replace(b"\n", b"\r\n") if helper_crlf else helper_lf)
                contents = (main.read_bytes(), helper.read_bytes())
                self.assertEqual(hooks.check_files("shellcheck", ["main.sh", "helper.sh"]), 0)
                self.assertEqual((main.read_bytes(), helper.read_bytes()), contents)

    @unittest.skipUnless(shutil.which("shellcheck"), "ShellCheck is required")
    def test_binary_attributes_and_indexed_crlf_keep_shellcheck_findings(self):
        attributes = self.write(".gitattributes", "*.sh -text\n")
        target = self.write("a.sh", "#!/bin/sh\nprintf '%s\\n' ready\n")
        target.write_bytes(target.read_bytes().replace(b"\n", b"\r\n"))
        self.git("add", ".")
        contents = target.read_bytes()
        self.assertEqual(hooks.canonical_bytes("a.sh"), contents)
        self.assertEqual(hooks.check_files("shellcheck", ["a.sh"]), 1)
        attributes.write_text("* text eol=lf\n", encoding="utf-8")
        self.assertEqual(hooks.canonical_bytes("a.sh"), contents)
        self.assertEqual(target.read_bytes(), contents)

    def test_ref_deletion_recovery_matches_pre_commit_without_a_merge_base(self):
        self.write(".golangci.yml", "")
        self.write("selected.go", "package selected\n")
        self.write("other.go", "package other\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Initial history")
        from_ref = self.git("rev-parse", "HEAD").decode().strip()
        self.git("rm", "--cached", ".golangci.yml")
        tree = self.git("write-tree").decode().strip()
        to_ref = self.git("commit-tree", tree, "-m", "Separate history").decode().strip()
        previous_directory = Path.cwd()
        self.addCleanup(os.chdir, previous_directory)
        with (
            patch.object(sys, "argv", ["hooks.py", "go-fmt", "--hook", "--", "selected.go"]),
            patch.dict(os.environ, {"PRE_COMMIT_FROM_REF": from_ref, "PRE_COMMIT_TO_REF": to_ref}),
            patch.object(hooks, "check_files", return_value=0) as check,
        ):
            self.assertEqual(hooks.main(), 0)
            check.assert_called_once_with("go-fmt", ["other.go", "selected.go"])

    @unittest.skipUnless(shutil.which("shellcheck"), "ShellCheck is required")
    def test_shellcheck_analyzes_normalized_continuations(self):
        self.write(".gitattributes", "* text eol=lf\n")
        target = self.write("a.sh", '#!/bin/sh\nvalue="$1"\necho \\\n  $value\n')
        target.write_bytes(target.read_bytes().replace(b"\n", b"\r\n"))
        contents = target.read_bytes()
        temporary_directory = tempfile.TemporaryDirectory
        views = []

        def create_view(*args, **kwargs):
            directory = temporary_directory(*args, **kwargs)
            views.append(Path(directory.name))
            return directory

        with patch.object(tempfile, "TemporaryDirectory", side_effect=create_view):
            self.assertEqual(hooks.check_files("shellcheck", ["a.sh"]), 1)
        self.assertEqual(target.read_bytes(), contents)
        self.assertTrue(views)
        self.assertTrue(all(not view.exists() for view in views))

    @unittest.skipUnless(shutil.which("shellcheck"), "ShellCheck is required")
    def test_shellcheck_retains_context_when_checkout_parent_is_unwritable(self):
        import stat

        parent = self.root / "shared/parent"
        checkout = parent / "repo"
        checkout.mkdir(parents=True)
        helper = self.write("shared/helper.sh", "value=ready\n")
        self.write("shared/parent/.shellcheckrc", "disable=SC2034\n")
        script = self.write("shared/parent/repo/a.sh", '#!/bin/sh\nunused=1\n. helper.sh\nprintf "%s\\n" "$value"\n')
        self.write("shared/parent/repo/.gitattributes", "* text eol=lf\n")
        unrelated = self.write("shared/parent/repo/unrelated.txt", "Preserve read-only attributes.\n")
        subprocess.run(["git", "init", "--template="], cwd=checkout, capture_output=True, check=True)
        subprocess.run(["git", "add", "."], cwd=checkout, capture_output=True, check=True)
        script.write_bytes(script.read_bytes().replace(b"\n", b"\r\n"))
        contents = script.read_bytes()
        helper.chmod(stat.S_IREAD)
        unrelated.chmod(stat.S_IREAD)
        modes = [path.stat().st_mode for path in (helper, unrelated)]
        temporary_directory = tempfile.TemporaryDirectory
        views = []

        def create_view(*args, **kwargs):
            if Path(kwargs.get("dir", ".")) == parent:
                raise PermissionError("The checkout parent is read-only.")
            directory = temporary_directory(*args, **kwargs)
            views.append(Path(directory.name))
            return directory

        iterdir = Path.iterdir

        def list_directory(path):
            if path == parent:
                raise PermissionError("The parent permits traversal, but not listing.")
            return iterdir(path)

        try:
            if os.name != "nt":
                parent.chmod(0o111)
            with (
                patch.object(hooks, "ROOT", checkout),
                patch.dict(os.environ, {"SHELLCHECK_OPTS": "-x -P ../.."}),
                patch.object(tempfile, "TemporaryDirectory", side_effect=create_view),
                patch.object(Path, "iterdir", list_directory),
            ):
                self.assertEqual(hooks.check_files("shellcheck", ["a.sh"]), 0)
                script.write_bytes(b"#!/bin/sh\r\necho $1\r\n")
                self.assertEqual(hooks.check_files("shellcheck", ["a.sh"]), 1)
            self.assertTrue(views)
            self.assertTrue(all(view.is_relative_to(checkout) and not view.exists() for view in views))
            self.assertEqual([path.stat().st_mode for path in (helper, unrelated)], modes)
            self.assertEqual(helper.read_bytes(), b"value=ready\n")
            self.assertEqual(unrelated.read_text("utf-8"), "Preserve read-only attributes.\n")
            self.assertEqual(script.read_bytes(), b"#!/bin/sh\r\necho $1\r\n")
            self.assertIn(b"\r\n", contents)
        finally:
            parent.chmod(0o755)
            helper.chmod(stat.S_IREAD | stat.S_IWRITE)
            unrelated.chmod(stat.S_IREAD | stat.S_IWRITE)

    @unittest.skipUnless(shutil.which("shellcheck"), "ShellCheck is required")
    def test_shellcheck_copies_readable_context_when_hard_links_are_unavailable(self):
        import errno

        self.write(".gitattributes", "* text eol=lf\n")
        self.write(".shellcheckrc", "disable=SC2034\n")
        script = self.write("a.sh", '#!/bin/sh\n. ./helper.sh\nprintf "%s\\n" "$value"\n')
        helper = self.write("helper.sh", "#!/bin/sh\nvalue=ready\n")
        self.git("add", ".")
        script.write_bytes(script.read_bytes().replace(b"\n", b"\r\n"))
        contents = (script.read_bytes(), helper.read_bytes())
        for error in (PermissionError("Protected hard links"), OSError(errno.EXDEV, "Different filesystems")):
            with self.subTest(error=error), patch.object(os, "link", side_effect=error):
                self.assertEqual(hooks.check_files("shellcheck", ["a.sh", "helper.sh"]), 0)
        self.assertEqual((script.read_bytes(), helper.read_bytes()), contents)

    def test_shellcheck_materializes_symlinked_configuration_contents(self):
        source = self.write("original/rules.rc", "disable=SC2034\n")
        link = source.parent / ".shellcheckrc"
        try:
            link.symlink_to("rules.rc")
        except OSError as error:
            self.skipTest(f"Symlinks are unavailable: {error}")
        destination = self.root / "view/.shellcheckrc"
        destination.parent.mkdir()
        hooks.shellcheck_link(link, destination)
        self.assertEqual(destination.read_bytes(), b"disable=SC2034\n")
        self.assertFalse(destination.is_symlink())
        self.assertEqual(source.read_bytes(), b"disable=SC2034\n")

    def test_shellcheck_never_copies_a_character_device(self):
        import errno

        source = Path("NUL") if os.name == "nt" else Path("/dev/zero")
        with (
            patch.object(os, "link", side_effect=OSError(errno.EXDEV, "Different filesystems")),
            patch.object(shutil, "copyfile", side_effect=AssertionError("A device must never be copied")),
            self.assertRaisesRegex(ValueError, "regular file"),
        ):
            hooks.shellcheck_link(source, self.root / "device")

    @unittest.skipUnless(shutil.which("shellcheck"), "ShellCheck is required")
    def test_shellcheck_normalizes_forced_text_despite_binary_classification(self):
        self.write(".gitattributes", "* text eol=lf\n")
        script = self.root / "a.sh"
        script.write_bytes(b"#!/bin/sh\n# " + b"\x01" * 20 + b"\nprintf '%s\\n' ready\n")
        self.git("add", ".")
        self.assertEqual(hooks.check_files("shellcheck", ["a.sh"]), 0)
        script.write_bytes(script.read_bytes().replace(b"\n", b"\r\n"))
        contents = script.read_bytes()
        self.assertIn(b"w/-text", self.git("ls-files", "--eol"))
        self.assertEqual(hooks.check_files("shellcheck", ["a.sh"]), 0)
        self.assertEqual(script.read_bytes(), contents)

    @unittest.skipUnless(os.name == "nt" and shutil.which("shellcheck"), "Windows ShellCheck is required")
    def test_shellcheck_ignores_an_unrelated_exclusively_locked_file(self):
        import ctypes
        from ctypes import wintypes

        self.write(".gitattributes", "* text eol=lf\n")
        script = self.write("a.sh", "#!/bin/sh\nprintf '%s\\n' ready\n")
        unrelated = self.write("unrelated.txt", "An editor holds this file.\n")
        self.git("add", ".")
        script.write_bytes(script.read_bytes().replace(b"\n", b"\r\n"))
        contents = script.read_bytes()
        kernel = ctypes.WinDLL("kernel32", use_last_error=True)
        kernel.CreateFileW.argtypes = [
            wintypes.LPCWSTR,
            wintypes.DWORD,
            wintypes.DWORD,
            ctypes.c_void_p,
            wintypes.DWORD,
            wintypes.DWORD,
            wintypes.HANDLE,
        ]
        kernel.CreateFileW.restype = wintypes.HANDLE
        kernel.CloseHandle.argtypes = [wintypes.HANDLE]
        handle = kernel.CreateFileW(str(unrelated), 0x80000000, 0, None, 3, 0x80, None)
        if handle == wintypes.HANDLE(-1).value:
            raise ctypes.WinError(ctypes.get_last_error())
        try:
            self.assertEqual(hooks.check_files("shellcheck", ["a.sh"]), 0)
        finally:
            kernel.CloseHandle(handle)
        self.assertEqual(script.read_bytes(), contents)
        self.assertEqual(unrelated.read_bytes(), b"An editor holds this file.\n")

    def test_silent_gofmt_failure_is_still_a_failure(self):
        self.write(".gitattributes", "* text eol=lf\n")
        target = self.write("a.go", "package example\n")
        target.write_bytes(target.read_bytes().replace(b"\n", b"\r\n"))
        original_run = subprocess.run
        temporary_inputs = []

        def run(command, **kwargs):
            if command[0] == "fake-gofmt":
                temporary_inputs.append(Path(command[-1]))
                return subprocess.CompletedProcess(command, 2, b"", b"")
            return original_run(command, **kwargs)

        with patch.object(hooks.subprocess, "run", side_effect=run):
            self.assertEqual(hooks.gofmt_normalized_files("fake-gofmt", ["a.go"]), 1)
        self.assertTrue(temporary_inputs)
        self.assertTrue(all(not path.exists() for path in temporary_inputs))

    def test_case_only_source_rename_is_not_filtered_as_deleted(self):
        self.write("Run.sh", "#!/bin/sh\ntrue\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Initial source")
        self.git("mv", "Run.sh", "temporary.sh")
        self.git("mv", "temporary.sh", "run.sh")
        self.write("run.sh", "#!/bin/sh\nset -x\n")
        self.git("add", "run.sh")
        previous_directory = Path.cwd()
        self.addCleanup(os.chdir, previous_directory)
        with (
            patch.object(sys, "argv", ["hooks.py", "shell-check-no-set-x", "--hook", "--", "run.sh"]),
            patch.dict(os.environ, {"PRE_COMMIT_FROM_REF": "", "PRE_COMMIT_TO_REF": ""}),
            patch.object(hooks, "check_files", return_value=0) as check,
        ):
            self.assertEqual(hooks.main(), 0)
            check.assert_called_once_with("shell-check-no-set-x", ["run.sh"])

    @unittest.skipUnless(shutil.which("shellcheck"), "ShellCheck is required")
    def test_shellcheck_preserves_source_paths_outside_the_repository(self):
        shared = tempfile.TemporaryDirectory(dir=self.root.parent)
        self.addCleanup(shared.cleanup)
        directory = Path(shared.name)
        (directory / "helper.sh").write_bytes(b"value=ready\n")
        self.write(".gitattributes", "* text eol=lf\n")
        target = self.write("a.sh", '#!/bin/sh\n. helper.sh\nprintf "%s\\n" "$value"\n')
        target.write_bytes(target.read_bytes().replace(b"\n", b"\r\n"))
        contents = target.read_bytes()
        with patch.dict(os.environ, {"SHELLCHECK_OPTS": f"-x -P ../{directory.name}"}):
            self.assertEqual(hooks.check_files("shellcheck", ["a.sh"]), 0)
        self.assertEqual(target.read_bytes(), contents)

    @unittest.skipUnless(shutil.which("ruff"), "Ruff is required")
    def test_ruff_validates_git_line_endings_without_writes_and_retains_exclusions(self):
        self.write(".gitattributes", "* text eol=lf\n")
        self.write("pyproject.toml", '[tool.ruff]\nexclude = ["excluded"]\n[tool.ruff.format]\nline-ending = "lf"\n')
        selected = self.write("selected.py", "value = 1\r\nother = 2\n")
        excluded = self.write("excluded/blocked.py", "this is not valid Python\r\n")
        self.git("add", ".")
        tree = self.git("write-tree")
        contents = selected.read_bytes()
        self.assertEqual(subprocess.run(["git", "diff", "--quiet"], cwd=self.root).returncode, 0)
        self.assertEqual(hooks.check_files("python-ruff", ["selected.py"]), 0)
        self.assertEqual(selected.read_bytes(), contents)
        selected.write_bytes(b"value=  1\r\nother = 2\n")
        invalid = selected.read_bytes()
        with patch.object(sys, "stdout", new_callable=io.StringIO) as output:
            self.assertEqual(hooks.check_files("python-ruff", ["selected.py"]), 1)
        self.assertIn("ruff format validation failed", output.getvalue())
        self.assertEqual(selected.read_bytes(), invalid)
        selected.write_bytes('print(\u672a\u5b9a\u4e49)\r\nother = 2\n'.encode())
        invalid = selected.read_bytes()
        output_bytes = io.BytesIO()
        stream = io.TextIOWrapper(output_bytes, encoding="cp1252", write_through=True)
        with patch.object(sys, "stdout", stream):
            self.assertEqual(hooks.check_files("python-ruff", ["selected.py"]), 1)
        self.assertIn(b"ruff check validation failed", output_bytes.getvalue())
        self.assertIn(b"F821", output_bytes.getvalue())
        self.assertEqual(selected.read_bytes(), invalid)
        original_read = Path.read_bytes

        def read_bytes(path):
            if path == excluded:
                raise PermissionError("The excluded input is held by another process.")
            return original_read(path)

        with patch.object(Path, "read_bytes", read_bytes):
            self.assertEqual(hooks.check_files("python-ruff", ["excluded/blocked.py"]), 0)
        self.assertEqual(self.git("write-tree"), tree)

    def test_buildifier_uses_ci_exclusions(self):
        source = REPO / "bazel/buildifier/BUILD.bazel"
        self.write("bazel/buildifier/BUILD.bazel", source.read_text("utf-8"))
        paths = ["BUILD.bazel", "third_party/x/BUILD", ".cache/BUILD", "deps/x/overlay/x.bzl", "vendor/BUILD"]
        self.assertEqual(hooks.buildifier_files(paths), ["BUILD.bazel"])
        with patch.object(hooks, "buildifier_path", return_value=self.root / "missing-buildifier"):
            self.assertEqual(hooks.check_files("buildifier", paths[1:]), 0)

    def test_buildifier_selects_ci_filename_variants(self):
        names = [
            "rules.bzl",
            "rules.sky",
            "rules.star",
            "BUILD.bazel",
            "MODULE.bazel",
            "REPO.bazel",
            "VENDOR.bazel",
            "BUILD",
            "overlay.BUILD",
            "BUILD.oss",
            "BUILD.linux.oss",
            "WORKSPACE",
            "WORKSPACE.oss",
            "WORKSPACE.bzlmod",
            "WORKSPACE.linux.oss",
        ]
        for name in names:
            with self.subTest(name=name):
                self.assertTrue(hooks.eligible("buildifier", "nested/" + name))
        for name in ["README", "README.oss", "BUILDING.md", "WORKSPACE.lock"]:
            with self.subTest(name=name):
                self.assertFalse(hooks.eligible("buildifier", "nested/" + name))

    @unittest.skipUnless(shutil.which("rustup"), "rustup is required")
    def test_rustfmt_does_not_follow_modules(self):
        self.write(".gitattributes", "* text eol=lf\n")
        self.write("MODULE.bazel", (REPO / "MODULE.bazel").read_text("utf-8"))
        version, _ = hooks.rust_settings()
        result = subprocess.run(["rustup", "which", "--toolchain", version, "rustfmt"], capture_output=True)
        if result.returncode:
            self.skipTest(f"Rust {version} is required")
        self.write("lib.rs", "mod child;\n")
        child = self.write("child.rs", "this is invalid Rust\n")
        self.assertEqual(hooks.check_files("rustfmt", ["lib.rs"]), 0)
        self.assertEqual(child.read_text("utf-8"), "this is invalid Rust\n")
        with patch.object(sys, "stdout", new_callable=io.StringIO) as output:
            self.assertEqual(hooks.check_files("rustfmt", ["child.rs"]), 1)
        self.assertIn("child.rs:1:6", output.getvalue())
        self.assertNotIn("\\\\?\\", output.getvalue())
        unformatted = self.write("unformatted.rs", 'pub fn f(){println!("ready");}\n')
        before = unformatted.read_bytes()
        with patch.object(sys, "stdout", new_callable=io.StringIO) as output:
            self.assertEqual(hooks.check_files("rustfmt", ["unformatted.rs"]), 1)
        self.assertIn("unformatted.rs:1:", output.getvalue())
        self.assertNotIn("\\\\?\\", output.getvalue())
        self.assertEqual(unformatted.read_bytes(), before)
        self.write("rustfmt.toml", "hard_tabs = true\n")
        self.write("formatted.rs", "pub fn f() {\n    println!(\"ready\");\n}\n")
        self.assertEqual(hooks.check_files("rustfmt", ["formatted.rs"]), 0)
        formatted = self.root / "formatted.rs"
        formatted.write_bytes(formatted.read_bytes().replace(b"\n", b"\r\n"))
        contents = formatted.read_bytes()
        self.assertEqual(hooks.check_files("rustfmt", ["formatted.rs"]), 0)
        self.assertEqual(formatted.read_bytes(), contents)
        skipped = self.write("skipped.rs", "#![rustfmt::skip]\npub fn f(){println!(\"ready\");}\n")
        skipped_contents = skipped.read_bytes()
        self.assertEqual(hooks.check_files("rustfmt", ["skipped.rs"]), 0)
        self.assertEqual(skipped.read_bytes(), skipped_contents)
        self.write(".gitattributes", "* text eol=lf\nskipped.rs -text\n")
        for contents in [
            b"\xef\xbb\xbf" + skipped_contents,
            skipped_contents.replace(b"\n", b"\r\n"),
            b"\xef\xbb\xbf" + skipped_contents.replace(b"\n", b"\r\n"),
        ]:
            skipped.write_bytes(contents)
            self.assertEqual(hooks.check_files("rustfmt", ["skipped.rs"]), 0)
            self.assertEqual(skipped.read_bytes(), contents)
        self.write("unicode.rs", 'pub fn f(){println!("\u65e5");}\n')
        output = io.BytesIO()
        stream = io.TextIOWrapper(output, encoding="cp1252", write_through=True)
        with patch.object(sys, "stdout", stream):
            self.assertEqual(hooks.check_files("rustfmt", ["unicode.rs"]), 1)
        self.assertIn(b"unicode.rs: rustfmt failed", output.getvalue())
        self.assertIn(b"\\u65e5", output.getvalue())

    def test_buildifier_normalizes_crlf_without_rewriting(self):
        self.write(".gitattributes", "* text eol=lf\n")
        with patch.object(hooks, "ROOT", REPO):
            executable = hooks.buildifier_path()
        if not executable.is_file():
            self.skipTest("Install the pinned Buildifier with tasks/hooks.py setup-buildifier")
        self.write(".buildifier.json", (REPO / ".buildifier.json").read_text("utf-8"))
        self.write("bazel/buildifier/BUILD.bazel", (REPO / "bazel/buildifier/BUILD.bazel").read_text("utf-8"))
        target = self.write("example.bzl", '"""Fixture."""\n\nVALUE = 1\n')
        target.write_bytes(target.read_bytes().replace(b"\n", b"\r\n"))
        contents = target.read_bytes()
        with patch.object(hooks, "buildifier_path", return_value=executable):
            self.assertEqual(hooks.check_files("buildifier", ["example.bzl"]), 0)
            self.assertEqual(target.read_bytes(), contents)
            (self.root / ".buildifier.json").unlink()
            self.assertEqual(hooks.check_files("buildifier", ["example.bzl"]), 0)
            self.assertEqual(target.read_bytes(), contents)
            target.write_bytes(b'"""Fixture."""\r\n\r\nVALUE=1\r\n')
            invalid = target.read_bytes()
            self.assertEqual(hooks.check_files("buildifier", ["example.bzl"]), 1)
            self.assertEqual(target.read_bytes(), invalid)


if __name__ == "__main__":
    unittest.main()
