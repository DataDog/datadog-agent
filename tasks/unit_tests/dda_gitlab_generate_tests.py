import io
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stderr
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / ".dda" / "extend" / "pythonpath"))

from utils.gitlab.context import is_documentation, is_no_code_change  # noqa: E402
from utils.gitlab.generate import HEADER, NO_CODE_JOB, NO_CODE_MESSAGE, generate, main  # noqa: E402

SOURCE = """---
.if_main_branch: &if_main_branch
  if: $CI_COMMIT_BRANCH == "main"

include:
  - local: .gitlab/.pre/**.yml

variables:
  CI_IMAGE_LINUX: v123-abcdef
  CI_IMAGE_LINUX_SUFFIX: ""
  OTHER: value

some_job:
  rules:
    - !reference [.on_main]
    - <<: *if_main_branch
  script: echo
"""


def make_context(*paths, kind="FILE_MODIFICATION_KIND_EDITED", ref="my-branch", **analysis):
    return {
        "request": {
            "kind": "REQUEST_KIND_CHANGE_REQUEST",
            "ref": ref,
            "repository": {"default_branch": "main"},
        },
        "bia_analysis": {
            "changed_files": [{"path": path, "kind": kind} for path in paths],
            "changed_files_missing": False,
            "large_scale_change": False,
            **analysis,
        },
    }


DOC_CONTEXT = make_context("doc/index.md", "pkg/README.md")


class TestIsNoCodeChange(unittest.TestCase):
    def test_documentation_only(self):
        self.assertTrue(is_no_code_change(DOC_CONTEXT))

    def test_code(self):
        self.assertFalse(is_no_code_change(make_context("doc/index.md", "pkg/main.go")))

    def test_not_a_change_request(self):
        context = make_context("doc/index.md")
        context["request"]["kind"] = "REQUEST_KIND_INTEGRATION_REQUEST"
        self.assertFalse(is_no_code_change(context))

    def test_scheduled(self):
        context = make_context("doc/index.md")
        context["request"]["scheduled_origin"] = True
        self.assertFalse(is_no_code_change(context))

    def test_protected_refs(self):
        for ref in ("main", "refs/heads/main", "7.70.x", "mq-working-branch-main-1234", "refs/tags/7.70.0"):
            with self.subTest(ref=ref):
                self.assertFalse(is_no_code_change(make_context("doc/index.md", ref=ref)))

    def test_incomplete_analysis(self):
        self.assertFalse(is_no_code_change(make_context("doc/index.md", changed_files_missing=True)))
        self.assertFalse(is_no_code_change(make_context("doc/index.md", large_scale_change=True)))
        self.assertFalse(is_no_code_change(make_context()))
        self.assertFalse(is_no_code_change({"request": DOC_CONTEXT["request"]}))

    def test_unknown_analysis_flags(self):
        for flag in ("changed_files_missing", "large_scale_change"):
            with self.subTest(flag=flag):
                context = make_context("doc/index.md")
                del context["bia_analysis"][flag]
                self.assertFalse(is_no_code_change(context))

    def test_renamed(self):
        self.assertFalse(is_no_code_change(make_context("doc/index.md", kind="FILE_MODIFICATION_KIND_RENAMED")))

    def test_is_documentation(self):
        for path in ("doc/index.md", "doc/assets/image.png", "docs/README.md", "pkg/foo/README.MD", "AGENTS.md"):
            with self.subTest(path=path):
                self.assertTrue(is_documentation(path))
        for path in (
            "pkg/foo/main.go",
            "releasenotes/notes/foo.yaml",
            "packages/install_dir/embedded/etc/README.md",
            "omnibus/README.md",
            "Dockerfiles/agent/README.md",
            ".gitlab/README.md",
            ".github/CODEOWNERS",
            "",
        ):
            with self.subTest(path=path):
                self.assertFalse(is_documentation(path))


class TestGenerate(unittest.TestCase):
    def test_full(self):
        self.assertEqual(generate(SOURCE), HEADER + SOURCE)
        self.assertEqual(generate(SOURCE, make_context("pkg/main.go")), HEADER + SOURCE)

    def test_no_code(self):
        config = yaml.safe_load(generate(SOURCE, DOC_CONTEXT))

        self.assertEqual(config["variables"], {"CI_IMAGE_LINUX": "v123-abcdef", "CI_IMAGE_LINUX_SUFFIX": ""})
        jobs = {name: job for name, job in config.items() if name != "variables"}
        self.assertEqual(list(jobs), [NO_CODE_JOB])
        self.assertEqual(jobs[NO_CODE_JOB]["script"], [f'echo "{NO_CODE_MESSAGE}"'])
        self.assertNotIn(jobs[NO_CODE_JOB]["stage"], (".pre", ".post"))

    def test_no_code_missing_image(self):
        with self.assertRaises(ValueError):
            generate("variables:\n  OTHER: value\n", DOC_CONTEXT)

    def test_no_code_on_repository_source(self):
        config = yaml.safe_load(generate((ROOT / "gitlab-source.yml").read_text(encoding="utf-8"), DOC_CONTEXT))

        self.assertIn(NO_CODE_JOB, config)


class TestMain(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.tmp)
        (self.tmp / "source.yml").write_text(SOURCE, encoding="utf-8")

    def run_main(self, *args):
        with redirect_stderr(io.StringIO()):
            return main(["--source", str(self.tmp / "source.yml"), "-o", str(self.tmp / "output.yml"), *args])

    def test_without_context(self):
        self.assertEqual(self.run_main(), 0)
        self.assertEqual((self.tmp / "output.yml").read_text(encoding="utf-8"), HEADER + SOURCE)

    def test_context(self):
        (self.tmp / "context.json").write_text(json.dumps(DOC_CONTEXT), encoding="utf-8")

        self.assertEqual(self.run_main("--context", str(self.tmp / "context.json")), 0)
        self.assertIn(NO_CODE_JOB, yaml.safe_load((self.tmp / "output.yml").read_text(encoding="utf-8")))

    def test_invalid_context(self):
        for content in ("{", "[]"):
            with self.subTest(content=content):
                (self.tmp / "context.json").write_text(content, encoding="utf-8")
                self.assertEqual(self.run_main("--context", str(self.tmp / "context.json")), 2)

    def test_missing_source(self):
        with redirect_stderr(io.StringIO()):
            self.assertEqual(main(["--source", str(self.tmp / "missing.yml"), "-o", "-"]), 2)


# Limits enforced by the DDCI task engine on the generator inputs
MAX_INPUT_FILES = 500
MAX_INPUT_BYTES = 20 * 1024 * 1024


def doublestar_regex(pattern):
    """Translate a glob into a regex with the semantics of github.com/bmatcuk/doublestar, used by the engine"""
    regex = ""
    i = 0
    while i < len(pattern):
        if pattern.startswith("**/", i):
            regex += "(?:.*/)?"
            i += 3
        elif pattern.startswith("**", i):
            regex += ".*"
            i += 2
        elif pattern[i] == "*":
            regex += "[^/]*"
            i += 1
        elif pattern[i] == "?":
            regex += "[^/]"
            i += 1
        else:
            regex += re.escape(pattern[i])
            i += 1
    return re.compile(regex + "$")


class TestGeneratorManifest(unittest.TestCase):
    """Runs the generator the way the DDCI task engine does, with only the files listed in .gitlab/generator.yml"""

    def input_files(self, patterns):
        # The engine matches the inputs against the repository tree at the head commit
        files = subprocess.run(
            ["git", "ls-files", "--cached", "--others", "--exclude-standard"],
            cwd=ROOT,
            capture_output=True,
            text=True,
            check=True,
        ).stdout.splitlines()
        matched = set()
        for pattern in patterns:
            regex = doublestar_regex(pattern)
            paths = {path for path in files if regex.match(path) and (ROOT / path).is_file()}
            self.assertTrue(paths, f"Input {pattern} matches no file")
            matched |= paths
        self.assertLessEqual(len(matched), MAX_INPUT_FILES)
        self.assertLessEqual(sum((ROOT / path).stat().st_size for path in matched), MAX_INPUT_BYTES)
        return matched

    def run_generator(self, context):
        manifest = yaml.safe_load((ROOT / ".gitlab" / "generator.yml").read_text(encoding="utf-8"))
        self.assertEqual(manifest["command"][0], "python3")
        with tempfile.TemporaryDirectory() as tmp:
            for path in self.input_files(manifest["inputs"]):
                (Path(tmp) / path).parent.mkdir(parents=True, exist_ok=True)
                shutil.copy(ROOT / path, Path(tmp) / path)
            context_path = Path(tmp) / "context.json"
            context_path.write_text(json.dumps(context), encoding="utf-8")

            command = [arg.replace("{context}", str(context_path)) for arg in manifest["command"]]
            # The interpreter running the tests has PyYAML, like the task engine image
            command[0] = sys.executable
            env = {"PATH": os.environ.get("PATH", ""), **manifest["env"]}
            result = subprocess.run(command, cwd=tmp, env=env, capture_output=True, text=True, timeout=20)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result.stdout

    def test_no_code(self):
        self.assertIn(NO_CODE_JOB, yaml.safe_load(self.run_generator(DOC_CONTEXT)))

    def test_full(self):
        source = (ROOT / "gitlab-source.yml").read_text(encoding="utf-8")
        self.assertEqual(self.run_generator(make_context("pkg/main.go")), HEADER + source)
