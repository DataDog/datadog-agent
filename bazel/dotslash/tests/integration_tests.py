"""Exercises Bazel actions and direct Buildozer edits with isolated workspaces and caches."""

import ast
import json
import os
import re
import shutil
import subprocess
import tempfile
import tomllib
import unittest
from pathlib import Path


class BuildozerTests(unittest.TestCase):
    def test_dictionary_edit_from_callers_directory(self):
        source = Path(__file__).resolve().parents[3]
        launcher = source / "tools/bin" / ("buildozer.exe" if os.name == "nt" else "buildozer")
        with tempfile.TemporaryDirectory(prefix="dotslash-buildozer-") as temporary:
            root = Path(temporary)
            (root / "MODULE.bazel").touch()
            package = root / "package with spaces"
            package.mkdir()
            fixture = package / "BUILD.bazel"
            fixture.write_text('go_download_sdk(name = "sdk", sdks = {})\n', encoding="utf-8")
            expected = {"windows_amd64": ["z-first%DOTSLASH_LITERAL%&.zip", "a-second-sha256"]}
            expression = json.dumps(expected, separators=(",", ":"))
            command = [str(launcher), f"set sdks:expr {expression}", ":sdk"]
            environment = {**os.environ, "DOTSLASH_CACHE": str(root / "cache"), "DOTSLASH_LITERAL": "must-not-expand"}
            for status in (0, 3):
                with self.subTest(expected_status=status):
                    result = subprocess.run(
                        command,
                        cwd=package,
                        env=environment,
                        stdin=subprocess.DEVNULL,
                        capture_output=True,
                        text=True,
                        timeout=300,
                        check=False,
                    )
                    self.assertEqual(result.returncode, status, result.stdout + result.stderr)
                    call = ast.parse(fixture.read_text(encoding="utf-8")).body[0].value
                    sdks = next(keyword.value for keyword in call.keywords if keyword.arg == "sdks")
                    self.assertEqual(ast.literal_eval(sdks), expected)


class DotSlashBazelTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.source = Path(__file__).resolve().parents[3]
        cls.temporary = tempfile.TemporaryDirectory(prefix="dotslash-bazel-")
        cls.addClassCleanup(cls.temporary.cleanup)
        cls.root = Path(cls.temporary.name)
        cls.workspace = cls.root / "workspace"
        cls.workspace.mkdir()
        cls.bazel = shutil.which("bazelisk") or shutil.which("bazel")
        if cls.bazel is None:
            raise RuntimeError("Bazelisk must be available to run the Bazel adapter tests.")
        cls.environment = dict(os.environ)
        cls.environment["DOTSLASH_LITERAL"] = "must-not-expand"
        if os.name == "nt":
            git_tools = subprocess.check_output(["git", "--exec-path"], text=True).strip()
            bash = Path(git_tools).parents[2] / "bin/bash.exe"
            if bash.is_file():
                # Bazel must use Git Bash rather than the similarly named WSL launcher.
                cls.environment["BAZEL_SH"] = str(bash)
        cls.startup = [
            cls.bazel,
            "--max_idle_secs=60",
            "--nosystem_rc",
            "--nohome_rc",
            f"--output_base={Path(os.environ.get('DOTSLASH_BAZEL_OUTPUT_BASE', cls.root / 'output')).as_posix()}",
            "--host_jvm_args=-Xmx1024m",
            # Some Windows hosts cannot connect to the server over IPv6 loopback.
            "--host_jvm_args=-Djava.net.preferIPv4Stack=true",
        ]
        for directory in ("bazel/dotslash", "tools/bin"):
            shutil.copytree(cls.source / directory, cls.workspace / directory)
        for filename in ("mise.toml", ".bazelversion"):
            shutil.copy2(cls.source / filename, cls.workspace / filename)
        (cls.workspace / "BUILD.bazel").write_text('exports_files(["mise.toml"])\n', encoding="utf-8")
        module = (cls.source / "MODULE.bazel").read_text(encoding="utf-8")
        dependencies = "\n".join(
            re.search(rf'^bazel_dep\(name = "{re.escape(name)}",.*$', module, re.MULTILINE)[0]
            for name in ("platforms", "toml.bzl", "rules_python")
        )
        (cls.workspace / "MODULE.bazel").write_text(
            dependencies + '\nhost_dotslash = use_repo_rule("//bazel/dotslash:repositories.bzl", "host_dotslash")\n'
            'host_dotslash(name = "dotslash_host")\n'
            'register_toolchains("@dotslash_host//:toolchain")\n',
            encoding="utf-8",
        )
        (cls.workspace / ".bazelrc").write_text(
            'common --experimental_strict_repo_env\n'
            'common --repo_env=BAZEL_DO_NOT_DETECT_CPP_TOOLCHAIN=1\n'
            'common --repo_env=HOME\n'
            'common --repo_env=USERPROFILE\n'
            'common --repo_env=SYSTEMROOT\n'
            'common --@rules_python//python/config_settings:bootstrap_impl=script\n',
            encoding="utf-8",
        )
        cls.addClassCleanup(
            subprocess.run,
            [*cls.startup, "shutdown"],
            cwd=cls.workspace,
            env=cls.environment,
            stdin=subprocess.DEVNULL,
            capture_output=True,
            timeout=60,
            check=True,
        )

    def setUp(self):
        self.test_root = self.root / self._testMethodName
        self.test_root.mkdir()
        self.cache = self.test_root / "cache"
        for relative in ("mise.toml", "tools/bin/buildifier", "bazel/dotslash/tests/input.txt"):
            path = self.workspace / relative
            self.addCleanup(path.write_bytes, path.read_bytes())

    def bazel_command(self, command, *args, success=True):
        result = subprocess.run(
            [*self.startup, command, f"--repo_env=DOTSLASH_CACHE={self.cache}", "--noshow_progress", *args],
            cwd=self.workspace,
            env=self.environment,
            stdin=subprocess.DEVNULL,
            capture_output=True,
            encoding="utf-8",
            errors="replace",
            timeout=600,
            check=False,
        )
        self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)
        return result

    def test_execution_and_invalidation(self):
        target = "//bazel/dotslash/tests:formatted"
        log = self.test_root / "execution.json"

        def build():
            self.bazel_command("build", target, f"--execution_log_json_file={log}")
            return log.read_text(encoding="utf-8")

        self.assertFalse(self.cache.exists())
        self.assertIn("DotSlashFixture", build())
        outputs = self.bazel_command("cquery", target, "--output=files").stdout.strip().splitlines()
        execution_root = self.bazel_command("info", "execution_root").stdout.strip()
        self.assertEqual((Path(execution_root) / outputs[-1]).read_text(encoding="utf-8"), "value = [1, 2]\n")
        self.assertTrue(self.cache.exists())
        self.assertNotIn("DotSlashFixture", build())

        manifest = self.workspace / "tools/bin/buildifier"
        original = manifest.read_text(encoding="utf-8")
        definition = json.loads(original.split("\n", 1)[1])
        definition["metadata"]["bazel_fixture_revision"] = 1
        manifest.write_text("#!/usr/bin/env dotslash\n" + json.dumps(definition), encoding="utf-8")
        self.assertIn("DotSlashFixture", build())

        config = self.workspace / "mise.toml"
        with config.open("a", encoding="utf-8") as file:
            file.write("\n# Unrelated configuration edits preserve action reuse.\n")
        self.assertNotIn("DotSlashFixture", build())

        self.cache = self.test_root / "alternate-cache"
        self.assertIn("DotSlashFixture", build())
        self.assertTrue(self.cache.exists())
        self.assertNotIn("DotSlashFixture", build())

        graph = json.loads(self.bazel_command("aquery", target, "--output=jsonproto").stdout)
        action = next(action for action in graph["actions"] if action["mnemonic"] == "DotSlashFixture")
        self.assertIn({"key": "local", "value": "1"}, action["executionInfo"])

    def test_arguments_and_exit_status(self):
        fixture = self.workspace / "bazel/dotslash/tests/input.txt"
        fixture.write_text("not valid Starlark [", encoding="utf-8")
        failure = self.bazel_command("build", "//bazel/dotslash/tests:formatted", success=False)
        self.assertRegex(failure.stderr, r"(?m)^file %DOTSLASH_LITERAL% & space\.bzl:\d+:\d+: syntax error")
        fixture.write_text("value=[1,2]\n", encoding="utf-8")
        self.bazel_command("build", "//bazel/dotslash/tests:formatted")

    def test_missing_and_wrong_dotslash(self):
        empty = self.test_root / "empty-path"
        empty.mkdir()
        missing = f"--repo_env=PATH={empty}"
        self.bazel_command("query", "//tools/bin:*", missing)
        self.bazel_command("build", "//tools/bin:buildifier_tool", missing)
        self.bazel_command("build", "//bazel/dotslash/tests:capture", missing)
        failure = self.bazel_command("build", "//bazel/dotslash/tests:formatted", missing, success=False)
        self.assertIn("DotSlash is unavailable", failure.stderr)

        config = self.workspace / "mise.toml"
        original = config.read_text(encoding="utf-8")
        version = tomllib.loads(original)["tools"]["dotslash"]["version"]
        config.write_text(original.replace(f'version = "{version}"', 'version = "0.0.0"', 1), encoding="utf-8")
        self.bazel_command("build", "//tools/bin:buildifier_tool")
        failure = self.bazel_command("build", "//bazel/dotslash/tests:formatted", success=False)
        self.assertIn("Expected DotSlash 0.0.0", failure.stderr)

    def test_invalid_configuration_only_fails_dotslash_consumers(self):
        for configuration in (
            "[tools\n",
            "[tools]\n",
            '[tools]\n"aqua:facebook/dotslash" = { version = "0.5.9" }\n',
            '[tools]\ndotslash = "0.5.9"\n',
            '[tools]\ndotslash = { version = "" }\n',
            "[tools]\ndotslash = { version = 42 }\n",
        ):
            with self.subTest(configuration=configuration):
                (self.workspace / "mise.toml").write_text(configuration, encoding="utf-8")
                self.bazel_command("build", "//tools/bin:buildifier_tool", "//bazel/dotslash/tests:capture")
                failure = self.bazel_command("build", "//bazel/dotslash/tests:formatted", success=False)
                self.assertIn("Invalid DotSlash pin in mise.toml", failure.stderr)

    def test_consumers_resolve_the_selected_runtime(self):
        failure = self.bazel_command(
            "build",
            "//bazel/dotslash/tests:formatted",
            "--extra_toolchains=//bazel/dotslash/tests:unavailable_toolchain",
            success=False,
        )
        self.assertIn("The alternate DotSlash runtime was selected.", failure.stderr)


if __name__ == "__main__":
    unittest.main()
