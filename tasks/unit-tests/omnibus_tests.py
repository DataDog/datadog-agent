import re
import unittest
from unittest import mock

from invoke.context import MockContext
from invoke.exceptions import UnexpectedExit
from invoke.runners import Result

from tasks import omnibus


class MockContextRaising(MockContext):
    """A more realistic `MockContext` which raises UnexpectedExit under the right circumstances."""

    def run(self, *args, **kwargs):
        result = super().run(*args, **kwargs)
        if not (result or kwargs.get("warn")):
            raise UnexpectedExit(result)
        return result


def _run_calls_to_string(mock_calls):
    """Transform a list of calls into a newline-separated string.

    This is aimed at making it easy to make relatively complex assertions on a sequence
    of `run` commands by using just regular expressions.
    """
    commands_run = (call.args[0] for call in mock_calls)
    return '\n'.join(commands_run)


@mock.patch('sys.platform', 'linux')
class TestOmnibusCache(unittest.TestCase):
    def setUp(self):
        self.mock_ctx = MockContextRaising(run={})

    def _set_up_default_command_mocks(self):
        # This should allow to postpone the setting up of these broadly catching patterns
        # after the ones specific for a test have been set up.
        patterns = [
            (r'bundle .*', Result()),
            (r'git describe --tags .*', Result('6.0.0-beta.0-1-g4f19118')),
            (r'git .*', Result()),
            (r'aws s3 .*', Result()),
            (r'go mod .*', Result()),
            (r'grep .*', Result()),
        ]
        for pattern, result in patterns:
            self.mock_ctx.set_result_for('run', re.compile(pattern), result)

    def assertRunLines(self, line_patterns):
        """Assert the given line patterns appear in the given order in `msg`."""
        commands = _run_calls_to_string(self.mock_ctx.run.mock_calls)

        pattern = '(\n|.)*'.join(line_patterns)
        return self.assertIsNotNone(
            re.search(pattern, commands, re.MULTILINE),
            f'Failed to match pattern {line_patterns}.',
        )

    def test_successful_cache_hit(self):
        self._set_up_default_command_mocks()
        env_override = {
            'OMNIBUS_GIT_CACHE_DIR': 'omnibus-git-cache',
            'CI_JOB_NAME_SLUG': 'slug',
            'CI_COMMIT_REF_NAME': '',
            'CI_PROJECT_DIR': '',
            'CI_PIPELINE_ID': '',
            'RELEASE_VERSION_7': 'nightly',
            'S3_OMNIBUS_CACHE_BUCKET': 'omnibus-cache',
        }
        with mock.patch.dict('os.environ', env_override):
            omnibus.build(self.mock_ctx)

        self.assertRunLines(
            [
                # We copied the cache from remote cache
                r'aws s3 cp (.* )?s3://omnibus-cache/builds/\w+/slug /tmp/omnibus-git-cache-bundle',
                # We cloned the repo
                r'git clone --mirror /tmp/omnibus-git-cache-bundle omnibus-git-cache/opt/datadog-agent',
                # We listed the tags to get current cache state
                r'git -C omnibus-git-cache/opt/datadog-agent tag -l',
                # We ran omnibus
                r'bundle exec omnibus build agent',
            ],
        )
