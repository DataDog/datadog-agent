"""Auth policy tests: no external response fixtures or live credentials needed."""

import os
import unittest
from unittest.mock import MagicMock, patch

from invoke import Result, UnexpectedExit

from tasks.auth import gitlab
from tasks.libs.ciproviders.github_api import GithubAPI, generate_local_github_token
from tasks.libs.ciproviders.gitlab_api import get_gitlab_api, get_gitlab_oauth_token, get_gitlab_token
from tasks.libs.common.auth import datadog_infra_token, dd_auth_api_app_keys, get_aws_vault_env
from tasks.libs.common.bits_code import running_in_bits_code
from tasks.libs.releasing.version import query_version


@patch.dict(os.environ, {"DD_BITS_CODE": "1"}, clear=True)
class BitsCodeAuthTests(unittest.TestCase):
    def test_auth_helpers_never_start_bootstrap(self):
        ctx = MagicMock()
        operations = [
            lambda: generate_local_github_token(ctx),
            lambda: get_gitlab_oauth_token(ctx),
            lambda: get_gitlab_token(ctx),
            lambda: gitlab.body(ctx),
            lambda: datadog_infra_token(ctx, "sdm"),
            lambda: get_aws_vault_env(ctx, "test"),
            lambda: dd_auth_api_app_keys(ctx, "example.invalid").__enter__(),
        ]
        with patch("subprocess.Popen") as popen, patch("builtins.input") as prompt:
            for operation in operations:
                with self.subTest(operation=operation):
                    with self.assertRaisesRegex(RuntimeError, "host coding agent's authenticated operation"):
                        operation()
            ctx.run.assert_not_called()
            popen.assert_not_called()
            prompt.assert_not_called()

    def test_gitlab_factory_skips_feature_flags_and_credentials(self):
        with (
            patch("tasks.libs.ciproviders.gitlab_api.is_enabled") as feature_flag,
            patch("tasks.libs.ciproviders.gitlab_api.get_gitlab_token") as token_helper,
            patch("tasks.libs.ciproviders.gitlab_api.get_gitlab_oauth_token") as oauth_helper,
            patch("subprocess.Popen") as popen,
            patch("builtins.input") as prompt,
        ):
            for token in (None, "explicit-test-token"):
                with self.subTest(token=token):
                    client = get_gitlab_api(token=token)
                    self.assertIsNone(client.private_token)
                    self.assertIsNone(client.oauth_token)
                    self.assertIsNone(client.job_token)
            for dependency in (feature_flag, token_helper, oauth_helper, popen, prompt):
                dependency.assert_not_called()

    def test_private_github_fails_before_client_or_token_generation(self):
        with (
            patch("tasks.libs.ciproviders.github_api.generate_local_github_token") as token,
            patch("tasks.libs.ciproviders.github_api.Github") as github,
        ):
            with self.assertRaisesRegex(RuntimeError, "Authenticated GitHub API access"):
                GithubAPI()
            token.assert_not_called()
            github.assert_not_called()

    def test_public_github_uses_supported_unauthenticated_client(self):
        with (
            patch("tasks.libs.ciproviders.github_api.generate_local_github_token") as token,
            patch("tasks.libs.ciproviders.github_api.Github") as github,
        ):
            GithubAPI(public_repo=True)
            github.assert_called_once_with(auth=None, per_page=100)
            token.assert_not_called()

    def test_ci_or_token_variables_do_not_override_bits_policy(self):
        with (
            patch.dict(os.environ, {"GITLAB_CI": "true", "GITHUB_TOKEN": "", "GITLAB_TOKEN": ""}),
            patch("tasks.libs.ciproviders.github_api.Github") as github,
            patch("tasks.libs.ciproviders.gitlab_api.is_enabled") as feature_flag,
        ):
            ctx = MagicMock()
            with self.assertRaisesRegex(RuntimeError, "Authenticated GitHub API access"):
                GithubAPI()
            with self.assertRaisesRegex(RuntimeError, "Authenticated GitLab API access"):
                get_gitlab_token(ctx)
            with self.assertRaisesRegex(RuntimeError, "Datadog infrastructure authentication"):
                datadog_infra_token(ctx, "sdm")
            ctx.run.assert_not_called()
            feature_flag.assert_not_called()
            github.assert_not_called()
            GithubAPI(public_repo=True)
            github.assert_called_once_with(auth=None, per_page=100)

    def test_version_uses_local_tags_without_github(self):
        ctx = MagicMock()
        ctx.run.return_value = Result("7.54.0-devel-12-g315e3a2")
        with patch("tasks.libs.releasing.version.GithubAPI") as github:
            self.assertEqual(query_version(ctx, "7")[:4], ("7.54.0", "devel", 12, "315e3a2"))
            github.assert_not_called()
            ctx.run.assert_called_once()
            self.assertIn("git describe --tags", ctx.run.call_args.args[0])

    def test_missing_tags_do_not_fabricate_version_or_call_github(self):
        ctx = MagicMock()
        ctx.run.side_effect = UnexpectedExit(Result(exited=128))
        with patch("tasks.libs.releasing.version.GithubAPI") as github:
            with self.assertRaisesRegex(RuntimeError, "requires reachable release tags"):
                query_version(ctx, "7")
            github.assert_not_called()
            ctx.run.assert_called_once()


@patch.dict(os.environ, {}, clear=True)
class RegularAuthTests(unittest.TestCase):
    def test_marker_is_explicit(self):
        for marker in ("", "0", "true"):
            with patch.dict(os.environ, {"DD_BITS_CODE": marker}):
                self.assertFalse(running_in_bits_code())

    def test_regular_gitlab_factory_preserves_explicit_token(self):
        with (
            patch("tasks.libs.ciproviders.gitlab_api.is_enabled", return_value=True) as feature_flag,
            patch("tasks.libs.ciproviders.gitlab_api.get_gitlab_oauth_token") as oauth_helper,
            patch("tasks.libs.ciproviders.gitlab_api.gitlab.Gitlab") as client,
        ):
            self.assertIs(get_gitlab_api(token="explicit-test-token"), client.return_value)
            feature_flag.assert_called_once()
            oauth_helper.assert_not_called()
            client.assert_called_once_with(
                "https://gitlab.ddbuild.io", oauth_token="explicit-test-token", retry_transient_errors=True
            )

    def test_regular_github_still_uses_local_auth(self):
        with (
            patch("tasks.libs.ciproviders.github_api.generate_local_github_token", return_value="gh-test") as token,
            patch("tasks.libs.ciproviders.github_api.Github"),
        ):
            api = GithubAPI(public_repo=True)
            self.assertEqual(api._auth.token, "gh-test")
            token.assert_called_once()

    def test_regular_infra_keeps_ddtool_command(self):
        ctx = MagicMock()
        ctx.run.return_value = Result("Authorization: Bearer test")
        self.assertEqual(datadog_infra_token(ctx, "sdm"), "Bearer test")
        ctx.run.assert_called_once_with('ddtool auth token sdm --datacenter "us1.ddbuild.io" --http-header', hide=True)
