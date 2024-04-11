import unittest
from types import SimpleNamespace
from typing import OrderedDict
from unittest import mock

from invoke.exceptions import Exit

from tasks import release
from tasks.libs.types.version import Version


def fake_tag(value):
    return SimpleNamespace(name=value)


def mocked_github_requests_get(*args, **_kwargs):
    if args[0][-1] == "6":
        return [
            fake_tag("6.28.0-rc.1"),
            fake_tag("6.28.0"),
            fake_tag("6.28.1-rc.1"),
            fake_tag("6.28.1"),
            fake_tag("6.29.0-rc.1"),
            fake_tag("6.29.0"),
        ]

    if args[0][-1] == "7":
        return [
            fake_tag("7.28.0-rc.1"),
            fake_tag("7.28.0"),
            fake_tag("7.28.1-rc.1"),
            fake_tag("7.28.1"),
            fake_tag("7.29.0-rc.1"),
            fake_tag("7.29.0"),
        ]

    return [
        fake_tag("6.28.0-rc.1"),
        fake_tag("6.28.0"),
        fake_tag("7.28.0-rc.1"),
        fake_tag("7.28.0"),
        fake_tag("6.28.1-rc.1"),
        fake_tag("6.28.1"),
        fake_tag("7.28.1-rc.1"),
        fake_tag("7.28.1"),
        fake_tag("6.29.0-rc.1"),
        fake_tag("6.29.0"),
        fake_tag("7.29.0-rc.1"),
        fake_tag("7.29.0"),
    ]


def mocked_github_requests_incorrect_get(*_args, **_kwargs):
    return [
        fake_tag("7.28.0-test"),
        fake_tag("7.28.0-rc.1"),
        fake_tag("7.28.0-rc.2"),
        fake_tag("7.28.0-beta"),
    ]


class TestGetHighestRepoVersion(unittest.TestCase):
    @mock.patch('tasks.release.GithubAPI')
    def test_ignore_incorrect_tag(self, gh_mock):
        gh_instance = mock.MagicMock()
        gh_instance.get_tags.side_effect = mocked_github_requests_incorrect_get
        gh_mock.return_value = gh_instance
        version = release._get_highest_repo_version(
            "target-repo",
            "",
            release.build_compatible_version_re(release.COMPATIBLE_MAJOR_VERSIONS[7], 28),
            release.COMPATIBLE_MAJOR_VERSIONS[7],
        )
        self.assertEqual(version, Version(major=7, minor=28, patch=0, rc=2))

    @mock.patch('tasks.release.GithubAPI')
    def test_one_allowed_major_multiple_entries(self, gh_mock):
        gh_instance = mock.MagicMock()
        gh_instance.get_tags.side_effect = mocked_github_requests_get
        gh_mock.return_value = gh_instance
        version = release._get_highest_repo_version(
            "target-repo",
            "",
            release.build_compatible_version_re(release.COMPATIBLE_MAJOR_VERSIONS[7], 28),
            release.COMPATIBLE_MAJOR_VERSIONS[7],
        )
        self.assertEqual(version, Version(major=7, minor=28, patch=1))

    @mock.patch('tasks.release.GithubAPI')
    def test_one_allowed_major_one_entry(self, gh_mock):
        gh_instance = mock.MagicMock()
        gh_instance.get_tags.side_effect = mocked_github_requests_get
        gh_mock.return_value = gh_instance
        version = release._get_highest_repo_version(
            "target-repo",
            "",
            release.build_compatible_version_re(release.COMPATIBLE_MAJOR_VERSIONS[7], 29),
            release.COMPATIBLE_MAJOR_VERSIONS[7],
        )
        self.assertEqual(version, Version(major=7, minor=29, patch=0))

    @mock.patch('tasks.release.GithubAPI')
    def test_multiple_allowed_majors_multiple_entries(self, gh_mock):
        gh_instance = mock.MagicMock()
        gh_instance.get_tags.side_effect = mocked_github_requests_get
        gh_mock.return_value = gh_instance
        version = release._get_highest_repo_version(
            "target-repo",
            "",
            release.build_compatible_version_re(release.COMPATIBLE_MAJOR_VERSIONS[6], 28),
            release.COMPATIBLE_MAJOR_VERSIONS[6],
        )
        self.assertEqual(version, Version(major=6, minor=28, patch=1))

    @mock.patch('tasks.release.GithubAPI')
    def test_multiple_allowed_majors_one_entry(self, gh_mock):
        gh_instance = mock.MagicMock()
        gh_instance.get_tags.side_effect = mocked_github_requests_get
        gh_mock.return_value = gh_instance
        version = release._get_highest_repo_version(
            "target-repo",
            "",
            release.build_compatible_version_re(release.COMPATIBLE_MAJOR_VERSIONS[6], 29),
            release.COMPATIBLE_MAJOR_VERSIONS[6],
        )
        self.assertEqual(version, Version(major=6, minor=29, patch=0))

    @mock.patch('tasks.release.GithubAPI')
    def test_nonexistant_minor(self, gh_mock):
        gh_instance = mock.MagicMock()
        gh_instance.get_tags.side_effect = mocked_github_requests_get
        gh_mock.return_value = gh_instance
        self.assertRaises(
            Exit,
            release._get_highest_repo_version,
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
                    "WINDOWS_DDPROCMON_DRIVER": "release-signed",
                    "WINDOWS_DDPROCMON_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDPROCMON_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
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
                    "WINDOWS_DDPROCMON_DRIVER": "release-signed",
                    "WINDOWS_DDPROCMON_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDPROCMON_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
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
                    "WINDOWS_DDPROCMON_DRIVER": "release-signed",
                    "WINDOWS_DDPROCMON_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDPROCMON_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
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
                    "WINDOWS_DDPROCMON_DRIVER": "release-signed",
                    "WINDOWS_DDPROCMON_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDPROCMON_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
                },
            }
        )

        integrations_version = Version(major=7, minor=30, patch=1, rc=2)
        omnibus_ruby_version = Version(major=7, minor=30, patch=1, rc=1)
        omnibus_software_version = Version(major=7, minor=30, patch=0)
        macos_build_version = Version(major=7, minor=30, patch=0)
        jmxfetch_version = Version(major=0, minor=45, patch=0)
        jmxfetch_shasum = "jmxfetchhashsum"
        security_agent_policies_version = Version(prefix="v", major="0", minor="15")
        windows_ddnpm_driver = "release-signed"
        windows_ddnpm_version = "1.2.1"
        windows_ddnpm_shasum = "windowsddnpmshasum"
        windows_ddprocmon_driver = "release-signed"
        windows_ddprocmon_version = "1.2.1"
        windows_ddprocmon_shasum = "windowsddprocmonshasum"

        release_json = release._update_release_json_entry(
            release_json=initial_release_json,
            release_entry=release.release_entry_for(7),
            integrations_version=integrations_version,
            omnibus_ruby_version=omnibus_ruby_version,
            omnibus_software_version=omnibus_software_version,
            macos_build_version=macos_build_version,
            jmxfetch_version=jmxfetch_version,
            jmxfetch_shasum=jmxfetch_shasum,
            security_agent_policies_version=security_agent_policies_version,
            windows_ddnpm_driver=windows_ddnpm_driver,
            windows_ddnpm_version=windows_ddnpm_version,
            windows_ddnpm_shasum=windows_ddnpm_shasum,
            windows_ddprocmon_driver=windows_ddprocmon_driver,
            windows_ddprocmon_version=windows_ddprocmon_version,
            windows_ddprocmon_shasum=windows_ddprocmon_shasum,
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
                    "WINDOWS_DDPROCMON_DRIVER": "release-signed",
                    "WINDOWS_DDPROCMON_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDPROCMON_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
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
                    "WINDOWS_DDPROCMON_DRIVER": "release-signed",
                    "WINDOWS_DDPROCMON_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDPROCMON_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
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
                    "WINDOWS_DDPROCMON_DRIVER": "release-signed",
                    "WINDOWS_DDPROCMON_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDPROCMON_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
                },
                release.release_entry_for(7): {
                    "INTEGRATIONS_CORE_VERSION": str(integrations_version),
                    "OMNIBUS_SOFTWARE_VERSION": str(omnibus_software_version),
                    "OMNIBUS_RUBY_VERSION": str(omnibus_ruby_version),
                    "JMXFETCH_VERSION": str(jmxfetch_version),
                    "JMXFETCH_HASH": str(jmxfetch_shasum),
                    "MACOS_BUILD_VERSION": str(macos_build_version),
                    "WINDOWS_DDNPM_DRIVER": str(windows_ddnpm_driver),
                    "WINDOWS_DDNPM_VERSION": str(windows_ddnpm_version),
                    "WINDOWS_DDNPM_SHASUM": str(windows_ddnpm_shasum),
                    "SECURITY_AGENT_POLICIES_VERSION": str(security_agent_policies_version),
                    "WINDOWS_DDPROCMON_DRIVER": str(windows_ddprocmon_driver),
                    "WINDOWS_DDPROCMON_VERSION": str(windows_ddprocmon_version),
                    "WINDOWS_DDPROCMON_SHASUM": str(windows_ddprocmon_shasum),
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
            "WINDOWS_DDPROCMON_DRIVER": "attestation-signed",
            "WINDOWS_DDPROCMON_VERSION": "nightly-ddprocmon-version",
            "WINDOWS_DDPROCMON_SHASUM": "nightly-ddprocmon-sha",
        },
        release.nightly_entry_for(7): {
            "WINDOWS_DDNPM_DRIVER": "attestation-signed",
            "WINDOWS_DDNPM_VERSION": "nightly-ddnpm-version",
            "WINDOWS_DDNPM_SHASUM": "nightly-ddnpm-sha",
            "WINDOWS_DDPROCMON_DRIVER": "attestation-signed",
            "WINDOWS_DDPROCMON_VERSION": "nightly-ddprocmon-version",
            "WINDOWS_DDPROCMON_SHASUM": "nightly-ddprocmon-sha",
        },
        release.release_entry_for(6): {
            "WINDOWS_DDNPM_DRIVER": "release-signed",
            "WINDOWS_DDNPM_VERSION": "rc3-ddnpm-version",
            "WINDOWS_DDNPM_SHASUM": "rc3-ddnpm-sha",
            "WINDOWS_DDPROCMON_DRIVER": "release-signed",
            "WINDOWS_DDPROCMON_VERSION": "rc3-ddprocmon-version",
            "WINDOWS_DDPROCMON_SHASUM": "rc3-ddprocmon-sha",
        },
        release.release_entry_for(7): {
            "WINDOWS_DDNPM_DRIVER": "release-signed",
            "WINDOWS_DDNPM_VERSION": "rc3-ddnpm-version",
            "WINDOWS_DDNPM_SHASUM": "rc3-ddnpm-sha",
            "WINDOWS_DDPROCMON_DRIVER": "release-signed",
            "WINDOWS_DDPROCMON_VERSION": "rc3-ddprocmon-version",
            "WINDOWS_DDPROCMON_SHASUM": "rc3-ddprocmon-sha",
        },
    }

    def test_ddnpm_info_is_taken_from_nightly_on_first_rc(self):
        (
            ddnpm_driver,
            ddnpm_version,
            ddnpm_shasum,
            ddprocmon_driver,
            ddprocmon_version,
            ddprocmon_shasum,
        ) = release._get_windows_release_json_info(self.test_release_json, 7, True)

        self.assertEqual(ddnpm_driver, 'attestation-signed')
        self.assertEqual(ddnpm_version, 'nightly-ddnpm-version')
        self.assertEqual(ddnpm_shasum, 'nightly-ddnpm-sha')
        self.assertEqual(ddprocmon_driver, 'attestation-signed')
        self.assertEqual(ddprocmon_version, 'nightly-ddprocmon-version')
        self.assertEqual(ddprocmon_shasum, 'nightly-ddprocmon-sha')

    def test_ddnpm_info_is_taken_from_previous_rc_on_subsequent_rcs(self):
        (
            ddnpm_driver,
            ddnpm_version,
            ddnpm_shasum,
            ddprocmon_driver,
            ddprocmon_version,
            ddprocmon_shasum,
        ) = release._get_windows_release_json_info(self.test_release_json, 7, False)

        self.assertEqual(ddnpm_driver, 'release-signed')
        self.assertEqual(ddnpm_version, 'rc3-ddnpm-version')
        self.assertEqual(ddnpm_shasum, 'rc3-ddnpm-sha')
        self.assertEqual(ddprocmon_driver, 'release-signed')
        self.assertEqual(ddprocmon_version, 'rc3-ddprocmon-version')
        self.assertEqual(ddprocmon_shasum, 'rc3-ddprocmon-sha')


class TestGetReleaseJsonInfoForNextRC(unittest.TestCase):
    test_release_json = {
        release.nightly_entry_for(6): {
            "VERSION": "ver6_nightly",
            "HASH": "hash6_nightly",
        },
        release.nightly_entry_for(7): {
            "VERSION": "ver7_nightly",
            "HASH": "hash7_nightly",
        },
        release.release_entry_for(6): {
            "VERSION": "ver6_release",
            "HASH": "hash6_release",
        },
        release.release_entry_for(7): {
            "VERSION": "ver7_release",
            "HASH": "hash7_release",
        },
    }

    def test_get_release_json_info_for_next_rc_on_first_rc(self):
        previous_release_json = release._get_release_json_info_for_next_rc(self.test_release_json, 7, True)

        self.assertEqual(
            previous_release_json,
            {
                "VERSION": "ver7_nightly",
                "HASH": "hash7_nightly",
            },
        )

    def test_get_release_json_info_for_next_rc_on_second_rc(self):
        previous_release_json = release._get_release_json_info_for_next_rc(self.test_release_json, 7, False)

        self.assertEqual(
            previous_release_json,
            {
                "VERSION": "ver7_release",
                "HASH": "hash7_release",
            },
        )


class TestGetJMXFetchReleaseJsonInfo(unittest.TestCase):
    test_release_json = {
        release.nightly_entry_for(6): {
            "JMXFETCH_VERSION": "ver6_nightly",
            "JMXFETCH_HASH": "hash6_nightly",
        },
        release.nightly_entry_for(7): {
            "JMXFETCH_VERSION": "ver7_nightly",
            "JMXFETCH_HASH": "hash7_nightly",
        },
        release.release_entry_for(6): {
            "JMXFETCH_VERSION": "ver6_release",
            "JMXFETCH_HASH": "hash6_release",
        },
        release.release_entry_for(7): {
            "JMXFETCH_VERSION": "ver7_release",
            "JMXFETCH_HASH": "hash7_release",
        },
    }

    def test_get_release_json_info_for_next_rc_on_first_rc(self):
        jmxfetch_version, jmxfetch_hash = release._get_jmxfetch_release_json_info(self.test_release_json, 7, True)

        self.assertEqual(jmxfetch_version, "ver7_nightly")
        self.assertEqual(jmxfetch_hash, "hash7_nightly")


class TestCreateBuildLinksPatterns(unittest.TestCase):
    current_version = "7.50.0-rc.1"

    def test_create_build_links_patterns_correct_values(self):
        new_rc_version = "7.51.1-rc.2"
        patterns = release._create_build_links_patterns(self.current_version, new_rc_version)

        self.assertEqual(patterns[".50.0-rc.1"], ".51.1-rc.2")
        self.assertEqual(patterns[".50.0-rc-1"], ".51.1-rc-2")
        self.assertEqual(patterns[".50.0~rc.1"], ".51.1~rc.2")
