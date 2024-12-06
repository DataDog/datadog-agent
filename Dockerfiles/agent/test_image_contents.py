#!/opt/datadog-agent/embedded/bin/python

import grp
import os
import os.path
import pwd
import stat
import unittest
from hashlib import sha256

from six import iteritems

EXPECTED_PRESENT = [
    "/etc/datadog-agent/datadog-docker.yaml",
    "/etc/datadog-agent/datadog-kubernetes.yaml",
    "/etc/datadog-agent/datadog-ecs.yaml",
    "/etc/datadog-agent/datadog-ci.yaml",
    "/etc/datadog-agent/install_info",
]

EXPECTED_ABSENT = [
    # This will be symlinked by an entrypoint script if no set by user
    "/etc/datadog-agent/datadog.yaml",
]

EXPECTED_CHECKSUMS = {
    # See https://github.com/DataDog/datadog-agent/pull/1337
    # and https://github.com/DataDog/datadog-agent/pull/5362
    "/etc/s6/init/init-stage3": "ea2d995df5a28709b2a040d2a212ebaa1e351c8233cc26cd4803fdc6df52d2b3",
    "/etc/s6/init/init-stage3-host-pid": "710c5b63d7bf1d23897991cba43b8de68aef163e570a2a676db2d897bb09baf7",
}


class TestFiles(unittest.TestCase):
    def test_files_present(self):
        for file in EXPECTED_PRESENT:
            self.assertTrue(os.path.isfile(file), file + " should be present")

    def test_files_absent(self):
        for file in EXPECTED_ABSENT:
            self.assertFalse(os.path.isfile(file), file + " should NOT be present")

    def test_files_checksums(self):
        for file, digest in iteritems(EXPECTED_CHECKSUMS):
            sha = sha256()
            with open(file, "rb") as f:
                for chunk in iter(lambda: f.read(4096), b""):
                    sha.update(chunk)
            self.assertEqual(sha.hexdigest(), digest, file + " checksum mismatch")

    def test_files_permissions(self):
        for root, dirs, files in os.walk("/"):
            dirs[:] = filter(
                lambda dir: not os.path.ismount(os.path.join(root, dir)), dirs
            )

            for name in dirs + files:
                f = os.path.join(root, name)

                try:
                    s = os.stat(f)
                except FileNotFoundError:
                    pass
                except Exception as e:
                    self.fail(f"Failed to stat {f}: {e}")
                self.assertFalse(
                    s.st_mode & (stat.S_IWOTH | stat.S_ISVTX) == stat.S_IWOTH,
                    f"{f} should not be world-writable",
                )

                try:
                    pwd.getpwuid(s.st_uid)
                except KeyError:
                    self.fail(f"Unknown user {s.st_uid} for {f}")

                try:
                    grp.getgrgid(s.st_gid)
                except KeyError:
                    self.fail(f"Unknown group {s.st_gid} for {f}")


if __name__ == "__main__":
    unittest.main()
