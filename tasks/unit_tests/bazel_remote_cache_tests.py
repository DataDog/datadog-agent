import os
import shutil
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).parent.parent.parent
SELECT_SH = REPO_ROOT / "bazel" / "tools" / "remote-cache-select.sh"
BASH = shutil.which("bash") or "/bin/bash"
_COREUTILS = ("id", "mkdir", "chmod", "stat", "date", "cat")


def _make_stub(directory: Path, name: str, exit_code: int) -> None:
    """Create an executable stub command that exits with exit_code."""
    path = directory / name
    path.write_text(f"#!/bin/sh\nexit {exit_code}\n")
    path.chmod(path.stat().st_mode | stat.S_IEXEC | stat.S_IXGRP | stat.S_IXOTH)


def _link_coreutils(bin_dir: Path, skip: tuple[str, ...] = ()) -> None:
    """Symlink the coreutils the script needs into bin_dir (except `skip`)."""
    for name in _COREUTILS:
        if name in skip:
            continue
        real = shutil.which(name)
        if real:
            (bin_dir / name).symlink_to(real)


class TestRemoteCacheSelect(unittest.TestCase):
    """Exercise bazel/tools/remote-cache-select.sh with stubbed curl/vault.

    We source the script in a subshell and print _remote_cache_config's output,
    controlling reachability (stub curl) and token source (stub vault) via a
    fully isolated PATH. Container branches are not covered here since
    /.dockerenv cannot be faked in the test host.
    """

    def _probe_path(self, tmpdir: Path) -> Path:
        return tmpdir / f"datadog-agent-{os.getuid()}" / "remote-cache-probe"

    def _run(self, policy=None, args="", *, curl=0, vault=True, probe_seed=None, extra_env=None):
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            bin_dir = tmp / "bin"
            bin_dir.mkdir()
            _link_coreutils(bin_dir)
            _make_stub(bin_dir, "curl", curl)
            if vault:
                _make_stub(bin_dir, "vault", 0)

            tmpdir = tmp / "tmp"
            tmpdir.mkdir()
            if probe_seed is not None:
                probe = self._probe_path(tmpdir)
                probe.parent.mkdir(parents=True)
                probe.write_text(probe_seed)

            # PATH holds only our stubs + symlinked coreutils, so `command -v
            # vault` reflects the test intent rather than the host toolchain.
            env = {
                "PATH": str(bin_dir),
                "HOME": str(tmp),
                "TMPDIR": str(tmpdir),  # isolate the probe cache
            }
            if policy is not None:
                env["DD_BAZEL_REMOTE_CACHE"] = policy
            if extra_env:
                env.update(extra_env)

            script = f'. "{SELECT_SH}"; _remote_cache_config {args}'
            res = subprocess.run(
                [BASH, "-c", script],
                capture_output=True,
                text=True,
                env=env,
                check=False,
            )
            return res.stdout.strip()

    def test_off_never_enables(self):
        self.assertEqual(self._run(policy="off", curl=0), "")

    def test_on_always_enables(self):
        # Even with curl failing, `on` forces the config.
        self.assertEqual(self._run(policy="on", curl=7), "--config=cache")

    def test_auto_reachable_with_vault(self):
        self.assertEqual(self._run(policy="auto", curl=0, vault=True), "--config=cache")

    def test_auto_unreachable(self):
        self.assertEqual(self._run(policy="auto", curl=7, vault=True), "")

    def test_auto_no_token_source(self):
        # No vault CLI and no token: ineligible before probing.
        self.assertEqual(self._run(policy="auto", curl=0, vault=False), "")

    def test_auto_env_token_without_vault(self):
        # An injected BUILDBARN_ID_TOKEN makes the build eligible without vault.
        self.assertEqual(
            self._run(policy="auto", curl=0, vault=False, extra_env={"BUILDBARN_ID_TOKEN": "deadbeef"}),
            "--config=cache",
        )

    def test_default_policy_is_auto(self):
        self.assertEqual(self._run(policy=None, curl=0, vault=True), "--config=cache")

    def test_explicit_config_cache_wins(self):
        self.assertEqual(self._run(policy="on", args="--config=cache:frontend"), "")

    def test_explicit_no_remote_cache_wins(self):
        self.assertEqual(self._run(policy="on", args="--config=no-remote-cache"), "")

    def test_unknown_policy_prints_nothing(self):
        self.assertEqual(self._run(policy="bogus", curl=0), "")

    def test_positive_probe_is_sticky(self):
        # A cached "ok" wins even when the current probe would fail.
        self.assertEqual(
            self._run(policy="auto", curl=7, vault=True, probe_seed="ok"),
            "--config=cache",
        )

    def test_fresh_negative_probe_not_retried(self):
        # A cached "no" younger than 60s suppresses the cache even if curl now
        # succeeds.
        self.assertEqual(
            self._run(policy="auto", curl=0, vault=True, probe_seed="no"),
            "",
        )

    def test_busybox_stat_with_cached_probe(self):
        # Regression: BusyBox `stat -f %m` prints human output ("File: ...") to
        # stdout and exits 0. Feeding that into arithmetic under `set -u` used to
        # abort the wrapper. A cached "no" forces the mtime code path.
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            bin_dir = tmp / "bin"
            bin_dir.mkdir()
            _link_coreutils(bin_dir, skip=("stat",))
            _make_stub(bin_dir, "curl", 0)
            _make_stub(bin_dir, "vault", 0)
            (bin_dir / "stat").write_text(
                '#!/bin/sh\nif [ "$1" = "-c" ]; then echo 1700000000; exit 0; fi\necho "  File: \\"x\\""; exit 0\n'
            )
            (bin_dir / "stat").chmod(0o755)

            tmpdir = tmp / "tmp"
            probe = self._probe_path(tmpdir)
            probe.parent.mkdir(parents=True)
            probe.write_text("no")

            env = {
                "PATH": str(bin_dir),
                "HOME": str(tmp),
                "TMPDIR": str(tmpdir),
                "DD_BAZEL_REMOTE_CACHE": "auto",
            }
            res = subprocess.run(
                [BASH, "-c", f'set -euo pipefail; . "{SELECT_SH}"; _remote_cache_config'],
                capture_output=True,
                text=True,
                env=env,
                check=False,
            )
            self.assertNotIn("unbound variable", res.stderr)


if __name__ == "__main__":
    unittest.main()
