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
        archive_commits = set(re.findall(r"libdd-rc/archive/([0-9a-f]{40})\.tar\.gz", module))
        archive_commits |= set(re.findall(r'strip_prefix = "libdd-rc-([0-9a-f]{40})"', module))
        archive_commits |= set(re.findall(r'package = "libdd-rc",\s*version = "([0-9a-f]{40})"', overlay))
        # The crate set git-pins the libdd-rc crates to the same commit.
        archive_commits |= set(re.findall(r'libdd-rc", rev = "([0-9a-f]{40})"', cargo))
        archive_commits |= set(re.findall(r'"BUILD_GIT_COMMIT_HASH": "([0-9a-f]{40})"', module))
        self.assertEqual(len(archive_commits), 1, f"libdd-rc source pins disagree: {sorted(archive_commits)}")

        archive_commit = archive_commits.pop()
        pseudo_version = re.fullmatch(r"v0\.0\.0-\d{14}-([0-9a-f]{12})", go_version.group(1))
        if pseudo_version:
            self.assertTrue(
                archive_commit.startswith(pseudo_version.group(1)),
                f"go.mod pins libdd-rc {pseudo_version.group(1)}, but the Bazel source archive pins {archive_commit}",
            )
            return

        # A release version names no commit, so the source archive pin records the Go
        # module tag that was cut on the same commit.
        tag = re.search(r"tagged\s*#\s*ffi-hosts/go/(v\S+) on the same commit", module)
        self.assertIsNotNone(tag, "libdd_rc.MODULE.bazel does not record the ffi-hosts/go tag for its pinned commit")
        self.assertEqual(
            go_version.group(1),
            tag.group(1),
            f"go.mod pins libdd-rc {go_version.group(1)}, but the Bazel source archive is the {tag.group(1)} commit",
        )
