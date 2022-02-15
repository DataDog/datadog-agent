import hashlib
import unittest
from typing import OrderedDict
from unittest import mock

from invoke.exceptions import Exit

from . import release
from .libs.version import Version


def mocked_github_requests_get(*args, **_kwargs):
    class MockResponse:
        def __init__(self, json_data, status_code):
            self.json_data = json_data
            self.status_code = status_code

        def json(self):
            return self.json_data

    if args[0][-1] == "6":
        return MockResponse(
            [
                {"ref": "6.28.0-rc.1"},
                {"ref": "6.28.0"},
                {"ref": "6.28.1-rc.1"},
                {"ref": "6.28.1"},
                {"ref": "6.29.0-rc.1"},
                {"ref": "6.29.0"},
            ],
            200,
        )

    if args[0][-1] == "7":
        return MockResponse(
            [
                {"ref": "7.28.0-rc.1"},
                {"ref": "7.28.0"},
                {"ref": "7.28.1-rc.1"},
                {"ref": "7.28.1"},
                {"ref": "7.29.0-rc.1"},
                {"ref": "7.29.0"},
            ],
            200,
        )

    return MockResponse(
        [
            {"ref": "6.28.0-rc.1"},
            {"ref": "6.28.0"},
            {"ref": "7.28.0-rc.1"},
            {"ref": "7.28.0"},
            {"ref": "6.28.1-rc.1"},
            {"ref": "6.28.1"},
            {"ref": "7.28.1-rc.1"},
            {"ref": "7.28.1"},
            {"ref": "6.29.0-rc.1"},
            {"ref": "6.29.0"},
            {"ref": "7.29.0-rc.1"},
            {"ref": "7.29.0"},
        ],
        200,
    )


class TestGetHighestRepoVersion(unittest.TestCase):
    @mock.patch('requests.get', side_effect=mocked_github_requests_get)
    def test_one_allowed_major_multiple_entries(self, _):
        version = release._get_highest_repo_version(
            "FAKE_TOKEN",
            "target-repo",
            "",
            release.build_compatible_version_re(release.COMPATIBLE_MAJOR_VERSIONS[7], 28),
            release.COMPATIBLE_MAJOR_VERSIONS[7],
        )
        self.assertEqual(version, Version(major=7, minor=28, patch=1))

    @mock.patch('requests.get', side_effect=mocked_github_requests_get)
    def test_one_allowed_major_one_entry(self, _):
        version = release._get_highest_repo_version(
            "FAKE_TOKEN",
            "target-repo",
            "",
            release.build_compatible_version_re(release.COMPATIBLE_MAJOR_VERSIONS[7], 29),
            release.COMPATIBLE_MAJOR_VERSIONS[7],
        )
        self.assertEqual(version, Version(major=7, minor=29, patch=0))

    @mock.patch('requests.get', side_effect=mocked_github_requests_get)
    def test_multiple_allowed_majors_multiple_entries(self, _):
        version = release._get_highest_repo_version(
            "FAKE_TOKEN",
            "target-repo",
            "",
            release.build_compatible_version_re(release.COMPATIBLE_MAJOR_VERSIONS[6], 28),
            release.COMPATIBLE_MAJOR_VERSIONS[6],
        )
        self.assertEqual(version, Version(major=6, minor=28, patch=1))

    @mock.patch('requests.get', side_effect=mocked_github_requests_get)
    def test_multiple_allowed_majors_one_entry(self, _):
        version = release._get_highest_repo_version(
            "FAKE_TOKEN",
            "target-repo",
            "",
            release.build_compatible_version_re(release.COMPATIBLE_MAJOR_VERSIONS[6], 29),
            release.COMPATIBLE_MAJOR_VERSIONS[6],
        )
        self.assertEqual(version, Version(major=6, minor=29, patch=0))

    @mock.patch('requests.get', side_effect=mocked_github_requests_get)
    def test_nonexistant_minor(self, _):
        self.assertRaises(
            Exit,
            release._get_highest_repo_version,
            "FAKE_TOKEN",
            "target-repo",
            "",
            release.build_compatible_version_re(release.COMPATIBLE_MAJOR_VERSIONS[7], 30),
            release.COMPATIBLE_MAJOR_VERSIONS[7],
        )


MOCK_JMXFETCH_CONTENT = "jmxfetch content".encode('utf-8')


def mocked_jmxfetch_requests_get(*_args, **_kwargs):
    class MockResponse:
        def __init__(self, content, status_code):
            self.content = content
            self.status_code = status_code

    return MockResponse(MOCK_JMXFETCH_CONTENT, 200)


class TestUpdateReleaseJsonEntry(unittest.TestCase):
    @mock.patch('requests.get', side_effect=mocked_jmxfetch_requests_get)
    def test_update_release_json_entry(self, _):
        self.maxDiff = None
        initial_release_json = OrderedDict(
            {
                release.nightly_entry_for(6): {
                    "INTEGRATIONS_CORE_VERSION": "master",
                    "OMNIBUS_SOFTWARE_VERSION": "master",
                    "OMNIBUS_RUBY_VERSION": "datadog-5.5.0",
                    "JMXFETCH_VERSION": "0.44.1",
                    "JMXFETCH_HASH": "fd369da4fd24d18dabd7b33abcaac825d386b9558e70f1c621d797faec2a657c",
                    "MACOS_BUILD_VERSION": "master",
                    "WINDOWS_DDNPM_DRIVER": "release-signed",
                    "WINDOWS_DDNPM_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDNPM_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
                    "SECURITY_AGENT_POLICIES_VERSION": "master",
                },
                release.nightly_entry_for(7): {
                    "INTEGRATIONS_CORE_VERSION": "master",
                    "OMNIBUS_SOFTWARE_VERSION": "master",
                    "OMNIBUS_RUBY_VERSION": "datadog-5.5.0",
                    "JMXFETCH_VERSION": "0.44.1",
                    "JMXFETCH_HASH": "fd369da4fd24d18dabd7b33abcaac825d386b9558e70f1c621d797faec2a657c",
                    "MACOS_BUILD_VERSION": "master",
                    "WINDOWS_DDNPM_DRIVER": "release-signed",
                    "WINDOWS_DDNPM_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDNPM_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
                    "SECURITY_AGENT_POLICIES_VERSION": "master",
                },
                release.release_entry_for(6): {
                    "INTEGRATIONS_CORE_VERSION": "master",
                    "OMNIBUS_SOFTWARE_VERSION": "master",
                    "OMNIBUS_RUBY_VERSION": "datadog-5.5.0",
                    "JMXFETCH_VERSION": "0.44.1",
                    "JMXFETCH_HASH": "fd369da4fd24d18dabd7b33abcaac825d386b9558e70f1c621d797faec2a657c",
                    "MACOS_BUILD_VERSION": "master",
                    "WINDOWS_DDNPM_DRIVER": "release-signed",
                    "WINDOWS_DDNPM_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDNPM_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
                    "SECURITY_AGENT_POLICIES_VERSION": "master",
                },
                release.release_entry_for(7): {
                    "INTEGRATIONS_CORE_VERSION": "master",
                    "OMNIBUS_SOFTWARE_VERSION": "master",
                    "OMNIBUS_RUBY_VERSION": "datadog-5.5.0",
                    "JMXFETCH_VERSION": "0.44.1",
                    "JMXFETCH_HASH": "fd369da4fd24d18dabd7b33abcaac825d386b9558e70f1c621d797faec2a657c",
                    "MACOS_BUILD_VERSION": "master",
                    "WINDOWS_DDNPM_DRIVER": "release-signed",
                    "WINDOWS_DDNPM_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDNPM_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
                    "SECURITY_AGENT_POLICIES_VERSION": "master",
                },
            }
        )

        integrations_version = Version(major=7, minor=30, patch=1, rc=2)
        omnibus_ruby_version = Version(major=7, minor=30, patch=1, rc=1)
        omnibus_software_version = Version(major=7, minor=30, patch=0)
        macos_build_version = Version(major=7, minor=30, patch=0)
        jmxfetch_version = Version(major=0, minor=45, patch=0)
        security_agent_policies_version = Version(prefix="v", major="0", minor="15")
        windows_ddnpm_driver = "release-signed"
        windows_ddnpm_version = "1.2.1"
        windows_ddnpm_shasum = "windowsddnpmshasum"

        release_json = release._update_release_json_entry(
            release_json=initial_release_json,
            release_entry=release.release_entry_for(7),
            integrations_version=integrations_version,
            omnibus_ruby_version=omnibus_ruby_version,
            omnibus_software_version=omnibus_software_version,
            macos_build_version=macos_build_version,
            jmxfetch_version=jmxfetch_version,
            security_agent_policies_version=security_agent_policies_version,
            windows_ddnpm_driver=windows_ddnpm_driver,
            windows_ddnpm_version=windows_ddnpm_version,
            windows_ddnpm_shasum=windows_ddnpm_shasum,
        )

        expected_release_json = OrderedDict(
            {
                release.nightly_entry_for(6): {
                    "INTEGRATIONS_CORE_VERSION": "master",
                    "OMNIBUS_SOFTWARE_VERSION": "master",
                    "OMNIBUS_RUBY_VERSION": "datadog-5.5.0",
                    "JMXFETCH_VERSION": "0.44.1",
                    "JMXFETCH_HASH": "fd369da4fd24d18dabd7b33abcaac825d386b9558e70f1c621d797faec2a657c",
                    "MACOS_BUILD_VERSION": "master",
                    "WINDOWS_DDNPM_DRIVER": "release-signed",
                    "WINDOWS_DDNPM_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDNPM_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
                    "SECURITY_AGENT_POLICIES_VERSION": "master",
                },
                release.nightly_entry_for(7): {
                    "INTEGRATIONS_CORE_VERSION": "master",
                    "OMNIBUS_SOFTWARE_VERSION": "master",
                    "OMNIBUS_RUBY_VERSION": "datadog-5.5.0",
                    "JMXFETCH_VERSION": "0.44.1",
                    "JMXFETCH_HASH": "fd369da4fd24d18dabd7b33abcaac825d386b9558e70f1c621d797faec2a657c",
                    "MACOS_BUILD_VERSION": "master",
                    "WINDOWS_DDNPM_DRIVER": "release-signed",
                    "WINDOWS_DDNPM_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDNPM_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
                    "SECURITY_AGENT_POLICIES_VERSION": "master",
                },
                release.release_entry_for(6): {
                    "INTEGRATIONS_CORE_VERSION": "master",
                    "OMNIBUS_SOFTWARE_VERSION": "master",
                    "OMNIBUS_RUBY_VERSION": "datadog-5.5.0",
                    "JMXFETCH_VERSION": "0.44.1",
                    "JMXFETCH_HASH": "fd369da4fd24d18dabd7b33abcaac825d386b9558e70f1c621d797faec2a657c",
                    "MACOS_BUILD_VERSION": "master",
                    "WINDOWS_DDNPM_DRIVER": "release-signed",
                    "WINDOWS_DDNPM_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDNPM_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
                    "SECURITY_AGENT_POLICIES_VERSION": "master",
                },
                release.release_entry_for(7): {
                    "INTEGRATIONS_CORE_VERSION": str(integrations_version),
                    "OMNIBUS_SOFTWARE_VERSION": str(omnibus_software_version),
                    "OMNIBUS_RUBY_VERSION": str(omnibus_ruby_version),
                    "JMXFETCH_VERSION": str(jmxfetch_version),
                    "JMXFETCH_HASH": hashlib.sha256(MOCK_JMXFETCH_CONTENT).hexdigest(),
                    "MACOS_BUILD_VERSION": str(macos_build_version),
                    "WINDOWS_DDNPM_DRIVER": str(windows_ddnpm_driver),
                    "WINDOWS_DDNPM_VERSION": str(windows_ddnpm_version),
                    "WINDOWS_DDNPM_SHASUM": str(windows_ddnpm_shasum),
                    "SECURITY_AGENT_POLICIES_VERSION": str(security_agent_policies_version),
                },
            }
        )

        self.assertDictEqual(release_json, expected_release_json)


class TestGetReleaseVersionFromReleaseJson(unittest.TestCase):
    test_release_json = {
        release.nightly_entry_for(6): {"JMXFETCH_VERSION": "0.44.1", "SECURITY_AGENT_POLICIES_VERSION": "master"},
        release.nightly_entry_for(7): {"JMXFETCH_VERSION": "0.44.1", "SECURITY_AGENT_POLICIES_VERSION": "master"},
        release.release_entry_for(6): {"JMXFETCH_VERSION": "0.43.0", "SECURITY_AGENT_POLICIES_VERSION": "v0.10"},
        release.release_entry_for(7): {"JMXFETCH_VERSION": "0.44.1", "SECURITY_AGENT_POLICIES_VERSION": "v0.10"},
    }

    def test_release_version_6(self):
        version = release._get_release_version_from_release_json(self.test_release_json, 6, release.VERSION_RE)
        self.assertEqual(version, release.release_entry_for(6))

    def test_release_version_7(self):
        version = release._get_release_version_from_release_json(self.test_release_json, 7, release.VERSION_RE)
        self.assertEqual(version, release.release_entry_for(7))

    def test_release_jmxfetch_version_6(self):
        version = release._get_release_version_from_release_json(
            self.test_release_json, 6, release.VERSION_RE, release_json_key="JMXFETCH_VERSION"
        )
        self.assertEqual(version, Version(major=0, minor=43, patch=0))

    def test_release_jmxfetch_version_7(self):
        version = release._get_release_version_from_release_json(
            self.test_release_json, 7, release.VERSION_RE, release_json_key="JMXFETCH_VERSION"
        )
        self.assertEqual(version, Version(major=0, minor=44, patch=1))

    def test_release_security_version_6(self):
        version = release._get_release_version_from_release_json(
            self.test_release_json, 6, release.VERSION_RE, release_json_key="SECURITY_AGENT_POLICIES_VERSION"
        )
        self.assertEqual(version, Version(prefix="v", major=0, minor=10))

    def test_release_security_version_7(self):
        version = release._get_release_version_from_release_json(
            self.test_release_json, 7, release.VERSION_RE, release_json_key="SECURITY_AGENT_POLICIES_VERSION"
        )
        self.assertEqual(version, Version(prefix="v", major=0, minor=10))


class TestGetWindowsDDNPMReleaseJsonInfo(unittest.TestCase):
    test_release_json = {
        release.nightly_entry_for(6): {
            "WINDOWS_DDNPM_DRIVER": "attestation-signed",
            "WINDOWS_DDNPM_VERSION": "nightly-ddnpm-version",
            "WINDOWS_DDNPM_SHASUM": "nightly-ddnpm-sha",
        },
        release.nightly_entry_for(7): {
            "WINDOWS_DDNPM_DRIVER": "attestation-signed",
            "WINDOWS_DDNPM_VERSION": "nightly-ddnpm-version",
            "WINDOWS_DDNPM_SHASUM": "nightly-ddnpm-sha",
        },
        release.release_entry_for(6): {
            "WINDOWS_DDNPM_DRIVER": "release-signed",
            "WINDOWS_DDNPM_VERSION": "rc3-ddnpm-version",
            "WINDOWS_DDNPM_SHASUM": "rc3-ddnpm-sha",
        },
        release.release_entry_for(7): {
            "WINDOWS_DDNPM_DRIVER": "release-signed",
            "WINDOWS_DDNPM_VERSION": "rc3-ddnpm-version",
            "WINDOWS_DDNPM_SHASUM": "rc3-ddnpm-sha",
        },
    }

    def test_ddnpm_info_is_taken_from_nightly_on_first_rc(self):
        driver, version, shasum = release._get_windows_ddnpm_release_json_info(self.test_release_json, 7, True)

        self.assertEqual(driver, 'attestation-signed')
        self.assertEqual(version, 'nightly-ddnpm-version')
        self.assertEqual(shasum, 'nightly-ddnpm-sha')

    def test_ddnpm_info_is_taken_from_previous_rc_on_subsequent_rcs(self):
        driver, version, shasum = release._get_windows_ddnpm_release_json_info(self.test_release_json, 7, False)

        self.assertEqual(driver, 'release-signed')
        self.assertEqual(version, 'rc3-ddnpm-version')
        self.assertEqual(shasum, 'rc3-ddnpm-sha')


if __name__ == '__main__':
    unittest.main()
