import re
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]


class TestRcX509Pin(unittest.TestCase):
    def test_go_module_and_source_archive_pin_the_same_commit(self):
        # Go builds use the libdd-rc Go module from go.mod, while Bazel builds the
        # same code from a source archive. Both must point at one commit.
        go_mod = (REPO_ROOT / "go.mod").read_text()
        module = (REPO_ROOT / "deps/libdd_rc/libdd_rc.MODULE.bazel").read_text()
        overlay = (REPO_ROOT / "deps/libdd_rc/libdd_rc.BUILD.bazel").read_text()
        cargo = (REPO_ROOT / "deps/libdd_rc/Cargo.toml").read_text()

        go_version = re.search(r"github\.com/DataDog/libdd-rc/ffi-hosts/go (\S+)", go_mod)
        self.assertIsNotNone(go_version, "libdd-rc Go module not found in go.mod")
        pseudo_version = re.fullmatch(r"v0\.0\.0-\d{14}-([0-9a-f]{12})", go_version.group(1))
        self.assertIsNotNone(
            pseudo_version,
            f"expected a pseudo-version for the libdd-rc Go module, got {go_version.group(1)}; update this test",
        )
        go_commit = pseudo_version.group(1)

        archive_commits = set(re.findall(r"libdd-rc/archive/([0-9a-f]{40})\.tar\.gz", module))
        archive_commits |= set(re.findall(r'strip_prefix = "libdd-rc-([0-9a-f]{40})"', module))
        archive_commits |= set(re.findall(r'package = "libdd-rc",\s*version = "([0-9a-f]{40})"', overlay))
        # The crate set git-pins the libdd-rc crates until they are published with the
        # fips/non-fips feature pass-through.
        archive_commits |= set(re.findall(r'libdd-rc", rev = "([0-9a-f]{40})"', cargo))
        self.assertEqual(len(archive_commits), 1, f"libdd-rc source pins disagree: {sorted(archive_commits)}")

        archive_commit = archive_commits.pop()
        self.assertTrue(
            archive_commit.startswith(go_commit),
            f"go.mod pins libdd-rc {go_commit}, but the Bazel source archive pins {archive_commit}",
        )
