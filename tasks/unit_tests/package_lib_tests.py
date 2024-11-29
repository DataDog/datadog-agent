import json
import unittest
from unittest.mock import MagicMock, patch

from invoke import Exit, MockContext, Result

from tasks.libs.package.size import (
    PACKAGE_SIZE_TEMPLATE,
    SCANNED_BINARIES,
    _get_uncompressed_size,
    compare,
    compute_package_size_metrics,
    get_previous_size,
)
from tasks.libs.package.utils import list_packages


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


class TestListPackages(unittest.TestCase):
    def test_no_package(self):
        template = {}
        self.assertEqual(list_packages(template), [])

    def test_single_package(self):
        template = {"key": "value"}
        self.assertEqual(list_packages(template), [["key", "value"]])

    def test_multiple_packages(self):
        template = {"key": {"key2": 42}}
        self.assertEqual(list_packages(template), [["key", "key2", 42]])

    def test_ignore_timestamp_root(self):
        template = {"key": {"key2": 42}, "timestamp": 1234567890}
        self.assertEqual(list_packages(template), [["key", "key2", 42]])

    def test_ignore_timestamp_nested(self):
        template = {"key": {"key2": 42, "timestamp": 1234567890}}
        self.assertEqual(list_packages(template), [["key", "key2", 42]])


class TestGetPreviousSize(unittest.TestCase):
    package_sizes = {}

    def setUp(self) -> None:
        with open('tasks/unit_tests/testdata/package_sizes.json') as f:
            self.package_sizes = json.load(f)

    def test_is_ancestor(self):
        self.assertEqual(get_previous_size(self.package_sizes, "grand_ma", "artdeco", "cherry", 'fibula'), 42)

    def test_is_other_ancestor(self):
        self.assertEqual(get_previous_size(self.package_sizes, "pa", "artdeco", "cherry", 'fibula'), 3)

    def test_is_not_ancestor(self):
        self.assertEqual(get_previous_size(self.package_sizes, "grandPa", "artdeco", "cherry", 'fibula'), 42)


class TestGetUncompressedSize(unittest.TestCase):
    def test_get_deb_uncompressed_size(self):
        flavor = 'datadog-agent.deb'
        c = MockContext(run={f"dpkg-deb --info {flavor} | grep Installed-Size | cut -d : -f 2 | xargs": Result(42)})
        self.assertEqual(_get_uncompressed_size(c, flavor, 'deb'), 43008)

    def test_get_rpm_uncompressed_size(self):
        flavor = 'datadog-agent.rpm'
        c = MockContext(run={f"rpm -qip {flavor} | grep Size | cut -d : -f 2 | xargs": Result(42)})
        self.assertEqual(_get_uncompressed_size(c, flavor, 'rpm'), 42)

    def test_get_suse_uncompressed_size(self):
        flavor = 'datadog-agent.rpm'
        c = MockContext(run={f"rpm -qip {flavor} | grep Size | cut -d : -f 2 | xargs": Result(69)})
        self.assertEqual(_get_uncompressed_size(c, flavor, 'suse'), 69)


class TestCompare(unittest.TestCase):
    package_sizes = {}

    def setUp(self) -> None:
        with open('tasks/unit_tests/testdata/package_sizes.json') as f:
            self.package_sizes = json.load(f)

    @patch.dict('os.environ', {'OMNIBUS_PACKAGE_DIR': 'datadog-agent', 'OMNIBUS_PACKAGE_DIR_SUSE': 'datadog-agent'})
    @patch('tasks.libs.package.size.get_package_path', new=MagicMock(return_value='datadog-agent'))
    @patch('builtins.print')
    def test_on_main(self, mock_print):
        flavor = 'datadog-agent'
        c = MockContext(
            run={
                'git rev-parse --abbrev-ref HEAD': Result('main'),
                'git merge-base main main': Result('12345'),
                f"dpkg-deb --info {flavor} | grep Installed-Size | cut -d : -f 2 | xargs": Result(42),
                f"rpm -qip {flavor} | grep Size | cut -d : -f 2 | xargs": Result(69),
            }
        )
        self.package_sizes['12345'] = PACKAGE_SIZE_TEMPLATE
        self.assertEqual(self.package_sizes['12345']['amd64']['datadog-agent']['deb'], 140000000)
        compare(c, self.package_sizes, 'amd64', flavor, 'deb', 2001)
        self.assertEqual(self.package_sizes['12345']['amd64']['datadog-agent']['deb'], 43008)
        mock_print.assert_not_called()

    @patch.dict('os.environ', {'OMNIBUS_PACKAGE_DIR': 'datadog-agent', 'OMNIBUS_PACKAGE_DIR_SUSE': 'datadog-agent'})
    @patch('tasks.libs.package.size.get_package_path', new=MagicMock(return_value='datadog-heroku-agent'))
    @patch('builtins.print')
    def test_on_branch_ok(self, mock_print):
        flavor, arch, os_name = 'datadog-heroku-agent', 'arm64', 'suse'
        c = MockContext(
            run={
                'git rev-parse --abbrev-ref HEAD': Result('pikachu'),
                'git merge-base pikachu main': Result('25'),
                f"dpkg-deb --info {flavor} | grep Installed-Size | cut -d : -f 2 | xargs": Result(42),
                f"rpm -qip {flavor} | grep Size | cut -d : -f 2 | xargs": Result(69000000),
            }
        )
        compare(c, self.package_sizes, arch, flavor, os_name, 70000000)
        mock_print.assert_called_with(f"""{flavor}-{arch}-{os_name} size increase is OK:
  New package size is 69.00MB
  Ancestor package (25) size is 68.00MB
  Diff is 1.00MB (max allowed diff: 70.00MB)""")

    @patch.dict('os.environ', {'OMNIBUS_PACKAGE_DIR': 'datadog-agent', 'OMNIBUS_PACKAGE_DIR_SUSE': 'datadog-agent'})
    @patch('tasks.libs.package.size.get_package_path', new=MagicMock(return_value='datadog-heroku-agent'))
    @patch('builtins.print')
    def test_on_branch_ko(self, mock_print):
        flavor = 'datadog-heroku-agent'
        c = MockContext(
            run={
                'git rev-parse --abbrev-ref HEAD': Result('pikachu'),
                'git merge-base pikachu main': Result('25'),
                f"dpkg-deb --info {flavor} | grep Installed-Size | cut -d : -f 2 | xargs": Result(42),
                f"rpm -qip {flavor} | grep Size | cut -d : -f 2 | xargs": Result(139000000),
            }
        )
        with self.assertRaises(Exit):
            compare(c, self.package_sizes, 'arm64', flavor, 'suse', 70000000)
            mock_print.assert_called_with("""suse size increase is too large:
  New package size is 139.00MB
  Ancestor package (25) size is 68.00MB
  Diff is 71.00MB (max allowed diff: 70.00MB)""")
