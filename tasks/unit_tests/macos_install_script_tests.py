import getpass
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
INVALID_LOGS_ENABLED = ["TRUE", "1", "yes", "false\nDD_LOGS_ENABLED=true", "$(touch injected)", "true/; touch injected"]
POSTINST = REPO_ROOT / "omnibus/package-scripts/agent-dmg/postinst"
SHIPPED_SYSPROBE_ENTRY = (
    REPO_ROOT / "pkg/fleet/installer/packages/embedded/tmpl/gen/darwin-pm/processes.d/datadog-agent-sysprobe.yaml"
)
PROCMGR_LABEL = "com.datadoghq.procmgr"

# Stand-ins for the real path_is_trusted, which needs BSD stat and ls -e. Sliced in after the real
# helpers, so they replace it.
STUB_TRUSTED = 'path_is_trusted() { return "$STUB_TRUSTED"; }\n'
STUB_SCAN = 'path_is_trusted() { printf "%s\\n" "$1" >> "$SCAN_LOG"; return 0; }\n'

# Records every launchctl call. bootout exits BOOTOUT_RC, and print reports the label as loaded
# when PRINT_LOADED is 1.
FAKE_LAUNCHCTL = """#!/bin/sh
echo "$*" >> "$LAUNCHCTL_LOG"
case "$1" in
    bootout) exit "${BOOTOUT_RC:-0}" ;;
    print) [ "${PRINT_LOADED:-0}" = 1 ] && exit 0; exit 113 ;;
esac
exit 0
"""


def postinst_function(script, name):
    """Returns the definition of the shell function name, from its opening line to its closing brace."""
    start = script.index(f"\n{name}() {{\n") + 1
    end = script.index("\n}\n", start) + len("\n}\n")
    return script[start:end]


def postinst_final_block(script):
    """Returns the end of the script, from the check that turns a failed omit into a failed install."""
    return script[script.index('if [ "$pit_unsafe_procmgr" -eq 1 ]; then\n    echo "ERROR: com.datadoghq.procmgr') :]


def run_sh(script, env, cwd):
    return subprocess.run(["/bin/sh", "-c", script], cwd=cwd, env=env, capture_output=True, text=True)


@unittest.skipIf(sys.platform == "win32", "macOS install scripts require a POSIX shell")
class TestMacOSInstallScript(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.env_file = self.root / "staging/env"
        self.config = self.root / "datadog.yaml"
        installer = (REPO_ROOT / "cmd/agent/macos/install_mac_os.sh").read_text()
        postinst = (REPO_ROOT / "omnibus/package-scripts/agent-dmg/postinst").read_text()
        # Execute only the real input/staging/config blocks, never package installation or service setup.
        self.stage = installer[installer.index("gui_app_menu_enabled=false") : installer.index("# Optional: Chrome")]
        self.stage += installer[
            installer.index("# Write configuration for the pkg") : installer.index("# Obtain the DMG")
        ]
        self.configure = postinst[
            postinst.index("# Read configuration written") : postinst.index("install_info_content=")
        ]
        if sys.platform != "darwin":
            # GNU sed takes no separate empty argument for an in-place edit without backup.
            self.configure = self.configure.replace("sed -i ''", "sed -i")

    def stage_logs_enabled(self, value):
        env = {
            "PATH": "/usr/bin:/bin",
            "install_staging_dir": str(self.env_file.parent),
            "install_env_file": str(self.env_file),
        }
        if value is not None:
            env["DD_LOGS_ENABLED"] = value
        return subprocess.run(["/bin/bash", "-ec", self.stage], cwd=self.root, env=env, capture_output=True, text=True)

    def configure_logs_enabled(self):
        result = subprocess.run(
            ["/bin/sh", "-c", self.configure],
            cwd=self.root,
            env={"PATH": "/usr/bin:/bin", "CONF_DIR": str(self.root), "INSTALL_ENV_FILE": str(self.env_file)},
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stderr, "")

    def test_logs_enabled_forwarding_and_config(self):
        for value in ("true", "false", "", None):
            for existing in ("# logs_enabled: false\n", "logs_enabled: false\n", "logs_enabled: true\n", ""):
                with self.subTest(value=value, existing=existing):
                    original = "site: datadoghq.com\n" + existing
                    self.config.write_text(original)
                    result = self.stage_logs_enabled(value)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    forwarded = [
                        line for line in self.env_file.read_text().splitlines() if line.startswith("DD_LOGS_ENABLED=")
                    ]
                    self.assertEqual(forwarded, [f"DD_LOGS_ENABLED={value}"] if value else [])
                    self.configure_logs_enabled()
                    expected = original
                    if value:
                        expected = "site: datadoghq.com\n" + ("" if existing else "\n") + f"logs_enabled: {value}\n"
                    self.assertEqual(self.config.read_text(), expected)

    def test_logs_enabled_rejects_invalid_install_input(self):
        for value in INVALID_LOGS_ENABLED:
            with self.subTest(value=value):
                result = self.stage_logs_enabled(value)
                self.assertEqual(result.returncode, 1)
                self.assertIn("DD_LOGS_ENABLED must be true or false", result.stdout)
                self.assertFalse(self.env_file.exists())
                self.assertFalse((self.root / "injected").exists())

    def test_logs_enabled_ignores_invalid_staged_input(self):
        self.env_file.parent.mkdir()
        for value in INVALID_LOGS_ENABLED:
            with self.subTest(value=value):
                original = "logs_enabled: true\n"
                self.config.write_text(original)
                self.env_file.write_text(f"DD_LOGS_ENABLED={value}\n")
                self.configure_logs_enabled()
                self.assertEqual(self.config.read_text(), original)
                self.assertFalse((self.root / "injected").exists())

    def test_logs_enabled_preserves_config_without_staging_file(self):
        original = "logs_enabled: true\n"
        self.config.write_text(original)
        self.configure_logs_enabled()
        self.assertEqual(self.config.read_text(), original)


def make_dir(path, mode=0o755):
    path.mkdir(parents=True)
    os.chmod(path, mode)
    return path


def make_file(path, content="", mode=0o644):
    path.write_text(content)
    os.chmod(path, mode)
    return path


def remove_tree(root):
    """Gives every directory back its write bit, so a test that made one read-only can still be cleaned up."""
    for dirpath, dirnames, _ in os.walk(root):
        for name in dirnames:
            path = os.path.join(dirpath, name)
            if not os.path.islink(path):
                os.chmod(path, 0o755)
    os.chmod(root, 0o755)
    shutil.rmtree(root, ignore_errors=True)


@unittest.skipIf(sys.platform == "win32", "macOS install scripts require a POSIX shell")
class TestMacOSInstallScriptPOSIX(unittest.TestCase):
    """The fallback's control flow, with a stub path_is_trusted, so it runs without BSD tools."""

    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(remove_tree, self.root)
        self.postinst = POSTINST.read_text()
        self.bin = make_dir(self.root / "bin")
        make_file(self.bin / "launchctl", FAKE_LAUNCHCTL, 0o755)
        make_file(self.bin / "sleep", "#!/bin/sh\nexit 0\n", 0o755)
        make_file(self.bin / "chown", "#!/bin/sh\nexit 0\n", 0o755)
        self.launchctl_log = self.root / "launchctl.log"
        self.launchd_dir = make_dir(self.root / "LaunchDaemons")
        self.install_dir = make_dir(self.root / "opt")
        self.jobs_dir = make_dir(self.root / "jobs")
        make_file(self.jobs_dir / f"{PROCMGR_LABEL}.plist", "<plist/>\n")

    def env(self, **extra):
        env = {
            "PATH": f"{self.bin}:/usr/bin:/bin",
            "LAUNCHCTL_LOG": str(self.launchctl_log),
            "INSTALL_DIR": str(self.install_dir),
            "PIT_ROOT_UID": "0",
            "PIT_LAUNCHD_DIR": str(self.launchd_dir),
        }
        env.update(extra)
        return env

    def helpers(self, stub):
        return (
            postinst_function(self.postinst, "path_is_trusted")
            + postinst_function(self.postinst, "pit_note_untrusted_catalog")
            + postinst_function(self.postinst, "pit_omit_procmgr")
            + stub
        )

    def launchctl_calls(self):
        if not self.launchctl_log.exists():
            return []
        return self.launchctl_log.read_text().splitlines()

    def skip_if_root(self):
        if os.geteuid() == 0:
            self.skipTest("root ignores the directory mode")

    def run_omit(self, **env):
        make_file(self.launchd_dir / f"{PROCMGR_LABEL}.plist", "<plist/>\n")
        return run_sh(self.helpers(STUB_TRUSTED) + "pit_omit_procmgr\n", self.env(STUB_TRUSTED="0", **env), self.root)

    def test_omit_when_bootout_fails_and_the_job_is_gone(self):
        result = self.run_omit(BOOTOUT_RC="1", PRINT_LOADED="0")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((self.launchd_dir / f"{PROCMGR_LABEL}.plist").exists())
        self.assertIn(f"bootout system/{PROCMGR_LABEL}", self.launchctl_calls())

    def test_omit_fails_when_the_job_stays_loaded(self):
        result = self.run_omit(BOOTOUT_RC="0", PRINT_LOADED="1")
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertFalse((self.launchd_dir / f"{PROCMGR_LABEL}.plist").exists())
        self.assertIn("still loaded", result.stdout)

    def test_omit_when_bootout_succeeds(self):
        result = self.run_omit(BOOTOUT_RC="0", PRINT_LOADED="0")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((self.launchd_dir / f"{PROCMGR_LABEL}.plist").exists())

    def test_omit_fails_when_the_definition_cannot_be_removed(self):
        self.skip_if_root()
        make_file(self.launchd_dir / f"{PROCMGR_LABEL}.plist", "<plist/>\n")
        os.chmod(self.launchd_dir, 0o555)
        result = run_sh(
            self.helpers(STUB_TRUSTED) + "pit_omit_procmgr\n",
            self.env(STUB_TRUSTED="0", BOOTOUT_RC="0", PRINT_LOADED="0"),
            self.root,
        )
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)

    def test_final_block_fails_the_install_on_a_failed_omit(self):
        block = postinst_final_block(self.postinst)
        for value, expected in (("1", 1), ("0", 0)):
            with self.subTest(pit_unsafe_procmgr=value):
                result = run_sh(block, self.env(pit_unsafe_procmgr=value), self.root)
                self.assertEqual(result.returncode, expected, result.stdout + result.stderr)

    def run_fallback(self, **env):
        script = (
            self.helpers(STUB_TRUSTED)
            + postinst_function(self.postinst, "install_fallback_launchdaemons")
            + "pit_skip_procmgr=0\npit_unsafe_procmgr=0\n"
            + "install_fallback_launchdaemons\n"
            + 'echo "pit_unsafe_procmgr=$pit_unsafe_procmgr"\n'
        )
        return run_sh(
            script,
            self.env(FALLBACK_JOBS=PROCMGR_LABEL, FALLBACK_JOBS_DIR=str(self.jobs_dir), **env),
            self.root,
        )

    def bootstraps(self):
        return [call for call in self.launchctl_calls() if call.startswith("bootstrap")]

    def test_fallback_on_an_untrusted_catalog_with_procmgr_still_loaded(self):
        make_dir(self.install_dir / "processes.d", 0o777)
        result = self.run_fallback(STUB_TRUSTED="1", PRINT_LOADED="1")
        self.assertIn("pit_unsafe_procmgr=1", result.stdout)
        self.assertEqual(self.bootstraps(), [])

    def test_fallback_on_an_untrusted_catalog_does_not_reload_procmgr(self):
        # print never sees the label, so the "still loaded" check would not skip bootstrap.
        # The loop guard is what keeps procmgr from being loaded, including when this runs as root.
        make_dir(self.install_dir / "processes.d", 0o777)
        plist = make_file(self.launchd_dir / f"{PROCMGR_LABEL}.plist", "<plist/>\n")
        result = self.run_fallback(STUB_TRUSTED="1", PRINT_LOADED="0")
        self.assertIn("pit_unsafe_procmgr=0", result.stdout)
        self.assertEqual(self.bootstraps(), [])
        self.assertFalse(plist.exists())

    def test_fallback_on_an_untrusted_catalog_when_the_definition_cannot_be_removed(self):
        self.skip_if_root()
        make_dir(self.install_dir / "processes.d", 0o777)
        make_file(self.launchd_dir / f"{PROCMGR_LABEL}.plist", "<plist/>\n")
        os.chmod(self.launchd_dir, 0o555)
        result = self.run_fallback(STUB_TRUSTED="1", PRINT_LOADED="0")
        self.assertIn("pit_unsafe_procmgr=1", result.stdout)
        self.assertEqual(self.bootstraps(), [])

    def test_fallback_on_a_trusted_catalog_loads_procmgr(self):
        make_dir(self.install_dir / "processes.d")
        result = self.run_fallback(STUB_TRUSTED="0", PRINT_LOADED="0")
        self.assertIn("pit_unsafe_procmgr=0", result.stdout)
        self.assertEqual(self.bootstraps(), [f"bootstrap system {self.launchd_dir}/{PROCMGR_LABEL}.plist"])

    def test_scan_judges_every_yaml_name_dotfiles_included(self):
        processes_d = make_dir(self.install_dir / "processes.d")
        make_file(processes_d / "datadog-agent-sysprobe.yaml", SHIPPED_SYSPROBE_ENTRY.read_text())
        for name in ("custom.yaml", ".custom.yaml", ".custom.yml", "..x.yaml", ".yaml", ".notes.txt"):
            make_file(processes_d / name, "command: /bin/true\n")
        make_dir(processes_d / ".d.yaml")
        scan_log = self.root / "scan.log"

        result = run_sh(
            self.helpers(STUB_SCAN) + 'pit_skip_procmgr=0\npit_note_untrusted_catalog\necho "$pit_skip_procmgr"\n',
            self.env(SCAN_LOG=str(scan_log)),
            self.root,
        )

        self.assertEqual(result.stdout.strip(), "0", result.stderr)
        judged = sorted(Path(line).name for line in scan_log.read_text().splitlines() if Path(line).parent == processes_d)
        self.assertEqual(
            judged,
            sorted(
                ["datadog-agent-sysprobe.yaml", "custom.yaml", ".custom.yaml", ".custom.yml", "..x.yaml", ".yaml", ".d.yaml"]
            ),
        )
        self.assertNotIn(".notes.txt", judged)

    def test_missing_install_root_is_untrusted(self):
        result = run_sh(
            self.helpers(STUB_TRUSTED) + 'pit_skip_procmgr=0\npit_note_untrusted_catalog\necho "$pit_skip_procmgr"\n',
            self.env(STUB_TRUSTED="0", INSTALL_DIR=str(self.root / "missing")),
            self.root,
        )
        self.assertEqual(result.stdout.strip(), "1", result.stderr)


@unittest.skipUnless(sys.platform == "darwin", "path_is_trusted needs BSD stat and ls -e")
class TestMacOSInstallScriptDarwin(unittest.TestCase):
    """The real path_is_trusted, on the real filesystem, with the test user standing in for root."""

    def setUp(self):
        # Never under /tmp: a host can carry an inheritable ACL there, and any ACL is untrusted.
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(remove_tree, self.root)
        subprocess.run(["/bin/chmod", "-N", str(self.root)], capture_output=True)
        acl = subprocess.run(["/bin/ls", "-lde", str(self.root)], capture_output=True, text=True, check=True)
        if len(acl.stdout.strip().splitlines()) > 1:
            self.fail(f"{self.root} still carries an ACL after chmod -N:\n{acl.stdout}")
        self.postinst = POSTINST.read_text()
        self.uid = str(os.getuid())
        self.user = getpass.getuser()

    def add_acl(self, path, perms):
        subprocess.run(["/bin/chmod", "+a", f"user:{self.user} allow {perms}", str(path)], check=True)

    def path_is_trusted(self, path, uid=None, kind=None):
        args = [str(path)]
        if uid is not None or kind is not None:
            args.append(uid if uid is not None else "0")
        if kind is not None:
            args.append(kind)
        script = postinst_function(self.postinst, "path_is_trusted") + 'path_is_trusted "$@"\n'
        result = subprocess.run(
            ["/bin/sh", "-c", script, "sh", *args],
            env={"PATH": "/usr/bin:/bin"},
            capture_output=True,
            text=True,
        )
        return result.returncode

    def test_path_is_trusted(self):
        user_dir = make_dir(self.root / "user")
        self.assertEqual(self.path_is_trusted(self.root / "missing"), 0)
        self.assertEqual(self.path_is_trusted("/"), 0)
        self.assertEqual(self.path_is_trusted(user_dir), 1, "owned by the test user, not root")
        self.assertEqual(self.path_is_trusted(user_dir, self.uid), 0)
        for mode in (0o775, 0o757):
            with self.subTest(mode=oct(mode)):
                os.chmod(user_dir, mode)
                self.assertEqual(self.path_is_trusted(user_dir, self.uid), 1)
        os.chmod(user_dir, 0o755)

        acl_dir = make_dir(self.root / "acl")
        self.add_acl(acl_dir, "add_file")
        self.assertEqual(self.path_is_trusted(acl_dir, self.uid), 1)

        link = self.root / "link"
        link.symlink_to("/")
        self.assertEqual(self.path_is_trusted(link), 1)

        regular = make_file(self.root / "regular")
        self.assertEqual(self.path_is_trusted(regular, self.uid, "dir"), 1)

    def note_untrusted_catalog(self, install_dir):
        script = (
            postinst_function(self.postinst, "path_is_trusted")
            + postinst_function(self.postinst, "pit_note_untrusted_catalog")
            + 'pit_skip_procmgr=0\npit_note_untrusted_catalog\necho "$pit_skip_procmgr"\n'
        )
        result = subprocess.run(
            ["/bin/sh", "-c", script],
            env={"PATH": "/usr/bin:/bin", "INSTALL_DIR": str(install_dir), "PIT_ROOT_UID": self.uid},
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.stderr, "")
        return result.stdout.strip()

    def install_tree(self, name, with_processes_d=True):
        install_dir = make_dir(self.root / name / "opt")
        if with_processes_d:
            processes_d = make_dir(install_dir / "processes.d")
            make_file(processes_d / "datadog-agent-sysprobe.yaml", SHIPPED_SYSPROBE_ENTRY.read_text())
        return install_dir

    def test_pit_note_untrusted_catalog(self):
        shipped = SHIPPED_SYSPROBE_ENTRY.read_text()

        def foreign(name, mode=0o644, acl=False):
            def plant(processes_d):
                path = make_file(processes_d / name, "command: /bin/true\n", mode)
                if acl:
                    self.add_acl(path, "write")

            return plant

        def auto_start_true(processes_d):
            make_file(processes_d / "datadog-agent-sysprobe.yaml", shipped.replace("auto_start: false", "auto_start: true"))

        cases = {
            "shipped entry": (None, "0"),
            "foreign custom.yaml at 0666": (foreign("custom.yaml", 0o666), "1"),
            "foreign custom.yml with an ACL": (foreign("custom.yml", acl=True), "1"),
            "hidden .custom.yaml at 0666": (foreign(".custom.yaml", 0o666), "1"),
            "hidden .custom.yml with an ACL": (foreign(".custom.yml", acl=True), "1"),
            "hidden .notes.txt at 0666": (foreign(".notes.txt", 0o666), "0"),
            "hidden ..x.yaml at 0666": (foreign("..x.yaml", 0o666), "1"),
            "shipped entry with auto_start true": (auto_start_true, "1"),
        }
        for index, (name, (plant, expected)) in enumerate(cases.items()):
            with self.subTest(name):
                install_dir = self.install_tree(f"case{index}")
                if plant is not None:
                    plant(install_dir / "processes.d")
                self.assertEqual(self.note_untrusted_catalog(install_dir), expected)

        with self.subTest("no processes.d"):
            self.assertEqual(self.note_untrusted_catalog(self.install_tree("bare", with_processes_d=False)), "0")

        with self.subTest("processes.d as a regular file"):
            install_dir = self.install_tree("file", with_processes_d=False)
            make_file(install_dir / "processes.d", "not a directory\n")
            self.assertEqual(self.note_untrusted_catalog(install_dir), "1")

        with self.subTest("processes.d as a symlink to a trusted directory"):
            install_dir = self.install_tree("link", with_processes_d=False)
            target = make_dir(self.root / "link-target")
            (install_dir / "processes.d").symlink_to(target)
            self.assertEqual(self.path_is_trusted(target, self.uid, "dir"), 0)
            self.assertEqual(self.note_untrusted_catalog(install_dir), "1")
