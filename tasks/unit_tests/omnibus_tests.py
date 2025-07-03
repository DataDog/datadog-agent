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
        'S3_OMNIBUS_GIT_CACHE_BUCKET': 'omnibus-cache',
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
                r'aws s3 cp (\S* )?s3://omnibus-cache/\w+/slug \S+/omnibus-git-cache-bundle',
                # We cloned the repo
                r'git clone --mirror /\S+/omnibus-git-cache-bundle omnibus-git-cache/opt/datadog-agent',
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
            r'git -C omnibus-git-cache/opt/datadog-agent bundle create /\S+/omnibus-git-cache-bundle --tags',
            r'aws s3 cp (\S* )?/\S+/omnibus-git-cache-bundle s3://omnibus-cache/\w+/slug',
        ]
        for line in lines:
            self.assertIsNone(re.search(line, commands))

    def test_cache_miss(self):
        self.mock_ctx.set_result_for(
            'run',
            re.compile(r'aws s3 cp (\S* )?s3://omnibus-cache/\S* /\S+/omnibus-git-cache-bundle'),
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
            r'git clone --mirror /\S+/omnibus-git-cache-bundle omnibus-git-cache/opt/datadog-agent',
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
                # And we created and uploaded the new cache
                r'git -C omnibus-git-cache/opt/datadog-agent bundle create /\S+/omnibus-git-cache-bundle --tags',
                r'aws s3 cp (\S* )?/\S+/omnibus-git-cache-bundle s3://omnibus-cache/\w+/slug',
            ],
        )

    def test_cache_hit_with_corruption(self):
        # Case where we get a bundle from S3 but git finds it to be corrupted

        # Fail to clone
        self.mock_ctx.set_result_for(
            'run',
            re.compile(r'git clone (\S* )?/\S+/omnibus-git-cache-bundle.*'),
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

    def test_mutated_cache(self):
        self.mock_ctx.set_result_for(
            'run',
            re.compile(r'git (.* )?tag -l'),
            [Result('foo-1'), Result('foo-2')],
        )
        self._set_up_default_command_mocks()
        with mock.patch('requests.post') as post_mock:
            omnibus.build(self.mock_ctx)

        # Assert we sent a cache mutation event
        assert post_mock.mock_calls
        self.assertIn("events", post_mock.mock_calls[0].args[0])
        self.assertIn("omnibus cache mutated", str(post_mock.mock_calls[0].kwargs['json']))
        # Assert we bundled and uploaded the cache (should always happen on cache misses)
        self.assertRunLines(
            [
                # We copied the cache from remote cache
                r'aws s3 cp (\S* )?s3://omnibus-cache/\w+/slug \S+/omnibus-git-cache-bundle',
                # We cloned the repo
                r'git clone --mirror /\S+/omnibus-git-cache-bundle omnibus-git-cache/opt/datadog-agent',
                # We listed the tags to get current cache state
                r'git -C omnibus-git-cache/opt/datadog-agent tag -l',
                # We ran omnibus
                r'bundle exec omnibus build agent',
            ],
        )


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


regex_match_otool = re.compile(r"otool -l some/file > .*")
regex_match_rpath = re.compile(r'cat .* \| grep -A 2 "RPATH"')
regex_match_lcload = re.compile(r'cat .* \| grep -A 2 "LC_LOAD_DYLIB"')
regex_match_lcid = re.compile(r'cat .* \| grep -A 2 "LC_ID_DYLIB"')


class TestRpathEdit(unittest.TestCase):
    def setUp(self):
        self.mock_ctx = MockContextRaising(run={})
        # Sample otool output for rpaths LC_RPATH and LC_LOAD
        self.otool_rpaths = """
        cmd LC_RPATH
        cmdsize 48
        path some/path/embedded/lib (offset 12)
        """
        self.otool_lc_loads = """
        cmd LC_LOAD_DYLIB
        cmdsize 56
        name some/path/somelib.dylib (offset 24)
        time stamp 2 Thu Jan  1 01:00:02 1970
        current version 1.0.0
        compatibility version 1.0.0
        """

    def test_rpath_edit_linux(self):
        self.mock_ctx.set_result_for(
            'run',
            r"find some/path -type f -exec file --mime-type \{\} \+",
            Result("some/file:application/x-executable"),
        )
        self.mock_ctx.set_result_for(
            'run', 'objdump -x some/file | grep "RPATH"', Result("some/path/result/binary/path")
        )
        self.mock_ctx.set_result_for(
            'run', 'patchelf --force-rpath --set-rpath \\$ORIGIN/other/path/embedded/lib some/file', Result()
        )
        omnibus.rpath_edit(self.mock_ctx, "some/path", "some/other/path")
        call_list = self.mock_ctx.run.mock_calls
        assert mock.call('find some/path -type f -exec file --mime-type \\{\\} \\+', hide=True) in call_list
        assert mock.call('objdump -x some/file | grep "RPATH"', warn=True, hide=True) in call_list
        assert mock.call('patchelf --force-rpath --set-rpath \\$ORIGIN/other/path/embedded/lib some/file') in call_list

    def test_rpath_edit_macos(self):
        self.mock_ctx.set_result_for(
            'run',
            r"find some/path -type f -exec file --mime-type \{\} \+",
            Result("some/file:application/x-mach-binary"),
        )
        self.mock_ctx.set_result_for('run', regex_match_otool, Result())
        self.mock_ctx.set_result_for('run', regex_match_rpath, Result(self.otool_rpaths))
        self.mock_ctx.set_result_for('run', regex_match_lcload, Result(self.otool_lc_loads))
        self.mock_ctx.set_result_for('run', regex_match_lcid, Result(self.otool_lc_loads))
        self.mock_ctx.set_result_for(
            'run',
            'install_name_tool -rpath some/path/embedded/lib @loader_path/other/path/embedded/lib some/file',
            Result(),
        )
        self.mock_ctx.set_result_for('run', 'install_name_tool -id some/path/somelib.dylib some/file', Result())
        self.mock_ctx.set_result_for(
            'run', 'install_name_tool -change some/path/somelib.dylib some/path/somelib.dylib some/file', Result()
        )
        omnibus.rpath_edit(self.mock_ctx, "some/path", "some/other/path", "macos")
        call_list = self.mock_ctx.run.mock_calls
        assert mock.call('find some/path -type f -exec file --mime-type \\{\\} \\+', hide=True) in call_list
        assert mock.call('install_name_tool -id some/path/somelib.dylib some/file') in call_list
        assert (
            mock.call('install_name_tool -change some/path/somelib.dylib some/path/somelib.dylib some/file')
            in call_list
        )
        assert (
            mock.call(
                'install_name_tool -rpath some/path/embedded/lib @loader_path/other/path/embedded/lib some/file',
                warn=True,
                hide=True,
            )
            in call_list
        )
        # We can't assert regex based temporary name in calls, hence we're checking that we get the correct total number of calls
        assert len(call_list) == 8


class TestBuildRepackagedAgent(unittest.TestCase):
    def test_package_parsing(self):
        # Sample Packages file content
        packages_content = """
Package: datadog-agent
Version: 1:7.67.0~devel.git.113.2750233.pipeline.63430585-1
Architecture: amd64
Filename: pool/d/da/datadog-agent_7.67.0~devel.git.113.2750233.pipeline.63430585-1_amd64.deb
SHA256: abc123def456
Description: Datadog Monitoring Agent
 The Datadog Monitoring Agent is a lightweight process that monitors system
 processes and services, and sends information back to your Datadog account.

Package: datadog-iot-agent
Version: 1:7.67.0~devel.git.113.2750233.pipeline.63430585-1
Architecture: amd64
Filename: pool/o/ot/other-package_1.0.0_amd64.deb
SHA256: 789ghi
Description: Datadog IoT Agent
 The Datadog IoT Agent is a lightweight process that monitors system
 processes and services, and sends information back to your Datadog account.

Package: datadog-agent
Version: 1:7.67.0~devel.git.113.2750233.pipeline.63448947-1
Architecture: amd64
Filename: pool/d/da/datadog-agent_7.67.0~devel.git.113.2750233.pipeline.63448947-1_amd64.deb
SHA256: def456abc789
Description: Datadog Monitoring Agent
 This is the datadog-agent package with the highest pipeline ID

Package: datadog-agent
Version: 1:7.0.0~alpha.1-1
Architecture: amd64
Filename: pool/d/da/datadog-agent_7.0.0~alpha.1-1_amd64.deb
SHA256: f8eb7c99c10c0362490f32c3bf3815193f5cf0778c2b074a208c838d09ef3181
Description: Datadog Monitoring Agent
 This package doesn't have a pipeline ID
"""
        mock_ctx = MockContextRaising(run={})
        # Set up patterns for commands that are known to run
        patterns = [
            (r'bundle .*', Result()),
            (r'git describe --tags .*', Result('7.67.0-beta.0-1-g4f19118')),
            (r'git .*', Result()),
            (r'aws s3 .*', Result()),
            (r'dpkg --print-architecture', Result('amd64')),
        ]
        for pattern, result in patterns:
            mock_ctx.set_result_for('run', re.compile(pattern), result)

        with (
            mock.patch('os.path.exists', return_value=False),
            mock.patch('tasks.omnibus.omnibus_run_task') as mock_run_task,
            mock.patch('requests.get') as mock_get,
        ):
            # Set up the mock response to return the packages content
            mock_response = mock.MagicMock()
            mock_response.__enter__.return_value.iter_lines.return_value = packages_content.splitlines()
            mock_get.return_value = mock_response

            omnibus.build_repackaged_agent(mock_ctx)

            # Verify that the URL we requested matches the architecture we set
            mock_get.assert_called_once_with(
                'https://apt.datad0g.com/dists/nightly/7/binary-amd64/Packages', stream=True, timeout=10
            )

            # Verify omnibus_run_task was called with the correct environment variables
            mock_run_task.assert_called_once()
            _, kwargs = mock_run_task.call_args
            env = kwargs['env']
            self.assertEqual(
                env['OMNIBUS_REPACKAGE_SOURCE_URL'],
                'https://apt.datad0g.com/pool/d/da/datadog-agent_7.67.0~devel.git.113.2750233.pipeline.63448947-1_amd64.deb',
            )
            self.assertEqual(env['OMNIBUS_REPACKAGE_SOURCE_SHA256'], 'def456abc789')
