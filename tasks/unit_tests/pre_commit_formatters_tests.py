import json
import os
import shutil
import subprocess
from pathlib import Path

from invoke.exceptions import Exit

from tasks import pre_commit
from tasks.unit_tests.pre_commit_foundation_tests import GitFixture

REPOSITORY = Path(__file__).resolve().parents[2]
BUILDIFIER = Path("bin", "buildifier.exe" if os.name == "nt" else "buildifier")
RULES = "bazel/buildifier/BUILD.bazel"
PINS = "bazel/prebuilt_buildtools.json"


class TestBuildifier(GitFixture):
    def setUp(self):
        super().setUp()
        self.stage(RULES, (REPOSITORY / RULES).read_bytes())
        self.stage(PINS, (REPOSITORY / PINS).read_bytes())
        self.git("commit", "-qm", "policy")

    def install(self):
        if not (REPOSITORY / BUILDIFIER).is_file():
            self.skipTest("requires the CI Buildifier binary from dda inv install-tools --buildifier")
        BUILDIFIER.parent.mkdir()
        shutil.copyfile(REPOSITORY / BUILDIFIER, BUILDIFIER)
        BUILDIFIER.chmod(0o755)

    def test_ci_exclusions_and_missing_binary(self):
        self.stage("deps/mylib/overlay/BUILD", b"broken(&\n")
        self.stage("vendor/a/WORKSPACE", b"broken(&\n")
        self.stage(".hidden/a.sky", b"broken(&\n")
        pre_commit.check_buildifier(self.ctx)
        self.stage("owned/BUILD.oss", b"broken(&\n")
        with self.assertRaisesRegex(Exit, "install-tools"):
            pre_commit.check_buildifier(self.ctx)

    def test_native_check_reads_staged_file_and_warning_policy(self):
        self.install()
        path = "owned/defs.bzl"
        self.stage(".buildifier.json", b'{"warnings":"-module-docstring"}\n')
        self.stage(path, b"X = 1\n")
        Path(".buildifier.json").write_bytes(b'{"warnings":"+module-docstring"}\n')
        self.assert_input_preserved(pre_commit.check_buildifier, path, fails=False)
        self.git("commit", "-qm", "valid")
        self.git("rm", "-q", "-f", ".buildifier.json")
        Path(".buildifier.json").write_bytes(b'{"warnings":"-module-docstring"}\n')
        self.assert_input_preserved(pre_commit.check_buildifier, path)
        self.stage(".buildifier.json", b'{"warnings":"-module-docstring"}\n')
        self.stage(path, b"X    =   1\n")
        Path(path).write_bytes(b"X = 1\n")
        self.assert_input_preserved(pre_commit.check_buildifier, path)

    def test_wrong_native_version_fails_before_validation(self):
        self.install()
        self.stage(PINS, json.dumps({"buildifier": {"version": "v0.0.0"}}).encode())
        with self.assertRaisesRegex(Exit, "Buildifier 0.0.0"):
            pre_commit.check_buildifier(self.ctx)


class TestRustfmt(GitFixture):
    def setUp(self):
        super().setUp()
        self.stage("MODULE.bazel", (REPOSITORY / "MODULE.bazel").read_bytes())
        self.git("commit", "-qm", "toolchain")
        import re

        toolchain = re.search(rb"(?ms)^toolchains\.toolchain\((.*?)^\)", (REPOSITORY / "MODULE.bazel").read_bytes())[1]
        version = re.search(rb'version\s*=\s*"([^"]+)"', toolchain)[1].decode()
        if (
            not shutil.which("rustup")
            or subprocess.run(["rustup", "run", version, "rustfmt", "--version"], capture_output=True).returncode
        ):
            self.skipTest(f"requires rustfmt in the {version} Rust toolchain")

    def test_child_modules_are_not_traversed_and_files_remain_untouched(self):
        self.stage("src/child.rs", b"fn  unformatted() {}\n")
        self.git("commit", "-qm", "child")
        path = "src/lib.rs"
        self.stage(path, b"mod child;\n")
        self.assert_input_preserved(pre_commit.check_rustfmt, path, fails=False)
        self.stage(path, b"mod  child;\n")
        Path(path).write_bytes(b"mod child;\n")
        self.assert_input_preserved(pre_commit.check_rustfmt, path)

    def test_config_deletion_and_edition_match_ci(self):
        self.stage("Cargo.toml", b"")
        self.stage("rustfmt.toml", b"hard_tabs = true\n")
        path = "src/lib.rs"
        self.stage(path, b"async fn f() {\n    return;\n}\n")
        self.assert_input_preserved(pre_commit.check_rustfmt, path, fails=False)
        self.stage(path, b"fn  needs_formatting() {}\n")
        self.git("commit", "-qm", "unformatted file")
        self.git("rm", "-q", "Cargo.toml")
        self.assert_input_preserved(pre_commit.check_rustfmt, path)

    def test_native_file_mode_honors_bom_and_file_level_skip(self):
        path = "src/lib.rs"
        self.stage(path, b"\xef\xbb\xbf#![rustfmt::skip]\nfn  skipped () {}\n")
        self.assert_input_preserved(pre_commit.check_rustfmt, path, fails=False)
