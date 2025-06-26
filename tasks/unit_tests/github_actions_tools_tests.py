import unittest
from unittest.mock import MagicMock, call, patch

from invoke.exceptions import Exit

from tasks.libs.ciproviders.github_actions_tools import download_artifacts, download_logs, download_with_retry


class TestDownloadWithRetry(unittest.TestCase):
    def setUp(self) -> None:
        self.gh = MagicMock()
        self.gh._auth.token = "largo_winch"

    @patch('tasks.libs.ciproviders.github_actions_tools.GithubAPI')
    @patch('tasks.libs.ciproviders.github_actions_tools.zipfile.ZipFile', new=MagicMock())
    def test_download_log(self, gh_mock):
        run = MagicMock()
        gh_mock.return_value = self.gh
        download_with_retry(
            download_function=download_logs,
            run=run,
            destination='.',
            retry_count=3,
            retry_interval=1,
            repository="https://github.com/DataDog/datadog-agent",
        )

    @patch('tasks.libs.ciproviders.github_actions_tools.GithubAPI')
    @patch('tasks.libs.ciproviders.github_actions_tools.zipfile.ZipFile', new=MagicMock())
    def test_downloads_artifacts_with_repository(self, gh_mock):
        run = MagicMock()
        gh_mock.return_value = self.gh
        download_with_retry(
            download_function=download_artifacts,
            run=run,
            destination='.',
            retry_count=3,
            retry_interval=1,
            repository="https://github.com/DataDog/datadog-agent",
        )

    @patch('tasks.libs.ciproviders.github_actions_tools.GithubAPI')
    @patch('tasks.libs.ciproviders.github_actions_tools.zipfile.ZipFile', new=MagicMock())
    def test_downloads_artifacts_without_repository(self, gh_mock):
        run = MagicMock()
        gh_mock.return_value = self.gh
        download_with_retry(
            download_function=download_artifacts,
            run=run,
            destination='.',
            retry_count=3,
            retry_interval=1,
            repository="simon_ovronnaz",
        )

    @patch('builtins.print')
    @patch('tasks.libs.ciproviders.github_actions_tools.GithubAPI')
    @patch('tasks.libs.ciproviders.github_actions_tools.zipfile.ZipFile', new=MagicMock(side_effect=ConnectionError))
    def test_connection_error(self, gh_mock, print_mock):
        run = MagicMock()
        gh_mock.return_value = self.gh
        with self.assertRaises(Exit):
            download_with_retry(
                download_function=download_logs,
                run=run,
                destination='.',
                retry_count=2,
                retry_interval=1,
                repository="https://github.com/DataDog/datadog-agent",
            )
        self.assertEqual(print_mock.call_count, 5)
        print_mock.assert_has_calls([call('Connectivity issue while downloading, retrying... 0 attempts left')])

    @patch('builtins.print')
    @patch('tasks.libs.ciproviders.github_actions_tools.GithubAPI')
    @patch('tasks.libs.ciproviders.github_actions_tools.zipfile.ZipFile', new=MagicMock(side_effect=LookupError))
    def test_other_error(self, gh_mock, print_mock):
        run = MagicMock()
        gh_mock.return_value = self.gh
        with self.assertRaises(LookupError):
            download_with_retry(
                download_function=download_logs,
                run=run,
                destination='.',
                retry_count=1,
                retry_interval=1,
                repository="https://github.com/DataDog/datadog-agent",
            )
        self.assertEqual(print_mock.call_count, 2)
        self.assertTrue(print_mock.call_args_list[-1].startswith('Exception that is not a connectivity issue'))
