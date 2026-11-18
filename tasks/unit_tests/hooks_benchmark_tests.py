"""Qualify the harness against a tiny Git repository and nine local system hooks."""

import importlib.util
import json
import os
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location(
    "hooks_benchmark", Path(__file__).resolve().parents[1] / "hooks_benchmark.py"
)
benchmark = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(benchmark)

VALIDATOR = """\
import os
import sys
from pathlib import Path

hook, *paths = sys.argv[1:]
mode = os.environ.get("QUALIFICATION_TEST_FAILURE")
if hook == "rustfmt" and mode == "preflight" and "hook_qualification.rs" in paths:
    sys.exit("Rust tool unavailable in fixture")
if hook == "go-fmt" and ".go-version" in paths:
    marker = Path(".git/qualification-runs")
    if mode == "repeat" and marker.exists():
        sys.exit("Fixture formatter could not launch")
    marker.touch()
if hook == "copyright" and mode == "untracked":
    Path("unexpected-output").touch()
"""


class TestHooksBenchmark(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.source = self.root / "source"
        self.source.mkdir()
        self.env = benchmark.fixture_environment(self.source)
        self.git("init", "--initial-branch=feature", "--template=")
        self.git("config", "user.name", "Hook tests")
        self.git("config", "user.email", "hooks@example.com")
        self.git("config", "core.autocrlf", "false")
        self.write("tasks/hooks.py", VALIDATOR)
        self.write(".gitattributes", "*.bat text eol=crlf\n")
        self.write("sample.bat", "echo sample\n")
        self.write(".gitignore", ".cache/\n")
        self.write("sentinel", "committed\n")
        for path in [".go-version", "doc/setup/optional.md"]:
            self.write(path, "setting\n")
        hooks = [
            {
                "id": hook,
                "name": hook,
                "language": "system",
                "entry": f'"{Path(sys.executable).as_posix()}" tasks/hooks.py {hook}',
                "always_run": True,
            }
            for hook in sorted(benchmark.EXPECTED_HOOKS)
        ]
        self.write(".pre-commit-config.yaml", json.dumps({"repos": [{"repo": "local", "hooks": hooks}]}))
        self.git("add", ".")
        self.git("update-index", "--chmod=+x", "sentinel")
        oid = self.git("hash-object", "-w", "--stdin", payload=b"sentinel").decode().strip()
        self.git("update-index", "--add", "--cacheinfo", f"120000,{oid},link")
        self.git("update-index", "--add", "--cacheinfo", f"160000,{'1' * 40},submodule")
        self.git("commit", "-qm", "Qualification fixture")
        self.archive = self.root / "fixture.tar"
        self.git("archive", "--format=tar", f"--output={self.archive}", "HEAD")

    def git(self, *args, payload=None):
        return subprocess.run(
            ["git", *args], cwd=self.source, env=self.env, input=payload, stdout=subprocess.PIPE, check=True
        ).stdout

    def write(self, path, contents):
        target = self.source / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(contents, encoding="utf-8", newline="\n")

    def verify(self, archive):
        with tarfile.open(archive) as snapshot:
            return benchmark.verified_archive(snapshot, self.source, self.env)

    def test_full_archive_checks_contents_modes_and_symlink_targets(self):
        revision, members = self.verify(self.archive)
        self.assertEqual(revision, self.git("rev-parse", "HEAD").decode().strip())
        self.assertEqual(members["link"][0], "120000")
        self.assertEqual(members["sentinel"][0], "100755")
        self.assertNotIn("submodule", members)
        with tarfile.open(self.archive) as snapshot:
            self.assertEqual(benchmark.archive_payload(snapshot, members["sample.bat"][1]), b"echo sample\r\n")
        self.assertEqual(self.git("cat-file", "blob", "HEAD:sample.bat"), b"echo sample\n")
        partial = self.root / "partial.tar"
        self.git("archive", "--format=tar", f"--output={partial}", "HEAD", "tasks")
        with self.assertRaisesRegex(ValueError, "omits"):
            self.verify(partial)
        for change in ("mode", "symlink", "content"):
            with self.subTest(change=change), tarfile.open(self.archive) as snapshot:
                with tarfile.open(partial, "w", pax_headers=snapshot.pax_headers) as altered:
                    for member in snapshot:
                        source = snapshot.extractfile(member) if member.isfile() else None
                        if member.name == "sentinel" and change == "mode":
                            member.mode = 0o644
                        if member.name == "link" and change == "symlink":
                            member.linkname = "other"
                        if member.name == "sentinel" and change == "content":
                            member.size -= 1
                        altered.addfile(member, source)
                with self.assertRaisesRegex(ValueError, "(mode|content) differs"):
                    self.verify(partial)

    def test_archive_identity_ignores_local_attributes_and_replacements(self):
        revision = self.git("rev-parse", "HEAD").decode().strip()
        self.write(".git/info/attributes", "sample.bat eol=lf\n")
        attributes = self.root / "xdg" / "git" / "attributes"
        attributes.parent.mkdir(parents=True)
        attributes.write_text("sentinel export-ignore\n", encoding="utf-8")
        self.env["XDG_CONFIG_HOME"] = str(attributes.parent.parent)
        self.verify(self.archive)

        altered = self.root / "altered.tar"
        self.git("archive", "--format=tar", f"--output={altered}", "HEAD")
        with tarfile.open(altered) as snapshot:
            self.assertEqual(snapshot.extractfile("sample.bat").read(), b"echo sample\n")
        with self.assertRaisesRegex(ValueError, "content differs"):
            self.verify(altered)

        self.write("sentinel", "replacement\n")
        self.git("add", "sentinel")
        self.git("commit", "-qm", "Replacement fixture")
        replacement = self.git("rev-parse", "HEAD").decode().strip()
        self.git("replace", revision, replacement)
        self.assertEqual(self.git("cat-file", "blob", f"{revision}:sentinel"), b"committed\n")
        self.verify(self.archive)

    def test_archive_rejects_a_tree_as_the_recorded_commit(self):
        altered = self.root / "tree.tar"
        tree = self.git("rev-parse", "HEAD^{tree}").decode().strip()
        with (
            tarfile.open(self.archive) as snapshot,
            tarfile.open(altered, "w", pax_headers={"comment": tree}) as target,
        ):
            for member in snapshot:
                target.addfile(member, snapshot.extractfile(member) if member.isfile() else None)
        with self.assertRaisesRegex(ValueError, "must identify a commit"):
            self.verify(altered)

    def test_only_shellcheck_diagnostics_qualify_a_failed_hook(self):
        lines = [f"{hook}.....Passed" for hook in sorted(benchmark.EXPECTED_HOOKS)]
        baseline = "\n".join(lines)
        self.assertEqual(benchmark.classify("mixed", 0, baseline), "passed")
        finding = "./file.sh:1:3: warning: Quote this variable. [SC2086]"
        output = baseline.replace(
            "shellcheck.....Passed", f"shellcheck.....Failed\n- hook id: shellcheck\n- exit code: 1\n\n{finding}"
        )
        self.assertEqual(benchmark.classify("shellcheck-settings", 1, output), "shellcheck_findings")
        for name, code, text in [
            ("shellcheck-settings", 1, output + "\nCannot open input"),
            ("shellcheck-settings", 1, output.replace(finding, "tool missing")),
            ("shellcheck-settings", -15, output),
            ("mixed", 1, output),
            ("mixed", 0, baseline.replace("rustfmt.....Passed", "rustfmt.....Skipped")),
            ("mixed", 0, baseline.replace("rustfmt", "go-fmt")),
            ("mixed", 0, ""),
        ]:
            with self.subTest(name=name, code=code, text=text):
                self.assertEqual(benchmark.classify(name, code, text), "unexpected_failure")

    @unittest.skipUnless(shutil.which("pre-commit"), "requires pre-commit")
    def test_cli_checkpoints_evidence_before_scenario_preparation(self):
        output = self.root / "report.json"
        completed = subprocess.run(
            [
                sys.executable,
                str(Path(benchmark.__file__).resolve()),
                str(self.archive),
                "--source-repository",
                str(self.source),
                "--runs",
                "1",
                "--output",
                str(output),
            ],
            cwd=self.root,
            env=self.env,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
        )
        self.assertNotEqual(completed.returncode, 0)
        self.assertIn(b"pyproject.toml", completed.stdout)
        report = json.loads(output.read_text(encoding="utf-8"))
        self.assertFalse(report["completed"])
        self.assertEqual(report["phase"], "python-settings:prepare")
        self.assertEqual(
            set(report["scenarios"]), {"documentation", "mixed", "mixed-crlf", "partially-staged", "go-settings"}
        )
        for scenario in report["scenarios"].values():
            self.assertEqual(len(scenario["invocations"]), 1)
            invocation = scenario["invocations"][0]
            self.assertEqual(invocation["classification"], "passed")
            self.assertTrue(invocation["output"])
            self.assertTrue(invocation["preserved_index_and_worktree"])

    @unittest.skipUnless(shutil.which("pre-commit"), "requires pre-commit")
    def test_preflight_rejects_fixture_tool_failure_and_untracked_output(self):
        for failure in ("preflight", "untracked"):
            with self.subTest(failure=failure), patch.dict(os.environ, QUALIFICATION_TEST_FAILURE=failure):
                report = benchmark.benchmark(self.archive, 1, self.source)
                self.assertFalse(report["completed"])
                self.assertEqual(report["scenarios"], {})
                preflight = report["preflight"]
                if failure == "preflight":
                    self.assertEqual(preflight["classification"], "unexpected_failure")
                    self.assertIn("Rust tool unavailable in fixture", preflight["output"])
                else:
                    self.assertEqual(preflight["classification"], "passed")
                    self.assertFalse(preflight["preserved_index_and_worktree"])

    @unittest.skipUnless(shutil.which("pre-commit"), "requires pre-commit")
    def test_failed_settings_run_retains_evidence_and_preserves_caller(self):
        self.write("sentinel", "staged\n")
        self.git("add", "sentinel")
        self.write("sentinel", "unstaged\n")
        caller = [self.git("rev-parse", "HEAD"), self.git("write-tree"), self.git("diff", "--binary")]
        hostile = {
            "GIT_DIR": str(self.source / ".git"),
            "GIT_WORK_TREE": str(self.source),
            "GIT_INDEX_FILE": str(self.source / ".git/index"),
            "GIT_CONFIG_COUNT": "1",
            "GIT_CONFIG_KEY_0": "core.hooksPath",
            "GIT_CONFIG_VALUE_0": str(self.root / "absent"),
            "GIT_CONFIG_GLOBAL": str(self.root / "config"),
            "SKIP": ",".join(benchmark.EXPECTED_HOOKS),
            "PRE_COMMIT_HOME": str(self.root / "caller-cache"),
            "PRE_COMMIT_FROM_REF": "missing",
            "QUALIFICATION_TEST_FAILURE": "repeat",
        }
        with patch.dict(os.environ, hostile):
            report = benchmark.benchmark(self.archive, 2, self.source)
            self.assertEqual({key: os.environ[key] for key in hostile}, hostile)
        self.assertFalse(report["completed"])
        self.assertEqual(report["preflight"]["classification"], "passed")
        first, second = report["scenarios"]["go-settings"]["invocations"]
        self.assertEqual(first["classification"], "passed")
        self.assertEqual(second["classification"], "unexpected_failure")
        self.assertEqual(second["exit_code"], 1)
        self.assertIn("Fixture formatter could not launch", second["output"])
        self.assertEqual(caller, [self.git("rev-parse", "HEAD"), self.git("write-tree"), self.git("diff", "--binary")])
        self.assertEqual((self.source / "sentinel").read_bytes(), b"unstaged\n")
        self.assertFalse((self.root / "caller-cache").exists())
