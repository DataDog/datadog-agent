import unittest
import hashlib
import unittest
import json
from typing import OrderedDict
from unittest import mock

from invoke.exceptions import Exit
from itertools import cycle

from .. import release
from ..libs.version import Version
from ..libs.common.gitlab import Gitlab, get_gitlab_bot_token, get_gitlab_token, CI_MODE
from ..libs.common import gitlab as glab

release.CI_MODE = True
glab.CI_MODE = True


def mocked_502_gitlab_requests(*args, **_kwargs):
    class MockResponse:
        def __init__(self, content, status_code):
            self.content = content
            self.status_code = status_code

        def json(self):
            return self.content

    return MockResponse(
        "<html>\r\n<head><title>502 Bad Gateway</title></head>\r\n<body>\r\n<center><h1>502 Bad Gateway</h1></center>\r\n</body>\r\n</html>\r\n",
        502,
    )


def mocked_gitlab_project_request(*args, **_kwargs):
    class MockResponse:
        def __init__(self, content, status_code):
            self.content = content
            self.status_code = status_code

        def json(self):
            return self.content

    return MockResponse("name", 200)


def mocked_github_requests_get(*_args, **kwargs):
    class MockResponse:
        def __init__(self, json_data, status_code):
            self.json_data = json_data
            self.status_code = status_code

        def json(self):
            return self.json_data

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


def mocked_502_github_requests(*_args, **_kwargs):
    class MockResponse:
        def __init__(self, json_data, status_code):
            self.json_data = json_data
            self.status_code = status_code

        def json(self):
            return self.json_data

    return MockResponse([], 502)


class SideEffect:
    def __init__(self, *fargs):
        self.functions = cycle(fargs)

    def __call__(self, *args, **kwargs):
        func = next(self.functions)
        return func(*args, **kwargs)


class TestStatusCode5XX(unittest.TestCase):
    @mock.patch('requests.get', side_effect=SideEffect(mocked_502_github_requests, mocked_github_requests_get))
    def test_github_one_fail_one_success(self, _):
        version = release._get_highest_repo_version(
            "FAKE_TOKEN",
            "target-repo",
            "",
            release.build_compatible_version_re(release.COMPATIBLE_MAJOR_VERSIONS[7], 29),
            release.COMPATIBLE_MAJOR_VERSIONS[7],
        )
        self.assertEqual(version, Version(major=7, minor=29, patch=0))

    @mock.patch(
        'requests.get',
        side_effect=SideEffect(
            mocked_502_github_requests,
            mocked_502_github_requests,
            mocked_502_github_requests,
            mocked_502_github_requests,
            mocked_github_requests_get,
        ),
    )
    def test_github_last_one_success(self, _):
        version = release._get_highest_repo_version(
            "FAKE_TOKEN",
            "target-repo",
            "",
            release.build_compatible_version_re(release.COMPATIBLE_MAJOR_VERSIONS[7], 29),
            release.COMPATIBLE_MAJOR_VERSIONS[7],
        )
        self.assertEqual(version, Version(major=7, minor=29, patch=0))

    @mock.patch('requests.get', side_effect=SideEffect(mocked_502_github_requests))
    def test_github_full_fail(self, _):
        failed = False
        try:
            version = release._get_highest_repo_version(
                "FAKE_TOKEN",
                "target-repo",
                "",
                release.build_compatible_version_re(release.COMPATIBLE_MAJOR_VERSIONS[7], 29),
                release.COMPATIBLE_MAJOR_VERSIONS[7],
            )
        except Exit:
            failed = True
        if not failed:
            Exit("GithubAPI was expected to fail !")

    @mock.patch('requests.get', side_effect=SideEffect(mocked_502_gitlab_requests, mocked_gitlab_project_request))
    def test_gitlab_one_fail_one_success(self, _):
        project_name = "DataDog/datadog-agent"
        gitlab = Gitlab(project_name=project_name, api_token=get_gitlab_token())
        gitlab.test_project_found()

    @mock.patch(
        'requests.get',
        side_effect=SideEffect(
            mocked_502_gitlab_requests,
            mocked_502_gitlab_requests,
            mocked_502_gitlab_requests,
            mocked_502_gitlab_requests,
            mocked_gitlab_project_request,
        ),
    )
    def test_gitlab_last_one_success(self, _):
        project_name = "DataDog/datadog-agent"
        gitlab = Gitlab(project_name=project_name, api_token=get_gitlab_token())
        gitlab.test_project_found()

    @mock.patch('requests.get', side_effect=SideEffect(mocked_502_gitlab_requests))
    def test_gitlab_full_fail(self, _):
        failed = False
        try:
            project_name = "DataDog/datadog-agent"
            gitlab = Gitlab(project_name=project_name, api_token=get_gitlab_token())
            gitlab.test_project_found()
        except Exit:
            failed = True
        if not failed:
            Exit("GitlabAPI was expected to fail")


if __name__ == "__main__":
    unittest.main()
