#!/opt/datadog-agent/embedded/bin/python

import os
import os.path
import unittest
from hashlib import sha256

EXPECTED_PRESENT = [
    "/etc/datadog-agent/datadog-docker.yaml",
    "/etc/datadog-agent/datadog-kubernetes.yaml",
    "/etc/datadog-agent/datadog-k8s-docker.yaml",
    "/etc/datadog-agent/datadog-ecs.yaml",
]

EXPECTED_ABSENT = [
    # This will be symlinked by an entrypoint script if no set by user
    "/etc/datadog-agent/datadog.yaml",
]

EXPECTED_CHECKSUMS = {
    # See https://github.com/DataDog/datadog-agent/pull/1337
    "/etc/s6/init/init-stage3": "710c5b63d7bf1d23897991cba43b8de68aef163e570a2a676db2d897bb09baf7",
}


class TestFiles(unittest.TestCase):

    def test_files_present(self):
        for file in EXPECTED_PRESENT:
            self.assertTrue(os.path.isfile(file), file + " should be present")

    def test_files_absent(self):
        for file in EXPECTED_ABSENT:
            self.assertFalse(os.path.isfile(file), file + " should NOT be present")

    def test_files_checksums(self):
        for file, digest in EXPECTED_CHECKSUMS.items():
            sha = sha256()
            with open(file, 'rb') as f:
                for chunk in iter(lambda: f.read(4096), b''):
                    sha.update(chunk)
            self.assertEqual(sha.hexdigest(), digest, file + " checksum mismatch")


if __name__ == '__main__':
    unittest.main()
