from __future__ import annotations

import os
import re
import sys
import unittest
from collections import OrderedDict
from contextlib import contextmanager
from types import SimpleNamespace
from unittest.mock import ANY, MagicMock, call, patch

from invoke import Context, MockContext, Result
from invoke.exceptions import Exit

from tasks import release
from tasks.libs.common.gomodules import GoModule
from tasks.libs.releasing.json import (
    COMPATIBLE_MAJOR_VERSIONS,
    _get_jmxfetch_release_json_info,
    _get_windows_release_json_info,
    _update_release_json_entry,
    find_previous_tags,
    generate_repo_data,
)
from tasks.libs.releasing.version import (
    _get_highest_repo_version,
    _get_release_version_from_release_json,
    build_compatible_version_re,
)
from tasks.libs.types.version import Version

MOCK_TMP_DIR = '/tmp/tmp'


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


@contextmanager
def mock_git_clone():
    @contextmanager
    def get_tmpdir():
        yield MOCK_TMP_DIR

    with patch('tempfile.TemporaryDirectory', side_effect=get_tmpdir) as mock:
        yield mock


class TestGetHighestRepoVersion(unittest.TestCase):
    @patch('tasks.libs.releasing.version.GithubAPI')
    def test_ignore_incorrect_tag(self, gh_mock):
        gh_instance = MagicMock()
        gh_instance.get_tags.side_effect = mocked_github_requests_incorrect_get
        gh_mock.return_value = gh_instance
        version = _get_highest_repo_version(
            "target-repo",
            "",
            build_compatible_version_re(COMPATIBLE_MAJOR_VERSIONS[7], 28),
            COMPATIBLE_MAJOR_VERSIONS[7],
        )
        self.assertEqual(version, Version(major=7, minor=28, patch=0, rc=2))

    @patch('tasks.libs.releasing.version.GithubAPI')
    def test_one_allowed_major_multiple_entries(self, gh_mock):
        gh_instance = MagicMock()
        gh_instance.get_tags.side_effect = mocked_github_requests_get
        gh_mock.return_value = gh_instance
        version = _get_highest_repo_version(
            "target-repo",
            "",
            build_compatible_version_re(COMPATIBLE_MAJOR_VERSIONS[7], 28),
            COMPATIBLE_MAJOR_VERSIONS[7],
        )
        self.assertEqual(version, Version(major=7, minor=28, patch=1))

    @patch('tasks.libs.releasing.version.GithubAPI')
    def test_one_allowed_major_one_entry(self, gh_mock):
        gh_instance = MagicMock()
        gh_instance.get_tags.side_effect = mocked_github_requests_get
        gh_mock.return_value = gh_instance
        version = _get_highest_repo_version(
            "target-repo",
            "",
            build_compatible_version_re(COMPATIBLE_MAJOR_VERSIONS[7], 29),
            COMPATIBLE_MAJOR_VERSIONS[7],
        )
        self.assertEqual(version, Version(major=7, minor=29, patch=0))

    @patch('tasks.libs.releasing.version.GithubAPI')
    def test_multiple_allowed_majors_multiple_entries(self, gh_mock):
        gh_instance = MagicMock()
        gh_instance.get_tags.side_effect = mocked_github_requests_get
        gh_mock.return_value = gh_instance
        version = _get_highest_repo_version(
            "target-repo",
            "",
            build_compatible_version_re(COMPATIBLE_MAJOR_VERSIONS[6], 28),
            COMPATIBLE_MAJOR_VERSIONS[6],
        )
        self.assertEqual(version, Version(major=6, minor=28, patch=1))

    @patch('tasks.libs.releasing.version.GithubAPI')
    def test_multiple_allowed_majors_one_entry(self, gh_mock):
        gh_instance = MagicMock()
        gh_instance.get_tags.side_effect = mocked_github_requests_get
        gh_mock.return_value = gh_instance
        version = _get_highest_repo_version(
            "target-repo",
            "",
            build_compatible_version_re(COMPATIBLE_MAJOR_VERSIONS[6], 29),
            COMPATIBLE_MAJOR_VERSIONS[6],
        )
        self.assertEqual(version, Version(major=6, minor=29, patch=0))

    @patch('tasks.libs.releasing.version.GithubAPI')
    def test_nonexistant_minor(self, gh_mock):
        gh_instance = MagicMock()
        gh_instance.get_tags.side_effect = mocked_github_requests_get
        gh_mock.return_value = gh_instance
        self.assertRaises(
            Exit,
            _get_highest_repo_version,
            "target-repo",
            "",
            build_compatible_version_re(COMPATIBLE_MAJOR_VERSIONS[7], 30),
            COMPATIBLE_MAJOR_VERSIONS[7],
        )


MOCK_JMXFETCH_CONTENT = b"jmxfetch content"


def mocked_jmxfetch_requests_get(*_args, **_kwargs):
    class MockResponse:
        def __init__(self, content, status_code):
            self.content = content
            self.status_code = status_code

    return MockResponse(MOCK_JMXFETCH_CONTENT, 200)


class TestUpdateReleaseJsonEntry(unittest.TestCase):
    @patch('requests.get', side_effect=mocked_jmxfetch_requests_get)
    def test_update_release_json_entry(self, _):
        self.maxDiff = None
        initial_release_json = OrderedDict(
            {
                "dependencies": {
                    "INTEGRATIONS_CORE_VERSION": "master",
                    "OMNIBUS_RUBY_VERSION": "datadog-5.5.0",
                    "JMXFETCH_VERSION": "0.44.1",
                    "JMXFETCH_HASH": "fd369da4fd24d18dabd7b33abcaac825d386b9558e70f1c621d797faec2a657c",
                    "WINDOWS_DDNPM_DRIVER": "release-signed",
                    "WINDOWS_DDNPM_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDNPM_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
                    "SECURITY_AGENT_POLICIES_VERSION": "master",
                    "WINDOWS_DDPROCMON_DRIVER": "release-signed",
                    "WINDOWS_DDPROCMON_VERSION": "0.98.2.git.86.53d1ee4",
                    "WINDOWS_DDPROCMON_SHASUM": "5d31cbf7aea921edd5ba34baf074e496749265a80468b65a034d3796558a909e",
                }
            }
        )

        integrations_version = Version(major=7, minor=30, patch=1, rc=2)
        omnibus_ruby_version = Version(major=7, minor=30, patch=1, rc=1)
        jmxfetch_version = Version(major=0, minor=45, patch=0)
        jmxfetch_shasum = "jmxfetchhashsum"
        security_agent_policies_version = Version(prefix="v", major="0", minor="15")
        windows_ddnpm_driver = "release-signed"
        windows_ddnpm_version = "1.2.1"
        windows_ddnpm_shasum = "windowsddnpmshasum"
        windows_ddprocmon_driver = "release-signed"
        windows_ddprocmon_version = "1.2.1"
        windows_ddprocmon_shasum = "windowsddprocmonshasum"

        release_json = _update_release_json_entry(
            release_json=initial_release_json,
            integrations_version=integrations_version,
            omnibus_ruby_version=omnibus_ruby_version,
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
                "dependencies": {
                    "INTEGRATIONS_CORE_VERSION": str(integrations_version),
                    "OMNIBUS_RUBY_VERSION": str(omnibus_ruby_version),
                    "JMXFETCH_VERSION": str(jmxfetch_version),
                    "JMXFETCH_HASH": str(jmxfetch_shasum),
                    "WINDOWS_DDNPM_DRIVER": str(windows_ddnpm_driver),
                    "WINDOWS_DDNPM_VERSION": str(windows_ddnpm_version),
                    "WINDOWS_DDNPM_SHASUM": str(windows_ddnpm_shasum),
                    "SECURITY_AGENT_POLICIES_VERSION": str(security_agent_policies_version),
                    "WINDOWS_DDPROCMON_DRIVER": str(windows_ddprocmon_driver),
                    "WINDOWS_DDPROCMON_VERSION": str(windows_ddprocmon_version),
                    "WINDOWS_DDPROCMON_SHASUM": str(windows_ddprocmon_shasum),
                }
            }
        )

        self.assertDictEqual(release_json, expected_release_json)


class TestGetReleaseVersionFromReleaseJson(unittest.TestCase):
    test_release_json = {
        "dependencies": {
            "JMXFETCH_VERSION": "0.44.1",
            "SECURITY_AGENT_POLICIES_VERSION": "v0.10",
            "TEST_COMPONENT_VERSION": "invalid_version",
        },
    }

    def test_release_jmxfetch_version(self):
        version = _get_release_version_from_release_json(
            self.test_release_json, release.VERSION_RE, release_json_key="JMXFETCH_VERSION"
        )
        self.assertEqual(version, Version(major=0, minor=44, patch=1))

    def test_release_security_version(self):
        version = _get_release_version_from_release_json(
            self.test_release_json, release.VERSION_RE, release_json_key="SECURITY_AGENT_POLICIES_VERSION"
        )
        self.assertEqual(version, Version(prefix="v", major=0, minor=10))

    def test_invalid_version(self):
        version = _get_release_version_from_release_json(
            self.test_release_json, release.VERSION_RE, release_json_key="TEST_COMPONENT_VERSION"
        )
        self.assertEqual(version, None)


class TestReleaseJsonInfo(unittest.TestCase):
    test_release_json = {
        "dependencies": {
            "VERSION": "ver7_nightly",
            "HASH": "hash7_nightly",
            "JMXFETCH_VERSION": "ver7_nightly",
            "JMXFETCH_HASH": "hash7_nightly",
            "WINDOWS_DDNPM_DRIVER": "attestation-signed",
            "WINDOWS_DDNPM_VERSION": "nightly-ddnpm-version",
            "WINDOWS_DDNPM_SHASUM": "nightly-ddnpm-sha",
            "WINDOWS_DDPROCMON_DRIVER": "attestation-signed",
            "WINDOWS_DDPROCMON_VERSION": "nightly-ddprocmon-version",
            "WINDOWS_DDPROCMON_SHASUM": "nightly-ddprocmon-sha",
        },
    }

    def test_jmxfetch_info_extraction(self):
        """Test extraction of JMXFetch specific fields"""
        jmxfetch_version, jmxfetch_hash = _get_jmxfetch_release_json_info(self.test_release_json)
        self.assertEqual(jmxfetch_version, "ver7_nightly")
        self.assertEqual(jmxfetch_hash, "hash7_nightly")

    def test_windows_info_extraction(self):
        """Test extraction of Windows driver specific fields"""
        (
            ddnpm_driver,
            ddnpm_version,
            ddnpm_shasum,
            ddprocmon_driver,
            ddprocmon_version,
            ddprocmon_shasum,
        ) = _get_windows_release_json_info(self.test_release_json)

        self.assertEqual(ddnpm_driver, 'attestation-signed')
        self.assertEqual(ddnpm_version, 'nightly-ddnpm-version')
        self.assertEqual(ddnpm_shasum, 'nightly-ddnpm-sha')
        self.assertEqual(ddprocmon_driver, 'attestation-signed')
        self.assertEqual(ddprocmon_version, 'nightly-ddprocmon-version')
        self.assertEqual(ddprocmon_shasum, 'nightly-ddprocmon-sha')


class TestCreateBuildLinksPatterns(unittest.TestCase):
    current_version = "7.50.0-rc.1"

    def test_create_build_links_patterns_correct_values(self):
        new_rc_version = "7.51.1-rc.2"
        patterns = release._create_build_links_patterns(self.current_version, new_rc_version)

        self.assertEqual(patterns[".50.0-rc.1"], ".51.1-rc.2")
        self.assertEqual(patterns[".50.0-rc-1"], ".51.1-rc-2")
        self.assertEqual(patterns[".50.0~rc.1"], ".51.1~rc.2")


class TestFindPreviousTags(unittest.TestCase):
    keys = ["HARRY_POTTER_VERSION", "HERMIONE_GRANGER_VERSION", "WEASLEY_VERSION"]

    @patch(
        'tasks.libs.releasing.json.load_release_json',
        new=MagicMock(
            return_value={
                'hogwarts': {
                    'HARRY_POTTER_VERSION': '6.6.6',
                    'HERMIONE_GRANGER_VERSION': '6.6.6',
                    'WEASLEY_VERSION': '6.6.6',
                }
            }
        ),
    )
    def test_one_repo(self):
        repos = ["harry-potter"]
        self.assertEqual({'harry-potter': '6.6.6'}, find_previous_tags("hogwarts", repos, self.keys))

    @patch(
        'tasks.libs.releasing.json.load_release_json',
        new=MagicMock(
            return_value={
                'hogwarts': {
                    'HARRY_POTTER_VERSION': '6.6.6',
                    'HERMIONE_GRANGER_VERSION': '6.6.6',
                    'WEASLEY_VERSION': '6.6.6',
                }
            }
        ),
    )
    def test_several_repos(self):
        repos = ["harry-potter", "hermione-granger", "ronald-weasley"]
        self.assertEqual(
            {'harry-potter': '6.6.6', 'hermione-granger': '6.6.6', 'ronald-weasley': '6.6.6'},
            find_previous_tags("hogwarts", repos, self.keys),
        )

    @patch(
        'tasks.libs.releasing.json.load_release_json',
        new=MagicMock(
            return_value={
                'hogwarts': {
                    'HARRY_POTTER_VERSION': '6.6.6',
                    'HERMIONE_GRANGER_VERSION': '6.6.6',
                    'WEASLEY_VERSION': '6.6.6',
                }
            }
        ),
    )
    def test_no_repo(self):
        repos = ["drago-malfoy"]
        self.assertEqual({}, find_previous_tags("hogwarts", repos, self.keys))

    @patch(
        'tasks.libs.releasing.json.load_release_json',
        new=MagicMock(
            return_value={
                'hogwarts': {
                    'HARRY_POTTER_VERSION': '6.6.6',
                    'HERMIONE_GRANGER_VERSION': '6.6.6',
                    'WEASLEY_VERSION': '6.6.6',
                }
            }
        ),
    )
    def test_match_and_no_match(self):
        repos = ["drago-malfoy", "ronald-weasley"]
        self.assertEqual({'ronald-weasley': '6.6.6'}, find_previous_tags("hogwarts", repos, self.keys))


class TestGenerateRepoData(unittest.TestCase):
    @patch(
        'tasks.libs.releasing.json.find_previous_tags', new=MagicMock(return_value={'integrations-core': '9.1.1-rc.0'})
    )
    def test_integrations_core_only_main(self):
        next_version = MagicMock()
        next_version.branch.return_value = "9.1.x"
        repo_data = generate_repo_data(Context(), True, next_version, "main")
        self.assertEqual(len(repo_data), 1)
        self.assertEqual("9.1.x", repo_data["integrations-core"]["branch"])
        self.assertEqual("9.1.1-rc.0", repo_data["integrations-core"]["previous_tag"])

    @patch(
        'tasks.libs.releasing.json.find_previous_tags', new=MagicMock(return_value={'integrations-core': '9.1.1-rc.0'})
    )
    def test_integrations_core_only_release(self):
        next_version = MagicMock()
        next_version.branch.return_value = "9.1.x"
        repo_data = generate_repo_data(Context(), True, next_version, "9.1.x")
        self.assertEqual(len(repo_data), 1)
        self.assertEqual("9.1.x", repo_data["integrations-core"]["branch"])
        self.assertEqual("9.1.1-rc.0", repo_data["integrations-core"]["previous_tag"])

    @patch(
        'tasks.libs.releasing.json.find_previous_tags',
        new=MagicMock(
            return_value={
                'integrations-core': '9.1.1-rc.0',
                'omnibus-ruby': "5.4.3-rc.2",
            }
        ),
    )
    def test_all_repos_default_branch(self):
        next_version = MagicMock()
        next_version.branch.return_value = "9.1.x"
        repo_data = generate_repo_data(Context(), False, next_version, "main")
        self.assertEqual(len(repo_data), 3)
        self.assertEqual("9.1.x", repo_data["integrations-core"]["branch"])
        self.assertEqual("9.1.1-rc.0", repo_data["integrations-core"]["previous_tag"])
        self.assertEqual("datadog-5.5.0", repo_data["omnibus-ruby"]["branch"])
        self.assertEqual("5.4.3-rc.2", repo_data["omnibus-ruby"]["previous_tag"])
        self.assertEqual("main", repo_data["datadog-agent"]["branch"])
        self.assertEqual("", repo_data["datadog-agent"]["previous_tag"])

    @patch(
        'tasks.libs.releasing.json.find_previous_tags',
        new=MagicMock(
            return_value={
                'integrations-core': '9.1.1-rc.0',
                'omnibus-ruby': "5.4.3-rc.2",
            }
        ),
    )
    def test_all_repos_release(self):
        next_version = MagicMock()
        next_version.branch.return_value = "9.1.x"
        repo_data = generate_repo_data(Context(), False, next_version, "9.1.x")
        self.assertEqual(len(repo_data), 3)
        self.assertEqual("9.1.x", repo_data["integrations-core"]["branch"])
        self.assertEqual("9.1.x", repo_data["omnibus-ruby"]["branch"])
        self.assertEqual("9.1.x", repo_data["datadog-agent"]["branch"])

    @patch('tasks.libs.releasing.json.find_previous_tags', new=MagicMock(return_value={'datadog-agent': '6.53.4-rc.2'}))
    def test_agent_6(self):
        next_version = MagicMock()
        next_version.major = 6
        next_version.branch.return_value = "6.53.x"
        repo_data = generate_repo_data(Context(), False, next_version, "6.53.x")
        self.assertEqual(len(repo_data), 1)
        self.assertEqual("6.53.x", repo_data["datadog-agent"]["branch"])
        self.assertEqual("6.53.4-rc.2", repo_data["datadog-agent"]["previous_tag"])


class TestCheckForChanges(unittest.TestCase):
    @patch('tasks.release.agent_context')
    @patch('builtins.print')
    @patch('tasks.release.next_rc_version')
    @patch(
        'tasks.release.generate_repo_data',
        new=MagicMock(
            return_value={
                'omnibus-ruby': {'branch': 'main', 'previous_tag': '7.55.0-rc.1'},
                'integrations-core': {'branch': '7.55.x', 'previous_tag': '7.55.0-rc.1'},
                'datadog-agent': {'branch': 'main', 'previous_tag': ''},
            }
        ),
    )
    def test_no_changes(self, version_mock, print_mock, _):
        next = MagicMock()
        next.tag_pattern.return_value = "7.55.0*"
        next.__str__.return_value = "7.55.0-rc.2"
        version_mock.return_value = next
        c = MockContext(
            run={
                'git rev-parse --abbrev-ref HEAD': Result("main"),
                'git ls-remote -h https://github.com/DataDog/omnibus-ruby "refs/heads/main"': Result(
                    "4n0th3rc0mm1t1        refs/heads/main"
                ),
                'git ls-remote -t https://github.com/DataDog/omnibus-ruby "7.55.0*"': Result(
                    "this1s4c0mmit1        refs/tags/7.55.0-rc.1\n4n0th3rc0mm1t1        refs/tags/7.55.0-rc.1^{}"
                ),
                'git ls-remote -h https://github.com/DataDog/integrations-core "refs/heads/7.55.x"': Result(
                    "4n0th3rc0mm1t3        refs/heads/main"
                ),
                'git ls-remote -t https://github.com/DataDog/integrations-core "7.55.0*"': Result(
                    "this1s4c0mmit3        refs/tags/7.55.0-rc.1\n4n0th3rc0mm1t3        refs/tags/7.55.0-rc.1^{}"
                ),
                'git ls-remote -h https://github.com/DataDog/datadog-agent "refs/heads/main"': Result(
                    "4n0th3rc0mm1t4        refs/heads/main"
                ),
                'git ls-remote -t https://github.com/DataDog/datadog-agent "7.55.0*"': Result(
                    "this1s4c0mmit4        refs/tags/7.55.0-devel\n4n0th3rc0mm1t4        refs/tags/7.55.0-devel^{}"
                ),
            },
        )
        with self.assertRaises(SystemExit) as e:
            release.check_for_changes(c, "main")
        self.assertEqual(e.exception.code, 0)
        print_mock.assert_not_called()

    @patch('slack_sdk.WebClient', autospec=True)
    @patch('tasks.release.agent_context')
    @patch('builtins.print')
    @patch('tasks.release.next_rc_version')
    @patch(
        'tasks.release.generate_repo_data',
        new=MagicMock(
            return_value={
                'omnibus-ruby': {'branch': 'main', 'previous_tag': '7.55.0-rc.1'},
                'integrations-core': {'branch': '7.55.x', 'previous_tag': '7.55.0-rc.1'},
                'datadog-agent': {'branch': 'main', 'previous_tag': ''},
            }
        ),
    )
    @patch.dict(os.environ, {'GITLAB_CI': 'true', 'GITHUB_ACTIONS': 'true', 'SLACK_DATADOG_AGENT_BOT_TOKEN': 'slock'})
    @patch('os.chdir', new=MagicMock())
    def test_changes_new_commit_first_repo(self, version_mock, print_mock, _, web_mock):
        with mock_git_clone():
            next, client_mock = MagicMock(), MagicMock()
            next.tag_pattern.return_value = "7.55.0*"
            next.__str__.return_value = "7.55.0-rc.2"
            version_mock.return_value = next
            web_mock.return_value = client_mock
            c = MockContext(
                run={
                    'git rev-parse --abbrev-ref HEAD': Result("main"),
                    'git config user.name github-actions[bot]': Result(""),
                    'git config user.email github-actions[bot]@users.noreply.github.com': Result(""),
                    'git ls-remote -h https://github.com/DataDog/omnibus-ruby "refs/heads/main"': Result(
                        "4n0th3rc0mm1t1        refs/heads/main"
                    ),
                    'git ls-remote -t https://github.com/DataDog/omnibus-ruby "7.55.0*"': Result(
                        "this1s4c0mmit1        refs/tags/7.55.0-rc.1\n4n0th3rc0mm1t2        refs/tags/7.55.0-rc.1^{}"
                    ),
                    f'git clone -b main --filter=blob:none --no-checkout https://github.com/DataDog/omnibus-ruby {MOCK_TMP_DIR}': Result(
                        ""
                    ),
                    'rm -rf omnibus-ruby': Result(""),
                    'git ls-remote -h https://github.com/DataDog/integrations-core "refs/heads/7.55.x"': Result(
                        "4n0th3rc0mm1t3        refs/heads/main"
                    ),
                    'git ls-remote -t https://github.com/DataDog/integrations-core "7.55.0*"': Result(
                        "this1s4c0mmit3        refs/tags/7.55.0-rc.1\n4n0th3rc0mm1t3        refs/tags/7.55.0-rc.1^{}"
                    ),
                    'git ls-remote -h https://github.com/DataDog/datadog-agent "refs/heads/main"': Result(
                        "4n0th3rc0mm1t4        refs/heads/main"
                    ),
                    'git ls-remote -t https://github.com/DataDog/datadog-agent "7.55.0*"': Result(
                        "this1s4c0mmit4        refs/tags/7.55.0-devel\n4n0th3rc0mm1t4        refs/tags/7.55.0-devel^{}"
                    ),
                    'git tag 7.55.0-rc.2': Result(""),
                    'git push origin tag 7.55.0-rc.2': Result(""),
                },
            )
            with self.assertRaises(SystemExit) as e:
                release.check_for_changes(c, "main")
            self.assertEqual(e.exception.code, 69)
            calls = [
                call("omnibus-ruby has new commits since 7.55.0-rc.1", file=sys.stderr),
            ]
            print_mock.assert_has_calls(calls)
            client_mock.chat_postMessage.assert_called_once_with(
                channel="#agent-release-sync",
                text=":warning: Please add the `7.55.0-rc.2` tag on the head of `main` for:\n - <https://github.com/DataDog/omnibus-ruby/commits/main/|omnibus-ruby>\nMake sure to tag them before merging the next RC PR.",
            )

    @patch('slack_sdk.WebClient', autospec=True)
    @patch('tasks.release.agent_context')
    @patch('builtins.print')
    @patch('tasks.release.next_rc_version')
    @patch(
        'tasks.release.generate_repo_data',
        new=MagicMock(
            return_value={
                'omnibus-ruby': {'branch': 'main', 'previous_tag': '7.55.0-rc.1'},
                'integrations-core': {'branch': '7.55.x', 'previous_tag': '7.55.0-rc.1'},
                'datadog-agent': {'branch': 'main', 'previous_tag': ''},
            }
        ),
    )
    @patch('os.chdir', new=MagicMock())
    @patch.dict(os.environ, {'GITLAB_CI': 'false', 'GITHUB_ACTIONS': 'false', 'SLACK_DATADOG_AGENT_BOT_TOKEN': 'slick'})
    def test_changes_new_commit_all_repo(self, version_mock, print_mock, _, web_mock):
        with mock_git_clone():
            next, client_mock = MagicMock(), MagicMock()
            next.tag_pattern.return_value = "7.55.0*"
            next.__str__.return_value = "7.55.0-rc.2"
            version_mock.return_value = next
            web_mock.return_value = client_mock
            c = MockContext(
                run={
                    'git rev-parse --abbrev-ref HEAD': Result("main"),
                    'git ls-remote -h https://github.com/DataDog/omnibus-ruby "refs/heads/main"': Result(
                        "4n0th3rc0mm1t8        refs/heads/main"
                    ),
                    'git ls-remote -t https://github.com/DataDog/omnibus-ruby "7.55.0*"': Result(
                        "this1s4c0mmit1        refs/tags/7.55.0-rc.1\n4n0th3rc0mm1t1        refs/tags/7.55.0-rc.1^{}"
                    ),
                    f'git clone -b main --filter=blob:none --no-checkout https://github.com/DataDog/omnibus-ruby {MOCK_TMP_DIR}': Result(
                        ""
                    ),
                    'rm -rf omnibus-ruby': Result(""),
                    'git ls-remote -h https://github.com/DataDog/integrations-core "refs/heads/7.55.x"': Result(
                        "4n0th3rc0mm1t6        refs/heads/main"
                    ),
                    'git ls-remote -t https://github.com/DataDog/integrations-core "7.55.0*"': Result(
                        "this1s4c0mmit3        refs/tags/7.55.0-rc.1\n4n0th3rc0mm1t3        refs/tags/7.55.0-rc.1^{}"
                    ),
                    'git ls-remote -h https://github.com/DataDog/datadog-agent "refs/heads/main"': Result(
                        "4n0th3rc0mm1t5        refs/heads/main"
                    ),
                    'git ls-remote -t https://github.com/DataDog/datadog-agent "7.55.0*"': Result(
                        "this1s4c0mmit4        refs/tags/7.55.0-devel\n4n0th3rc0mm1t4        refs/tags/7.55.0-devel^{}"
                    ),
                    'git tag 7.55.0-rc.2': Result(""),
                    'git push origin tag 7.55.0-rc.2': Result(""),
                },
            )
            with self.assertRaises(SystemExit) as e:
                release.check_for_changes(c, "main")
            self.assertEqual(e.exception.code, 69)
            calls = [
                call("omnibus-ruby has new commits since 7.55.0-rc.1", file=sys.stderr),
                call("integrations-core has new commits since 7.55.0-rc.1", file=sys.stderr),
                call("datadog-agent has new commits since 7.55.0-devel", file=sys.stderr),
            ]
            print_mock.assert_has_calls(calls)
            client_mock.chat_postMessage.assert_called_once_with(
                channel="#agent-release-sync",
                text=":warning: Please add the `7.55.0-rc.2` tag on the head of `main` for:\n - <https://github.com/DataDog/omnibus-ruby/commits/main/|omnibus-ruby>\nMake sure to tag them before merging the next RC PR.",
            )

    @patch('tasks.release.agent_context')
    @patch('builtins.print')
    @patch('tasks.release.next_rc_version')
    @patch(
        'tasks.release.generate_repo_data',
        new=MagicMock(
            return_value={
                'omnibus-ruby': {'branch': 'main', 'previous_tag': '7.55.0-rc.1'},
                'integrations-core': {'branch': '7.55.x', 'previous_tag': '7.55.0-rc.1'},
                'datadog-agent': {'branch': 'main', 'previous_tag': ''},
            }
        ),
    )
    def test_changes_new_release_one_repo(self, version_mock, print_mock, _):
        next = MagicMock()
        next.tag_pattern.return_value = "7.55.0*"
        next.__str__.return_value = "7.55.0-rc.2"
        version_mock.return_value = next
        c = MockContext(
            run={
                'git rev-parse --abbrev-ref HEAD': Result("main"),
                'git ls-remote -h https://github.com/DataDog/omnibus-ruby "refs/heads/main"': Result(
                    "4n0th3rc0mm1t1        refs/heads/main"
                ),
                'git ls-remote -t https://github.com/DataDog/omnibus-ruby "7.55.0*"': Result(
                    "this1s4c0mmit1        refs/tags/7.55.0-rc.1\n4n0th3rc0mm1t1        refs/tags/7.55.0-rc.2^{}"
                ),
                'git ls-remote -h https://github.com/DataDog/integrations-core "refs/heads/7.55.x"': Result(
                    "4n0th3rc0mm1t3        refs/heads/main"
                ),
                'git ls-remote -t https://github.com/DataDog/integrations-core "7.55.0*"': Result(
                    "this1s4c0mmit3        refs/tags/7.55.0-rc.1\n4n0th3rc0mm1t3        refs/tags/7.55.0-rc.1^{}"
                ),
                'git ls-remote -h https://github.com/DataDog/datadog-agent "refs/heads/main"': Result(
                    "4n0th3rc0mm1t4        refs/heads/main"
                ),
                'git ls-remote -t https://github.com/DataDog/datadog-agent "7.55.0*"': Result(
                    "this1s4c0mmit4        refs/tags/7.55.0-devel\n4n0th3rc0mm1t4        refs/tags/7.55.0-devel^{}"
                ),
            },
        )
        with self.assertRaises(SystemExit) as e:
            release.check_for_changes(c, "main")
        self.assertEqual(e.exception.code, 69)
        calls = [
            call(
                "omnibus-ruby has a new tag 7.55.0-rc.2 since last release candidate (was 7.55.0-rc.1)",
                file=sys.stderr,
            ),
        ]
        print_mock.assert_has_calls(calls, any_order=True)
        self.assertEqual(print_mock.call_count, 1)

    @patch('slack_sdk.WebClient', autospec=True)
    @patch('tasks.release.agent_context')
    @patch('builtins.print')
    @patch('tasks.release.next_rc_version')
    @patch(
        'tasks.release.generate_repo_data',
        new=MagicMock(
            return_value={
                'omnibus-ruby': {'branch': '7.55.x', 'previous_tag': '7.55.0-rc.1'},
                'integrations-core': {'branch': '7.55.x', 'previous_tag': '7.55.0-rc.1'},
                'datadog-agent': {'branch': '7.55.x', 'previous_tag': ''},
            }
        ),
    )
    @patch.dict(os.environ, {'GITLAB_CI': 'true', 'GITHUB_ACTIONS': 'true', 'SLACK_DATADOG_AGENT_BOT_TOKEN': 'sluck'})
    @patch('os.chdir', new=MagicMock())
    def test_changes_new_commit_second_repo_branch_out(self, version_mock, print_mock, _, web_mock):
        with mock_git_clone():
            next, client_mock = MagicMock(), MagicMock()
            next.tag_pattern.return_value = "7.55.0*"
            next.__str__.return_value = "7.55.0-rc.2"
            version_mock.return_value = next
            web_mock.return_value = client_mock
            c = MockContext(
                run={
                    'git rev-parse --abbrev-ref HEAD': Result("main"),
                    'git config user.name github-actions[bot]': Result(""),
                    'git config user.email github-actions[bot]@users.noreply.github.com': Result(""),
                    'git ls-remote -h https://github.com/DataDog/omnibus-ruby "refs/heads/7.55.x"': Result(
                        "4n0th3rc0mm1t9        refs/heads/main"
                    ),
                    'git ls-remote -t https://github.com/DataDog/omnibus-ruby "7.55.0*"': Result(
                        "this1s4c0mmit1        refs/tags/7.55.0-rc.1\n4n0th3rc0mm1t1        refs/tags/7.55.0-rc.1^{}"
                    ),
                    f'git clone -b 7.55.x --filter=blob:none --no-checkout https://github.com/DataDog/omnibus-ruby {MOCK_TMP_DIR}': Result(
                        ""
                    ),
                    'rm -rf omnibus-ruby': Result(""),
                    'git ls-remote -h https://github.com/DataDog/integrations-core "refs/heads/7.55.x"': Result(
                        "4n0th3rc0mm1t3        refs/heads/main"
                    ),
                    'git ls-remote -t https://github.com/DataDog/integrations-core "7.55.0*"': Result(
                        "this1s4c0mmit3        refs/tags/7.55.0-rc.1\n4n0th3rc0mm1t3        refs/tags/7.55.0-rc.1^{}"
                    ),
                    'git ls-remote -h https://github.com/DataDog/datadog-agent "refs/heads/7.55.x"': Result(
                        "4n0th3rc0mm1t4        refs/heads/main"
                    ),
                    'git ls-remote -t https://github.com/DataDog/datadog-agent "7.55.0*"': Result(
                        "this1s4c0mmit4        refs/tags/7.55.0-devel\n4n0th3rc0mm1t4        refs/tags/7.55.0-devel^{}"
                    ),
                    'git tag 7.55.0-rc.2': Result(""),
                    'git push origin tag 7.55.0-rc.2': Result(""),
                },
            )
            with self.assertRaises(SystemExit) as e:
                release.check_for_changes(c, "7.55.x")
            self.assertEqual(e.exception.code, 69)
            calls = [
                call("omnibus-ruby has new commits since 7.55.0-rc.1", file=sys.stderr),
            ]
            print_mock.assert_has_calls(calls)
            client_mock.chat_postMessage.assert_called_once_with(
                channel="#agent-release-sync",
                text=":warning: Please add the `7.55.0-rc.2` tag on the head of `7.55.x` for:\n - <https://github.com/DataDog/omnibus-ruby/commits/7.55.x/|omnibus-ruby>\nMake sure to tag them before merging the next RC PR.",
            )

    # def test_no_changes_warning(self, print_mock):
    @patch('tasks.release.agent_context')
    @patch('builtins.print')
    @patch('tasks.release.next_rc_version')
    @patch(
        'tasks.release.generate_repo_data',
        new=MagicMock(
            return_value={
                'integrations-core': {'branch': '7.55.x', 'previous_tag': '7.55.0-rc.1'},
            }
        ),
    )
    def test_no_changes_warning(self, version_mock, print_mock, _):
        next = MagicMock()
        next.tag_pattern.return_value = "7.55.0*"
        next.__str__.return_value = "7.55.0-rc.2"
        version_mock.return_value = next
        c = MockContext(
            run={
                'git rev-parse --abbrev-ref HEAD': Result("main"),
                'git ls-remote -h https://github.com/DataDog/integrations-core "refs/heads/7.55.x"': Result(
                    "4n0th3rc0mm1t3        refs/heads/main"
                ),
                'git ls-remote -t https://github.com/DataDog/integrations-core "7.55.0*"': Result(
                    "this1s4c0mmit3        refs/tags/7.55.0-rc.1\n4n0th3rc0mm1t3        refs/tags/7.55.0-rc.1^{}"
                ),
            },
        )
        with self.assertRaises(SystemExit) as e:
            release.check_for_changes(c, "main", True)
        self.assertEqual(e.exception.code, 0)
        print_mock.assert_not_called()

    @patch('tasks.release.agent_context')
    @patch('builtins.print')
    @patch('tasks.release.next_rc_version')
    @patch(
        'tasks.release.generate_repo_data',
        new=MagicMock(
            return_value={
                'integrations-core': {'branch': '7.55.x', 'previous_tag': '7.55.0-rc.1'},
            }
        ),
    )
    @patch('tasks.release.warn_new_commits', new=MagicMock())
    def test_changes_other_repo_warning(self, version_mock, print_mock, _):
        next = MagicMock()
        next.tag_pattern.return_value = "7.55.0*"
        next.__str__.return_value = "7.55.0-rc.2"
        version_mock.return_value = next
        c = MockContext(
            run={
                'git rev-parse --abbrev-ref HEAD': Result("main"),
                'git ls-remote -h https://github.com/DataDog/integrations-core "refs/heads/7.55.x"': Result(
                    "4n0th3rc0mm1t3        refs/heads/main"
                ),
                'git ls-remote -t https://github.com/DataDog/integrations-core "7.55.0*"': Result(
                    "this1s4c0mmit3        refs/tags/7.55.0-rc.1\n4n0th3rc0mm1t3        refs/tags/7.55.0-rc.1^{}"
                ),
            },
        )
        with self.assertRaises(SystemExit) as e:
            release.check_for_changes(c, "main", True)
        self.assertEqual(e.exception.code, 0)
        print_mock.assert_not_called()

    @patch('tasks.release.agent_context')
    @patch('builtins.print')
    @patch('tasks.release.next_rc_version')
    @patch(
        'tasks.release.generate_repo_data',
        new=MagicMock(
            return_value={
                'integrations-core': {'branch': '7.55.x', 'previous_tag': '7.55.0-rc.1'},
            }
        ),
    )
    @patch('tasks.release.warn_new_commits', new=MagicMock())
    def test_changes_integrations_core_warning(self, version_mock, print_mock, _):
        next = MagicMock()
        next.tag_pattern.return_value = "7.55.0*"
        next.__str__.return_value = "7.55.0-rc.2"
        version_mock.return_value = next
        c = MockContext(
            run={
                'git rev-parse --abbrev-ref HEAD': Result("main"),
                'git ls-remote -h https://github.com/DataDog/integrations-core "refs/heads/7.55.x"': Result(
                    "4n0th3rc0mm1t9        refs/heads/main"
                ),
                'git ls-remote -t https://github.com/DataDog/integrations-core "7.55.0*"': Result(
                    "this1s4c0mmit3        refs/tags/7.55.0-rc.1\n4n0th3rc0mm1t3        refs/tags/7.55.0-rc.1^{}"
                ),
            },
        )
        with self.assertRaises(SystemExit) as e:
            release.check_for_changes(c, "main", True)
        self.assertEqual(e.exception.code, 69)
        calls = [
            call("integrations-core has new commits since 7.55.0-rc.1", file=sys.stderr),
        ]
        print_mock.assert_has_calls(calls)
        self.assertEqual(print_mock.call_count, 1)

    @patch('tasks.release.agent_context')
    @patch('builtins.print')
    @patch('tasks.release.next_rc_version')
    @patch(
        'tasks.release.generate_repo_data',
        new=MagicMock(
            return_value={
                'integrations-core': {'branch': '7.55.x', 'previous_tag': '7.55.0-rc.1'},
            }
        ),
    )
    @patch('tasks.release.warn_new_commits', new=MagicMock())
    def test_changes_integrations_core_warning_branch_out(self, version_mock, print_mock, _):
        next = MagicMock()
        next.tag_pattern.return_value = "7.55.0*"
        next.__str__.return_value = "7.55.0-rc.2"
        version_mock.return_value = next
        c = MockContext(
            run={
                'git rev-parse --abbrev-ref HEAD': Result("main"),
                'git ls-remote -h https://github.com/DataDog/integrations-core "refs/heads/7.55.x"': Result(
                    "4n0th3rc0mm1t9        refs/heads/main"
                ),
                'git ls-remote -t https://github.com/DataDog/integrations-core "7.55.0*"': Result(
                    "this1s4c0mmit3        refs/tags/7.55.0-rc.1\n4n0th3rc0mm1t3        refs/tags/7.55.0-rc.1^{}"
                ),
            },
        )
        with self.assertRaises(SystemExit) as e:
            release.check_for_changes(c, "7.55.x", True)
        self.assertEqual(e.exception.code, 69)
        calls = [
            call("integrations-core has new commits since 7.55.0-rc.1", file=sys.stderr),
        ]
        print_mock.assert_has_calls(calls)
        self.assertEqual(print_mock.call_count, 1)


class TestUpdateModules(unittest.TestCase):
    @patch('tasks.release.agent_context', new=MagicMock())
    def test_update_module_no_run_for_optional_in_agent_6(self):
        c = MockContext(run=Result("yolo"))
        new_e2e = GoModule('test/new-e2e')
        new_e2e._dependencies = ['pkg/util/optional', 'pkg/utils/pointer']
        optional = GoModule('pkg/util/optional')
        optional._dependencies = []
        pointer = GoModule('pkg/utils/pointer')
        pointer._dependencies = []
        with patch('tasks.release.get_default_modules') as mock_modules:
            mock_dict = MagicMock()
            mock_dict.values.return_value = [new_e2e]
            mock_dict.__getitem__.side_effect = [new_e2e, optional, pointer]
            mock_modules.return_value = mock_dict
            release.update_modules(c, version="6.53.1337")
        edit_optional = re.compile(r"pkg/util/optional.*test/new-e2e")
        self.assertFalse(any(edit_optional.search(call[0][0]) for call in c.run.call_args_list))
        self.assertEqual(c.run.call_count, 1)

    @patch('tasks.release.agent_context', new=MagicMock())
    def test_update_module_optional_in_agent_7(self):
        c = MockContext(run=Result("yolo"))
        new_e2e = GoModule('test/new-e2e')
        new_e2e._dependencies = ['pkg/util/optional', 'pkg/utils/pointer']
        optional = GoModule('pkg/util/optional')
        optional._dependencies = []
        pointer = GoModule('pkg/utils/pointer')
        pointer._dependencies = []
        with patch('tasks.release.get_default_modules') as mock_modules:
            mock_dict = MagicMock()
            mock_dict.values.return_value = [new_e2e]
            mock_dict.__getitem__.side_effect = [new_e2e, optional, pointer]
            mock_modules.return_value = mock_dict
            release.update_modules(c, version="7.53.1337")
        edit_optional = re.compile(r"pkg/util/optional.*test/new-e2e")
        self.assertTrue(any(edit_optional.search(call[0][0]) for call in c.run.call_args_list))
        self.assertEqual(c.run.call_count, 2)


class TestTagModules(unittest.TestCase):
    @patch('tasks.release.__tag_single_module', new=MagicMock(side_effect=[[str(i)] for i in range(2)]))
    @patch('tasks.release.agent_context', new=MagicMock())
    @patch.dict(os.environ, {'GITLAB_CI': 'false', 'GITHUB_ACTIONS': 'false'})
    def test_2_tags(self):
        c = MockContext(run=Result("yolo"))
        with patch('tasks.release.get_default_modules') as mock_modules:
            mock_dict = MagicMock()
            mock_dict.values.return_value = 2 * [GoModule('pkg/one')]
            mock_modules.return_value = mock_dict
            release.tag_modules(c, version="version")
        self.assertEqual(c.run.call_count, 1)
        c.run.assert_called_with("git push origin 0 1")

    @patch('tasks.release.__tag_single_module', new=MagicMock(side_effect=[[str(i)] for i in range(3)]))
    @patch('tasks.release.agent_context', new=MagicMock())
    @patch.dict(os.environ, {'GITLAB_CI': 'false', 'GITHUB_ACTIONS': 'false'})
    def test_3_tags(self):
        c = MockContext(run=Result("yolo"))
        with patch('tasks.release.get_default_modules') as mock_modules:
            mock_dict = MagicMock()
            mock_dict.values.return_value = 3 * [GoModule('pkg/one')]
            mock_modules.return_value = mock_dict
            release.tag_modules(c, version="version")
        self.assertEqual(c.run.call_count, 1)
        c.run.assert_called_with("git push origin 0 1 2")

    @patch('tasks.release.__tag_single_module', new=MagicMock(side_effect=[[str(i)] for i in range(4)]))
    @patch('tasks.release.agent_context', new=MagicMock())
    @patch.dict(os.environ, {'GITLAB_CI': 'false', 'GITHUB_ACTIONS': 'false'})
    def test_4_tags(self):
        c = MockContext(run=Result("yolo"))
        with patch('tasks.release.get_default_modules') as mock_modules:
            mock_dict = MagicMock()
            mock_dict.values.return_value = 4 * [GoModule('pkg/one')]
            mock_modules.return_value = mock_dict
            release.tag_modules(c, version="version")
        self.assertEqual(c.run.call_count, 2)
        calls = [
            call("git push origin 0 1 2"),
            call("git push origin 3"),
        ]
        c.run.assert_has_calls(calls)

    @patch('tasks.release.__tag_single_module', new=MagicMock(side_effect=[[str(i)] for i in range(100)]))
    @patch('tasks.release.agent_context', new=MagicMock())
    @patch.dict(os.environ, {'GITLAB_CI': 'false', 'GITHUB_ACTIONS': 'false'})
    def test_100_tags(self):
        c = MockContext(run=Result("yolo"))
        with patch('tasks.release.get_default_modules') as mock_modules:
            mock_dict = MagicMock()
            mock_dict.values.return_value = 100 * [GoModule('pkg/one')]
            mock_modules.return_value = mock_dict
            release.tag_modules(c, version="version")
        self.assertEqual(c.run.call_count, 34)


class TestTagVersion(unittest.TestCase):
    c = MockContext(run=Result("yolo"))

    @patch('tasks.release.__tag_single_module')
    @patch('tasks.release.push_tags_in_batches')
    @patch('tasks.release.is_agent6', new=MagicMock(return_value=True))
    @patch('tasks.release.is_qualification', new=MagicMock(return_value=False))
    @patch('tasks.release.agent_context', new=MagicMock())
    @patch.dict(os.environ, {'GITLAB_CI': 'false', 'GITHUB_ACTIONS': 'false'})
    def test_not_in_qualification_phase(self, push_tags_in_batches_mock, tag_single_module_mock):
        rc_version = "6.53.5-rc.2"
        release.tag_version(self.c, start_qual=False, version=rc_version)
        tag_single_module_mock.assert_called_with(self.c, ANY, rc_version, ANY, ANY, ANY)
        assert tag_single_module_mock.call_count == 1
        assert push_tags_in_batches_mock.call_count == 1

    @patch('tasks.release.__tag_single_module')
    @patch('tasks.release.push_tags_in_batches')
    @patch('time.time', new=MagicMock(return_value=1234))
    @patch('tasks.release.is_agent6', new=MagicMock(return_value=True))
    @patch('tasks.release.is_qualification', new=MagicMock(return_value=False))
    @patch('tasks.release.agent_context', new=MagicMock())
    @patch.dict(os.environ, {'GITLAB_CI': 'false', 'GITHUB_ACTIONS': 'false'})
    def test_start_qualification_phase(self, push_tags_in_batches_mock, tag_single_module_mock):
        rc_version = "6.53.5-rc.2"
        release.tag_version(self.c, start_qual=True, version=rc_version)
        calls = tag_single_module_mock.call_args_list
        calls[0].assert_called_with(self.c, ANY, rc_version, ANY, ANY, ANY)
        calls[1].assert_called_with(self.c, ANY, "qualification-1234", ANY, ANY, ANY)
        assert tag_single_module_mock.call_count == 2
        assert push_tags_in_batches_mock.call_count == 1

    @patch('tasks.release.__tag_single_module')
    @patch('tasks.release.push_tags_in_batches')
    @patch('time.time', new=MagicMock(return_value=2345))
    @patch('tasks.release.is_agent6', new=MagicMock(return_value=True))
    @patch('tasks.release.is_qualification', new=MagicMock(return_value=True))
    @patch('tasks.release.agent_context', new=MagicMock())
    @patch.dict(os.environ, {'GITLAB_CI': 'false', 'GITHUB_ACTIONS': 'false'})
    def test_during_qualification_phase(self, push_tags_in_batches_mock, tag_single_module_mock):
        rc_version = "6.53.5-rc.3"
        release.tag_version(self.c, start_qual=False, version=rc_version)
        calls = tag_single_module_mock.call_args_list
        calls[0].assert_called_with(self.c, ANY, rc_version, ANY, ANY, ANY)
        calls[1].assert_called_with(self.c, ANY, "qualification-2345", ANY, ANY, ANY)
        assert tag_single_module_mock.call_count == 2
        assert push_tags_in_batches_mock.call_count == 1

    @patch('tasks.release.__tag_single_module')
    @patch('tasks.release.push_tags_in_batches')
    @patch('tasks.release.is_agent6', new=MagicMock(return_value=True))
    @patch('tasks.release.is_qualification', new=MagicMock(return_value=True))
    @patch('tasks.release.agent_context', new=MagicMock())
    @patch('tasks.release.get_qualification_tags', new=MagicMock())
    @patch.dict(os.environ, {'GITLAB_CI': 'false', 'GITHUB_ACTIONS': 'false'})
    def test_end_qualification_phase(self, push_tags_in_batches_mock, tag_single_module_mock):
        final_release_version = "6.53.5"
        release.tag_version(self.c, start_qual=False, version=final_release_version)
        tag_single_module_mock.assert_called_with(self.c, ANY, final_release_version, ANY, ANY, ANY)
        assert tag_single_module_mock.call_count == 1
        assert push_tags_in_batches_mock.call_count == 2


class TestGetQualificationTags(unittest.TestCase):
    c = MockContext(run=Result("yolo"))

    @patch('tasks.release.qualification_tag_query')
    @patch('tasks.release.agent_context', new=MagicMock())
    def test_returns_all_tags(self, qualification_tag_query_mock):
        qualification_tag_query_mock.return_value = ['hash2\tqualification_2345^{}', 'hash1\tqualification_1234^{}']
        tags = release.get_qualification_tags(self.c, "6.53.x")
        qualification_tag_query_mock.assert_called_with(self.c, "6.53.x", sort=True)
        assert tags == [['hash2', 'qualification_2345'], ['hash1', 'qualification_1234']]
        self.assertEqual(len(tags), 2)

    @patch('tasks.release.qualification_tag_query')
    @patch('tasks.release.agent_context', new=MagicMock())
    def test_returns_only_one_tag(self, qualification_tag_query_mock):
        qualification_tag_query_mock.return_value = ['hash2\tqualification_2345^{}', 'hash1\tqualification_1234^{}']
        tags = release.get_qualification_tags(self.c, "6.53.x", latest_tag=True)
        qualification_tag_query_mock.assert_called_with(self.c, "6.53.x", sort=True)
        assert tags == [['hash2', 'qualification_2345']]
        self.assertEqual(len(tags), 1)


class TestIsQualification(unittest.TestCase):
    c = MockContext(run=Result("yolo"))

    @patch('builtins.print')
    @patch('tasks.release.qualification_tag_query', new=MagicMock(return_value="hash1\tqualification_1234"))
    def test_is_qualification(self, print_mock):
        self.assertTrue(release.is_qualification(self.c, "6.53.x"))
        assert print_mock.call_count == 0
        self.assertTrue(release.is_qualification(self.c, "6.53.x", output=True))
        print_mock.assert_called_with("true")
        assert print_mock.call_count == 1

    @patch('builtins.print')
    @patch('tasks.release.qualification_tag_query', new=MagicMock(return_value=None))
    def test_is_not_qualification(self, print_mock):
        self.assertFalse(release.is_qualification(self.c, "6.53.x"))
        assert print_mock.call_count == 0
        self.assertFalse(release.is_qualification(self.c, "6.53.x", output=True))
        print_mock.assert_called_with("false")
        assert print_mock.call_count == 1
