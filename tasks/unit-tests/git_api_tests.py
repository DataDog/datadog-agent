import unittest
from itertools import cycle
from unittest import mock

from invoke.exceptions import Exit

from ..libs.common.gitlab import Gitlab, get_gitlab_token
from ..libs.common.remote_api import APIError


class MockResponse:
    def __init__(self, content, status_code):
        self.content = content
        self.status_code = status_code

    def json(self):
        return self.content


#################### FAIL REQUEST  #####################


def fail_not_found_request(*_args, **_kwargs):
    return MockResponse([], 404)


##################### MOCKED GITLAB #####################


def mocked_502_gitlab_requests(*_args, **_kwargs):
    return MockResponse(
        "<html>\r\n<head><title>502 Bad Gateway</title></head>\r\n<body>\r\n<center><h1>502 Bad Gateway</h1></center>\r\n</body>\r\n</html>\r\n",
        502,
    )


def mocked_gitlab_project_request(*_args, **_kwargs):
    return MockResponse("name", 200)


class SideEffect:
    def __init__(self, *fargs):
        self.functions = cycle(fargs)

    def __call__(self, *args, **kwargs):
        func = next(self.functions)
        return func(*args, **kwargs)


class TestStatusCode5XX(unittest.TestCase):
    @mock.patch('requests.get', side_effect=SideEffect(mocked_502_gitlab_requests, mocked_gitlab_project_request))
    def test_gitlab_one_fail_one_success(self, _):
        project_name = "DataDog/datadog-agent"
        gitlab = Gitlab(project_name=project_name, api_token=get_gitlab_token())
        gitlab.requests_sleep_time = 0
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
        gitlab.requests_sleep_time = 0
        gitlab.test_project_found()

    @mock.patch('requests.get', side_effect=SideEffect(mocked_502_gitlab_requests))
    def test_gitlab_full_fail(self, _):
        failed = False
        try:
            project_name = "DataDog/datadog-agent"
            gitlab = Gitlab(project_name=project_name, api_token=get_gitlab_token())
            gitlab.requests_sleep_time = 0
            gitlab.test_project_found()
        except Exit:
            failed = True
        if not failed:
            Exit("GitlabAPI was expected to fail")

    @mock.patch('requests.get', side_effect=SideEffect(fail_not_found_request, mocked_gitlab_project_request))
    def test_gitlab_real_fail(self, _):
        failed = False
        try:
            project_name = "DataDog/datadog-agent"
            gitlab = Gitlab(project_name=project_name, api_token=get_gitlab_token())
            gitlab.requests_sleep_time = 0
            gitlab.test_project_found()
        except APIError:
            failed = True
        if not failed:
            Exit("GitlabAPI was expected to fail")


if __name__ == "__main__":
    unittest.main()
