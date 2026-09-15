import re
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).parents[2]
OMNIBUS_TASK = REPO_ROOT / 'tasks' / 'omnibus.py'
FINALIZE_RECIPE = REPO_ROOT / 'omnibus' / 'config' / 'software' / 'datadog-agent-finalize.rb'
INSTALL_SCRIPT = REPO_ROOT / 'cmd' / 'agent' / 'macos' / 'install_mac_os.sh'


def _search(path: Path, pattern: str) -> str:
    match = re.search(pattern, path.read_text())
    assert match is not None, f'{pattern!r} no longer matches anything in {path.relative_to(REPO_ROOT)}'
    return match.group(1)


class TestMacOSFloorIsConsistent(unittest.TestCase):
    """
    The minimum macOS we support is written down in several places that have to agree.
    https://docs.datadoghq.com/agent/supported_platforms/?tab=macos

    Deliberately narrow. Most divergence here is already caught by the ABI gate in
    datadog-agent-finalize.rb, which fails the DMG build if any binary declares a minimum
    newer than the floor -- so the Swift systray target, for instance, needs no test. What
    the gate cannot catch is covered below.
    """

    def _build_target(self) -> str:
        return _search(OMNIBUS_TASK, r"MACOSX_DEPLOYMENT_TARGET'\] = '(\d+)\.")

    def test_installer_minimum_matches_build_target(self):
        # The gate structurally cannot catch this one. It asks "is every binary loadable on
        # the floor?", while the installer encodes what the floor *is* -- and the installer is
        # a shell script running on the user's Mac, not a binary the gate ever scans. Raise the
        # build target without raising this and the gate stays green while macOS users below
        # the new floor install an Agent that cannot start.
        installer_minimum = _search(INSTALL_SCRIPT, r'macos_major_version\}" -lt (\d+)')
        self.assertEqual(
            installer_minimum,
            self._build_target(),
            f'install_mac_os.sh admits macOS {installer_minimum}+ but binaries are built for '
            f'{self._build_target()}.0+. Nothing in the build catches this: the installer would '
            'let users below the build target install an Agent that cannot launch.',
        )

    def test_abi_gate_floor_matches_build_target(self):
        # Divergence here does fail the DMG build, but only once that job runs -- and a change
        # to tasks/omnibus.py alone does not trigger it, so the breakage would land on main and
        # surface in a later pipeline. Cheap to catch on the PR instead.
        gate_floor = _search(FINALIZE_RECIPE, r'MIN_ACCEPTABLE_VERSION: "(\d+)\.')
        self.assertEqual(
            gate_floor,
            self._build_target(),
            f'the macOS ABI gate accepts a {gate_floor}.0 floor but binaries are built for '
            f'{self._build_target()}.0+. Keep both in step, or every binary trips the gate.',
        )
