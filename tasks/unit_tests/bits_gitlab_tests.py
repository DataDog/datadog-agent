"""Exercise the real SDK with captured public GitLab responses."""

import json
import os
import unittest
from pathlib import Path
from unittest.mock import patch
from urllib.parse import parse_qs, urlsplit

import gitlab
import requests

from tasks.libs.ciproviders.gitlab_api import get_gitlab_api, get_gitlab_repo

FIXTURES = Path(__file__).parent / "testdata" / "bits_gitlab"
PROXY = "https://bits-proxy.example:8081"
PROJECT = "gitlab-org/cli"


class CapturedGitlabAdapter(requests.adapters.BaseAdapter):
    # Fixture captured via record-external-calls skill on 2026-10-14 from
    # https://gitlab.com/api/v4/projects/gitlab-org%2Fcli and /pipelines?per_page=1.
    # The Python SDK runs outside code-gen-api, so capture used public HTTPS
    # directly; the staging internal GitLab integration is not configured.
    def __init__(self, status=200, redirect=None):
        self.calls = []
        self.status = status
        self.redirect = redirect

    def send(self, request, **kwargs):
        self.calls.append((request, kwargs))
        response = requests.Response()
        response.request = request
        response.url = request.url
        response.status_code = self.status
        response.headers["Content-Type"] = "application/json"
        if self.redirect:
            response.headers["Location"] = self.redirect
            response._content = b""
        elif self.status != 200:
            # Transport-only denial: the SDK's status handling does not need a payload.
            response._content = b""
        elif urlsplit(request.url).path.endswith("/pipelines"):
            response._content = (FIXTURES / "pipelines.json").read_bytes()
            if "page" not in parse_qs(urlsplit(request.url).query):
                response.headers.update(json.loads((FIXTURES / "pipelines-headers.json").read_text()))
        else:
            response._content = (FIXTURES / "project.json").read_bytes()
        return response

    def close(self):
        pass


@patch.dict(os.environ, {"DD_BITS_CODE": "1", "GITLAB_HOST": PROXY}, clear=True)
class BitsGitlabTests(unittest.TestCase):
    def test_project_and_paginated_subresources_use_proxy_without_credentials(self):
        # Tokens, CI mode, .netrc and HTTP_PROXY must not change this client.
        with (
            patch.dict(
                os.environ,
                {
                    "GITLAB_TOKEN": "unused-test-token",
                    "CI_JOB_TOKEN": "unused-job-token",
                    "GITLAB_CI": "true",
                    "HTTPS_PROXY": "http://unused-proxy.invalid",
                    "REQUESTS_CA_BUNDLE": "/run/public-scm-ca.pem",
                },
            ),
            patch("requests.sessions.get_netrc_auth") as netrc,
        ):
            api = get_gitlab_api(token="unused-explicit-token")
            adapter = CapturedGitlabAdapter()
            api.session.mount("https://", adapter)
            with patch("tasks.libs.ciproviders.gitlab_api.get_gitlab_api", return_value=api):
                project = get_gitlab_repo(PROJECT)
            self.assertEqual(project.id, json.loads((FIXTURES / "project.json").read_text())["id"])
            pipelines = list(project.pipelines.list(iterator=True, per_page=1))
            self.assertEqual(len(pipelines), 2)
            self.assertEqual(len(adapter.calls), 3)
            expected_path = "/api/v4/projects/gitlab-org%2Fcli"
            for request, options in adapter.calls:
                self.assertEqual(urlsplit(request.url).netloc, urlsplit(PROXY).netloc)
                self.assertTrue(urlsplit(request.url).path.startswith(expected_path))
                self.assertFalse({"Authorization", "PRIVATE-TOKEN", "JOB-TOKEN"} & set(request.headers))
                self.assertEqual(options["verify"], "/run/public-scm-ca.pem")
                self.assertFalse(options["proxies"])
            self.assertEqual(parse_qs(urlsplit(adapter.calls[-1][0].url).query)["page"], ["2"])
            netrc.assert_not_called()

    def test_unknown_numeric_project_is_not_rewritten_to_authorized_project(self):
        api = get_gitlab_api()
        api.register_project(123, PROJECT)
        self.assertEqual(api._build_url("/projects/124/pipelines"), f"{PROXY}/api/v4/projects/124/pipelines")

    def test_denied_request_keeps_sdk_error_and_does_not_bootstrap_auth(self):
        api = get_gitlab_api()
        adapter = CapturedGitlabAdapter(status=403)
        api.session.mount("https://", adapter)
        with patch("subprocess.Popen") as popen, patch("builtins.input") as prompt:
            with self.assertRaises(gitlab.exceptions.GitlabGetError) as error:
                api.projects.get(PROJECT)
            self.assertEqual(error.exception.response_code, 403)
            self.assertEqual(len(adapter.calls), 1)
            popen.assert_not_called()
            prompt.assert_not_called()

    def test_redirect_cannot_leave_proxy(self):
        api = get_gitlab_api()
        adapter = CapturedGitlabAdapter(status=302, redirect="https://gitlab.example/api/v4/projects/123")
        api.session.mount("https://", adapter)
        with self.assertRaises(requests.exceptions.TooManyRedirects):
            api.projects.get(PROJECT)
        self.assertEqual(len(adapter.calls), 1)

    def test_absolute_api_url_cannot_leave_proxy(self):
        api = get_gitlab_api()
        with self.assertRaisesRegex(ValueError, "configured Bits Code endpoint"):
            api.http_get("https://gitlab.example/api/v4/projects/123")

    def test_certificate_fallback_and_default_verification(self):
        with patch.dict(os.environ, {"SSL_CERT_FILE": "/run/public-ca.pem"}):
            self.assertEqual(get_gitlab_api().ssl_verify, "/run/public-ca.pem")
        self.assertIs(get_gitlab_api().ssl_verify, True)

    def test_host_without_scheme_uses_https(self):
        with patch.dict(os.environ, {"GITLAB_HOST": "bits-proxy.example:8081"}):
            self.assertEqual(get_gitlab_api().url, PROXY)

    def test_default_host_does_not_attempt_credential_bootstrap(self):
        with patch.dict(os.environ, {"GITLAB_HOST": ""}):
            self.assertEqual(get_gitlab_api().url, "https://gitlab.ddbuild.io")

    def test_invalid_host_settings_are_rejected(self):
        for host in (
            "http://proxy.example",
            "https://user:pass@proxy.example",
            "https://proxy.example/path",
            "https://proxy.example?x=1",
            "https://proxy.example#fragment",
        ):
            with self.subTest(host=host), patch.dict(os.environ, {"GITLAB_HOST": host}):
                with self.assertRaises(ValueError):
                    get_gitlab_api()
