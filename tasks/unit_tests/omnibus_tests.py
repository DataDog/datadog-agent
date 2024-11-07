import os
import re
import unittest
from unittest import mock

from invoke.context import MockContext
from invoke.exceptions import Exit, UnexpectedExit
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
@mock.patch.dict(
    'os.environ',
    {
        'OMNIBUS_GIT_CACHE_DIR': 'omnibus-git-cache',
        'CI_JOB_NAME_SLUG': 'slug',
        'CI_COMMIT_REF_NAME': '',
        'CI_PROJECT_DIR': '',
        'CI_PIPELINE_ID': '',
        'RELEASE_VERSION_7': 'nightly',
        'S3_OMNIBUS_CACHE_BUCKET': 'omnibus-cache',
        'API_KEY_ORG2': 'api-key',
        'AGENT_API_KEY_ORG2': 'agent-api-key',
    },
    clear=True,
)
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
            (r'aws ssm .*', Result()),
            (r'vault kv get .*', Result()),
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
        self.mock_ctx.set_result_for(
            'run',
            re.compile(r'git (.* )?tag -l'),
            Result('foo-1234'),
        )
        self._set_up_default_command_mocks()
        omnibus.build(self.mock_ctx)

        # Assert main actions were taken in the expected order
        self.assertRunLines(
            [
                # We copied the cache from remote cache
                r'aws s3 cp (\S* )?s3://omnibus-cache/builds/\w+/slug /tmp/omnibus-git-cache-bundle',
                # We cloned the repo
                r'git clone --mirror /tmp/omnibus-git-cache-bundle omnibus-git-cache/opt/datadog-agent',
                # We listed the tags to get current cache state
                r'git -C omnibus-git-cache/opt/datadog-agent tag -l',
                # We ran omnibus
                r'bundle exec omnibus build agent',
            ],
        )

        # By the way the mocks are set up, we expect the `cache state` to not have changed and thus the cache
        # shouldn't have been bundled and uploaded
        commands = _run_calls_to_string(self.mock_ctx.run.mock_calls)
        lines = [
            'git -C omnibus-git-cache/opt/datadog-agent bundle create /tmp/omnibus-git-cache-bundle --tags',
            r'aws s3 cp (\S* )?/tmp/omnibus-git-cache-bundle s3://omnibus-cache/builds/\w+/slug',
        ]
        for line in lines:
            self.assertIsNone(re.search(line, commands))

    def test_cache_miss(self):
        self.mock_ctx.set_result_for(
            'run',
            re.compile(r'aws s3 cp (\S* )?s3://omnibus-cache/builds/\S* /tmp/omnibus-git-cache-bundle'),
            Result(exited=1),
        )
        self.mock_ctx.set_result_for(
            'run',
            re.compile(r'git (.* )?tag -l'),
            Result('foo-1234'),
        )
        self._set_up_default_command_mocks()
        with mock.patch('requests.post') as post_mock:
            omnibus.build(self.mock_ctx)

        commands = _run_calls_to_string(self.mock_ctx.run.mock_calls)
        commands_before_build = commands.split('bundle exec omnibus')[0]

        # Assert we did NOT clone nor list tags before the omnibus build
        lines = [
            r'git clone --mirror /tmp/omnibus-git-cache-bundle omnibus-git-cache/opt/datadog-agent',
            r'git -C omnibus-git-cache/opt/datadog-agent tag -l',
        ]
        for line in lines:
            self.assertIsNone(re.search(line, commands_before_build))
        # Assert we sent a cache miss event
        assert post_mock.mock_calls
        self.assertIn("events", post_mock.mock_calls[0].args[0])
        self.assertIn("omnibus cache miss", str(post_mock.mock_calls[0].kwargs['json']))
        # Assert we bundled and uploaded the cache (should always happen on cache misses)
        self.assertRunLines(
            [
                # We ran omnibus
                r'bundle exec omnibus build agent',
                # Listed tags for cache comparison
                r'git -C omnibus-git-cache/opt/datadog-agent tag -l',
                # And we created and uploaded the new cache
                r'git -C omnibus-git-cache/opt/datadog-agent bundle create /tmp/omnibus-git-cache-bundle --tags',
                r'aws s3 cp (\S* )?/tmp/omnibus-git-cache-bundle s3://omnibus-cache/builds/\w+/slug',
            ],
        )

    def test_cache_hit_with_corruption(self):
        # Case where we get a bundle from S3 but git finds it to be corrupted

        # Fail to clone
        self.mock_ctx.set_result_for(
            'run',
            re.compile(r'git clone (\S* )?/tmp/omnibus-git-cache-bundle.*'),
            Result('fatal: remote did not send all necessary objects', exited=1),
        )
        self._set_up_default_command_mocks()

        omnibus.build(self.mock_ctx)

        # We're satisfied if we ran the build despite that failure
        self.assertRunLines([r'bundle exec omnibus build agent'])

    def test_cache_is_disabled_by_unsetting_env_var(self):
        del os.environ['OMNIBUS_GIT_CACHE_DIR']
        self._set_up_default_command_mocks()

        omnibus.build(self.mock_ctx)

        # We ran the build but no command related to the cache
        self.assertRunLines(['bundle exec omnibus build agent'])
        commands = _run_calls_to_string(self.mock_ctx.run.mock_calls)
        self.assertNotIn('omnibus-git-cache', commands)


class TestOmnibusInstall(unittest.TestCase):
    def setUp(self):
        self.mock_ctx = MockContextRaising(run={})

    def test_success(self):
        self.mock_ctx.set_result_for('run', 'bundle install', Result())
        omnibus.bundle_install_omnibus(self.mock_ctx)
        self.assertEqual(len(self.mock_ctx.run.mock_calls), 1)

    def test_failure(self):
        self.mock_ctx.set_result_for('run', 'bundle install', Result(exited=1))
        with self.assertRaises(UnexpectedExit):
            omnibus.bundle_install_omnibus(self.mock_ctx)
        self.assertEqual(len(self.mock_ctx.run.mock_calls), 1)

    def test_transient(self):
        self.mock_ctx = MockContextRaising(run=[Result(exited=1, stderr='Net::HTTPNotFound: something'), Result()])
        omnibus.bundle_install_omnibus(self.mock_ctx)
        self.assertEqual(len(self.mock_ctx.run.mock_calls), 2)

    def test_transient_repeated(self):
        self.mock_ctx.set_result_for('run', 'bundle install', Result(exited=1, stderr='Net::HTTPNotFound: something'))
        max_try = 2
        with self.assertRaises(Exit):
            omnibus.bundle_install_omnibus(self.mock_ctx)
        self.assertEqual(len(self.mock_ctx.run.mock_calls), max_try)
