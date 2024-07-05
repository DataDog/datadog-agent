import unittest
from unittest.mock import MagicMock, patch

from invoke import MockContext

from tasks.libs.package.size import SCANNED_BINARIES, compute_package_size_metrics


class TestProduceSizeStats(unittest.TestCase):
    @patch('tempfile.TemporaryDirectory', autospec=True)
    @patch('tasks.libs.package.size.extract_package', new=MagicMock())
    @patch('tasks.libs.package.size.file_size', new=MagicMock(return_value=20))
    @patch('tasks.libs.package.size.directory_size', new=MagicMock(return_value=250))
    def test_compute_size(self, _):
        context_mock = MockContext()
        test_flavor, test_os, test_path, test_version, test_ref, test_branch, test_arch = (
            "agent",
            "os",
            "/path/to/package",
            "version",
            "gitref",
            "branch",
            "arch",
        )
        series = compute_package_size_metrics(
            ctx=context_mock,
            flavor=test_flavor,
            package_os=test_os,
            package_path=test_path,
            major_version=test_version,
            git_ref=test_ref,
            bucket_branch=test_branch,
            arch=test_arch,
        )
        print(series)

        expected_tags = [
            "os:os",
            "package:datadog-agent",
            "agent:version",
            "git_ref:gitref",
            "bucket_branch:branch",
            "arch:arch",
        ]

        # Verify compressed package data
        compressed_package_series = [s for s in series if s["metric"] == "datadog.agent.compressed_package.size"]
        self.assertEqual(len(compressed_package_series), 1)
        s = compressed_package_series[0]
        self.assertListEqual(s["tags"], expected_tags)
        self.assertEqual(len(s["points"]), 1)
        self.assertEqual(s["points"][0]["value"], 20.0)

        # Verify uncompressed package data
        uncompressed_package_series = [s for s in series if s["metric"] == "datadog.agent.package.size"]
        self.assertEqual(len(uncompressed_package_series), 1)
        s = uncompressed_package_series[0]
        self.assertListEqual(s["tags"], expected_tags)
        self.assertEqual(len(s["points"]), 1)
        self.assertEqual(s["points"][0]["value"], 250.0)

        # Verify that each binary has data, and have their binary tag attached
        binary_package_series = [s for s in series if s["metric"] == "datadog.agent.binary.size"]
        self.assertEqual(len(binary_package_series), len(SCANNED_BINARIES[test_flavor]))

        binary_tags = []
        for s in binary_package_series:
            self.assertEqual(len(s["points"]), 1)
            self.assertEqual(s["points"][0]["value"], 20.0)
            binary_tags += [tag for tag in s["tags"] if "bin" in tag]

        expected_binary_tags = [f"bin:{binary}" for binary in SCANNED_BINARIES[test_flavor].keys()]
        self.assertListEqual(binary_tags, expected_binary_tags)

    def test_compute_size_invalid_flavor(self):
        context_mock = MockContext()
        test_flavor, test_os, test_path, test_version, test_ref, test_branch, test_arch = (
            "invalid",
            "os",
            "/path/to/package",
            "version",
            "gitref",
            "branch",
            "arch",
        )
        with self.assertRaisesRegex(ValueError, "is not part of the accepted flavors"):
            compute_package_size_metrics(
                ctx=context_mock,
                flavor=test_flavor,
                package_os=test_os,
                package_path=test_path,
                major_version=test_version,
                git_ref=test_ref,
                bucket_branch=test_branch,
                arch=test_arch,
            )
