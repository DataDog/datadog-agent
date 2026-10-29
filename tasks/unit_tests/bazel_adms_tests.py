import os
import shlex
import shutil
import stat
import subprocess
import tempfile
import time
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SELECT_SH = REPO_ROOT / "bazel/tools/adms-select.sh"
BASH = shutil.which("bash")

# (workspace rc, home rc, command-line options, expected injected config).
NAMED_CONFIG_CASES = (
    ("common --config=_local\ncommon:_local --downloader_config=\n", "", (), ""),
    ("common:_local --downloader_config=custom.cfg\ncommon --config _local\n", "", (), ""),
    ("common:_local --downloader_config=\n", "", ("--config=_local",), ""),
    ("common:_local --downloader_config=custom.cfg\n", "", ("--config", "_local"), ""),
    # Follow nested configs regardless of definition order and rc-file order.
    (
        "common:_mirror --downloader_config=\ncommon:_local --config=_mirror\ncommon --config=_local\n",
        "",
        (),
        "",
    ),
    ("common:_mirror --downloader_config=\n", "common --config=_local\ncommon:_local --config=_mirror\n", (), ""),
    ("common --config=_local\ncommon:_local --config=adms\n", "", (), ""),
    (
        "common:_local --config=_other\ncommon:_other --config=_local --downloader_config=\n",
        "",
        ("--config=_local",),
        "",
    ),
    # Cycles terminate, and inactive configs (including prefix matches) stay inactive.
    (
        "common:_local --config=_other\ncommon:_other --config=_local\ncommon:_unused --downloader_config=\n",
        "",
        ("--config=_local",),
        "--config=adms",
    ),
    ("common --config=_local_extra\ncommon:_local --downloader_config=\n", "", (), "--config=adms"),
    ("common:_local --downloader_config=\n", "", ("run", "//:tool", "--", "--config=_local"), "--config=adms"),
    ("common:_local --downloader_config=\n", "", ("--config=_other", "--config=_local"), ""),
)


@unittest.skipUnless(BASH and os.name != "nt", "requires a POSIX host with Bash")
class TestAdmsSelect(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory(prefix="bazel adms ")
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.bin_dir = self.root / "bin"
        self.bin_dir.mkdir()
        for name in ("id", "mkdir", "chmod", "stat", "date", "cat", "dirname"):
            (self.bin_dir / name).symlink_to(shutil.which(name))
        self.tmpdir = self.root / "tmp"
        self.tmpdir.mkdir()
        self.env = {
            "PATH": str(self.bin_dir),
            "HOME": str(self.root),
            "TMPDIR": str(self.tmpdir),
        }
        self.curl(0)

    def curl(self, status):
        stub = self.bin_dir / "curl"
        stub.write_text(f'#!/bin/sh\nprintf "%s\\n" "$@" >"$HOME/curl-args"\nexit {status}\n')
        stub.chmod(0o755)

    def run_select(self, policy=None, args=()):
        if policy is not None:
            self.env["DD_BAZEL_ADMS"] = policy
        script = f'set -euo pipefail; . {shlex.quote(str(SELECT_SH))}; _adms_repo_root="$HOME"; _adms_config "$@" || :'
        result = subprocess.run(
            [BASH, "-c", script, "adms-test", *args],
            capture_output=True,
            text=True,
            env=self.env,
            timeout=10,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        return result

    def seed_probe(self, value, age=0):
        probe = self.tmpdir / f"datadog-agent-{os.getuid()}" / "adms-probe"
        probe.parent.mkdir(parents=True, exist_ok=True)
        probe.write_text(value)
        os.utime(probe, (time.time() - age, time.time() - age))
        return probe

    def test_auto_is_default_and_probes_mirror(self):
        self.assertEqual(self.run_select().stdout.strip(), "--config=adms")
        curl_args = (self.root / "curl-args").read_text().splitlines()
        self.assertIn("--fail", curl_args)  # HTTP 502 must fail, even if TLS/connect succeeds.
        self.assertIn("--connect-timeout", curl_args)
        self.assertIn("--max-time", curl_args)
        self.assertIn("https://depot-read-api-bzl.us1.ddbuild.io/bcr.bazel.build/bazel_registry.json", curl_args)

    def test_on_forces_without_probe(self):
        self.curl(7)
        self.assertEqual(self.run_select("on").stdout.strip(), "--config=adms")
        self.assertFalse((self.root / "curl-args").exists())

    def test_off_skips_without_probe(self):
        self.assertEqual(self.run_select("off").stdout, "")
        self.assertFalse((self.root / "curl-args").exists())

    def test_auto_unreachable(self):
        for status in (7, 22, 28):  # connect failure, HTTP error, timeout
            with self.subTest(status=status):
                self.seed_probe("no", age=120)
                self.curl(status)
                self.assertEqual(self.run_select("auto").stdout, "")

    def test_missing_curl_skips(self):
        (self.bin_dir / "curl").unlink()
        self.assertEqual(self.run_select().stdout, "")

    def test_explicit_command_line_options_win(self):
        for args in (
            ("--config=adms",),
            ("--config", "adms"),
            ("--config=dd-internal",),
            ("--downloader_config=",),
            ("--downloader_config=custom.cfg",),
            ("--downloader_config", "custom.cfg"),
        ):
            with self.subTest(args=args):
                self.assertEqual(self.run_select("on", args).stdout, "")
        self.assertFalse((self.root / "curl-args").exists())

    def test_program_arguments_do_not_override_bazel(self):
        self.assertEqual(
            self.run_select("on", ("run", "//:tool", "--", "--downloader_config=")).stdout.strip(), "--config=adms"
        )

    def test_rc_downloader_settings_and_explicit_adms_win(self):
        for rc_name in ("user.bazelrc", ".bazelrc"):
            for option in (
                "--downloader_config=",
                "--downloader_config=custom.cfg",
                "--downloader_config custom.cfg",
                "--config=adms",
                "--config adms",
                "--config=dd-internal",
            ):
                for policy in ("auto", "on"):
                    with self.subTest(rc=rc_name, option=option, policy=policy):
                        rc = self.root / rc_name
                        rc.write_text(f"common {option}\n")
                        self.assertEqual(self.run_select(policy).stdout, "")
                        rc.unlink()
        self.assertFalse((self.root / "curl-args").exists())

    def test_rc_comments_and_inactive_configs_are_ignored(self):
        for line in (
            "# common --downloader_config=",
            "  # common --downloader_config=",
            "common --color=yes # --downloader_config=",
            "common:_custom --downloader_config=",
        ):
            with self.subTest(line=line):
                (self.root / "user.bazelrc").write_text(line + "\n")
                self.assertEqual(self.run_select("on").stdout.strip(), "--config=adms")

    def test_rc_optout_with_whitespace_and_no_final_newline(self):
        (self.root / "user.bazelrc").write_text(" \tcommon\t--downloader_config= # opt out")
        self.assertEqual(self.run_select("on").stdout, "")

    def test_named_configs(self):
        for user_rc, home_rc, args, expected in NAMED_CONFIG_CASES:
            for workspace, home in ((user_rc, home_rc), (home_rc, user_rc)):
                for policy in ("auto", "on"):
                    with self.subTest(workspace=workspace, home=home, args=args, policy=policy):
                        (self.root / "user.bazelrc").write_text(workspace)
                        (self.root / ".bazelrc").write_text(home)
                        (self.root / "curl-args").unlink(missing_ok=True)
                        self.assertEqual(self.run_select(policy, args).stdout.strip(), expected)
                        if not expected:
                            self.assertFalse((self.root / "curl-args").exists())

    def test_invalid_policy_reports_error_without_injecting(self):
        result = self.run_select("bogus")
        self.assertEqual(result.stdout, "")
        self.assertIn("expected auto|on|off", result.stderr)

    def test_recent_probe_results_are_reused(self):
        for cached, curl_status, expected in (("ok", 7, "--config=adms"), ("no", 0, "")):
            with self.subTest(cached=cached):
                self.seed_probe(cached)
                self.curl(curl_status)
                self.assertEqual(self.run_select().stdout.strip(), expected)
                self.assertFalse((self.root / "curl-args").exists())

    def test_expired_probe_results_are_retried(self):
        for cached, curl_status, expected in (("ok", 22, ""), ("no", 0, "--config=adms")):
            with self.subTest(cached=cached):
                probe = self.seed_probe(cached, age=120)
                self.curl(curl_status)
                self.assertEqual(self.run_select().stdout.strip(), expected)
                self.assertEqual(probe.read_text().strip(), "no" if curl_status else "ok")

    def test_unwritable_probe_cache_does_not_abort(self):
        (self.tmpdir / f"datadog-agent-{os.getuid()}").write_text("not a directory")
        self.assertEqual(self.run_select().stdout.strip(), "--config=adms")


@unittest.skipUnless(BASH and os.name != "nt", "requires a POSIX host with Bash")
class TestAdmsWrapper(unittest.TestCase):
    def test_local_and_ci_argument_injection(self):
        with tempfile.TemporaryDirectory(prefix="bazel wrapper ") as directory:
            root = Path(directory)
            (root / "tools").mkdir()
            (root / "bazel/tools").mkdir(parents=True)
            shutil.copy(REPO_ROOT / "tools/bazel", root / "tools/bazel")
            # Copy all selectors so cache and ADMS selection run together.
            for selector in (REPO_ROOT / "bazel/tools").glob("*-select.sh"):
                shutil.copy(selector, root / "bazel/tools" / selector.name)
            fake_bazel = root / "fake-bazel"
            fake_bazel.write_text('#!/bin/sh\nprintf "%s\\n" "$@"\n')
            fake_bazel.chmod(fake_bazel.stat().st_mode | stat.S_IEXEC)
            env = {
                "PATH": os.environ["PATH"],
                "HOME": str(root),
                "XDG_CACHE_HOME": str(root),
                "BAZEL_REAL": str(fake_bazel),
                "BAZELISK_SKIP_WRAPPER": "true",
            }
            for ci, policy, named_config in (
                (False, "on", False),
                (False, "off", False),
                (True, "on", False),
                (False, "on", True),
            ):
                with self.subTest(ci=ci, policy=policy, named_config=named_config):
                    (root / "user.bazelrc").write_text("common:_local --downloader_config=\n" if named_config else "")
                    user_args = ["--config=_local"] if named_config else []
                    run_env = env | {"DD_BAZEL_ADMS": policy, "DD_BAZEL_REMOTE_CACHE": policy}
                    if ci:
                        run_env |= {"CI": "true", "BUILDBARN_ID_TOKEN": "test-token"}
                    result = subprocess.run(
                        [
                            BASH,
                            str(root / "tools/bazel"),
                            "--output_base=/tmp/test base",
                            "build",
                            "//:target",
                            *user_args,
                        ],
                        capture_output=True,
                        text=True,
                        env=run_env,
                        timeout=10,
                        check=False,
                    )
                    self.assertEqual(result.returncode, 0, result.stderr)
                    configs = ["--config=ci", "--config=cache:frontend"] if ci else []
                    if not ci and policy == "on":
                        configs = ["--config=cache"] + ([] if named_config else ["--config=adms"])
                    self.assertEqual(
                        result.stdout.splitlines(),
                        ["--output_base=/tmp/test base", "build", f"--disk_cache={root}/bazel/disk-cache"]
                        + configs
                        + ["//:target", *user_args],
                    )


@unittest.skipUnless(os.name == "nt", "requires native Windows cmd.exe")
class TestAdmsWindowsSelect(unittest.TestCase):
    """Run the actual batch selection routines with a deterministic probe."""

    def run_select(self, policy, args=(), *, reachable=True, rc_name=None, rc_contents="", user_rc="", home_rc=""):
        with tempfile.TemporaryDirectory(prefix="bazel adms ") as directory:
            root = Path(directory)
            (root / "tools").mkdir()
            (root / "user.bazelrc").write_text(user_rc)
            (root / ".bazelrc").write_text(home_rc)
            if rc_name:
                (root / rc_name).write_text(rc_contents)
            routines = (REPO_ROOT / "tools/bazel.bat").read_text(encoding="utf-8")
            routines = routines[routines.index(":: ADMS download mirror auto-selection") :]
            routines = routines.split(":: ADMS reachability:", 1)[0]
            script = root / "tools/select.bat"
            script.write_text(
                '@echo off\nsetlocal EnableDelayedExpansion\nset "extra_args="\n'
                'set "_adms_options=%*"\ncall :adms_select\necho(!extra_args!\nexit /b 0\n'
                + routines
                + f"\n:adms_reachable\nexit /b {0 if reachable else 1}\n",
                encoding="utf-8",
            )
            result = subprocess.run(
                [os.environ["COMSPEC"], "/d", "/c", str(script), *args],
                capture_output=True,
                text=True,
                env=os.environ | {"DD_BAZEL_ADMS": policy, "USERPROFILE": str(root)},
                timeout=10,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            return result.stdout.strip()

    def test_policies(self):
        for policy, reachable, expected in (
            ("auto", True, "--config=adms"),
            ("auto", False, ""),
            ("on", False, "--config=adms"),
            ("off", True, ""),
        ):
            with self.subTest(policy=policy, reachable=reachable):
                self.assertEqual(self.run_select(policy, reachable=reachable), expected)

    def test_command_line_overrides(self):
        for args in (
            ("--config=adms",),
            ("--config", "adms"),
            ("--config=dd-internal",),
            ("--downloader_config=",),
            ("--downloader_config", "custom.cfg"),
        ):
            with self.subTest(args=args):
                self.assertEqual(self.run_select("on", args), "")
        self.assertEqual(self.run_select("on", ("run", "//:tool", "--", "--downloader_config=")), "--config=adms")

    def test_rc_overrides_and_comments(self):
        for rc_name in ("user.bazelrc", ".bazelrc"):
            for contents, expected in (
                ("common --downloader_config=\n", ""),
                ("common --downloader_config=custom.cfg\n", ""),
                ("common --downloader_config custom.cfg\n", ""),
                ("common --config=adms\n", ""),
                ("common --config adms\n", ""),
                ("common --config=dd-internal\n", ""),
                (" \tcommon\t--downloader_config= # opt out", ""),
                ("  # common --downloader_config=\n", "--config=adms"),
                ("common --color=yes # --downloader_config=\n", "--config=adms"),
                ("common # --downloader_config=\n", "--config=adms"),
                ("common:_custom --downloader_config=\n", "--config=adms"),
            ):
                with self.subTest(rc_name=rc_name, contents=contents):
                    self.assertEqual(self.run_select("on", rc_name=rc_name, rc_contents=contents), expected)

    def test_named_configs(self):
        for user_rc, home_rc, args, expected in NAMED_CONFIG_CASES:
            for workspace, home in ((user_rc, home_rc), (home_rc, user_rc)):
                for policy in ("auto", "on"):
                    with self.subTest(workspace=workspace, home=home, args=args, policy=policy):
                        self.assertEqual(self.run_select(policy, args, user_rc=workspace, home_rc=home), expected)


if __name__ == "__main__":
    unittest.main()
