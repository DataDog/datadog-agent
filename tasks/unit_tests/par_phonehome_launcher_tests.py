import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
MARKER = "DD_INTERNAL_PAR_LEGACY_INIT"


@unittest.skipUnless(os.name == "posix" and shutil.which("bash"), "requires a POSIX shell")
class TestPARPhoneHomeLaunchers(unittest.TestCase):
    def run_launcher(self, template, sysv=False):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            output = root / "marker"
            binary = root / "bin/agent/agent"
            binary.parent.mkdir(parents=True)
            capture = '#!/bin/sh\nprintf "%s" "${DD_INTERNAL_PAR_LEGACY_INIT-unset}" > "$PAR_TEST_OUTPUT"\n'
            binary.write_text(capture)
            binary.chmod(0o755)
            # Exercise exported environment across subprocesses without root or real services.
            ssd = root / "start-stop-daemon"
            ssd.write_text(
                '#!/bin/sh\nfor arg; do [ "$arg" != "--test" ] || exit 0; done\n' + capture.removeprefix("#!/bin/sh\n")
            )
            ssd.chmod(0o755)
            su = root / "su"
            su.write_text(
                '#!/bin/sh\n[ "$1" = "-s" ] && [ "$3" = "-c" ] || exit 2\n'
                'shell=$2; command=$4; shift 4\n'
                '[ "$1" = "dd-agent" ] && [ "$2" = "--" ] || exit 2\n'
                'shift 2\nexec "$shell" -c "$command" "$@"\n'
            )
            su.chmod(0o755)
            text = (ROOT / template).read_text().replace("{install_dir}", directory)
            if sysv:
                body = text.split("do_start()\n{", 1)[1].split("\n}", 1)[0]
                script = "do_start() {\n" + body + "\n}\ndo_start\n"
            else:
                script = text.split("\nscript\n", 1)[1].split("\nend script", 1)[0]
                script = script.replace("/var/log/datadog/errors.log", str(root / "errors.log"))
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.defpath, PAR_TEST_OUTPUT=str(output))
            env.pop(MARKER, None)
            result = subprocess.run(
                [shutil.which("bash") or "bash", "-c", script], env=env, capture_output=True, text=True, timeout=10
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(output.read_text(), "true")

    def test_sysv_core_marks_phonehome(self):
        self.run_launcher("packages/debian/etc/sysvinit_debian.in", sysv=True)

    def test_debian_upstart_core_marks_phonehome(self):
        self.run_launcher("packages/debian/etc/upstart_debian.conf.in")

    def test_redhat_upstart_core_marks_phonehome(self):
        self.run_launcher("packages/redhat/etc/upstart_redhat.conf.in")

    def test_systemd_core_does_not_mark_phonehome(self):
        template = ROOT / "pkg/fleet/installer/packages/embedded/tmpl/datadog-agent.service.tmpl"
        self.assertNotIn(MARKER, template.read_text())
