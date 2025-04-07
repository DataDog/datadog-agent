import json
import unittest
from unittest.mock import MagicMock, patch

from invoke import Context, Exit, MockContext, Result

from tasks.package import check_size


class TestCheckSize(unittest.TestCase):
    @patch.dict(
        'os.environ',
        {
            'OMNIBUS_PACKAGE_DIR': 'tasks/unit_tests/testdata/packages',
            'OMNIBUS_PACKAGE_DIR_SUSE': 'tasks/unit_tests/testdata/packages',
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch('tasks.libs.package.size.find_package', new=MagicMock(return_value='datadog-agent'))
    @patch('tasks.package.display_message')
    def test_dev_branch_ko(self, display_mock):
        flavor = 'datadog-agent'
        c = MockContext(
            run={
                'git merge-base HEAD origin/main': Result('25'),
                f"dpkg-deb --info {flavor} | grep Installed-Size | cut -d : -f 2 | xargs": Result(42),
                f"rpm -qip {flavor} | grep Size | cut -d : -f 2 | xargs": Result(141000000),
            }
        )
        with self.assertRaises(Exit):
            check_size(c, filename='tasks/unit_tests/testdata/package_sizes_real.json', dry_run=True)
        display_mock.assert_called_with(
            c,
            '12345',
            '|datadog-dogstatsd-x86_64-rpm|131.00MB|❌|141.00MB|10.00MB|0.50MB|\n|datadog-dogstatsd-x86_64-suse|131.00MB|❌|141.00MB|10.00MB|0.50MB|\n|datadog-iot-agent-x86_64-rpm|131.00MB|❌|141.00MB|10.00MB|0.50MB|\n|datadog-iot-agent-x86_64-suse|131.00MB|❌|141.00MB|10.00MB|0.50MB|\n|datadog-iot-agent-aarch64-rpm|131.00MB|❌|141.00MB|10.00MB|0.50MB|\n|datadog-agent-x86_64-rpm|1.00MB|❌|141.00MB|140.00MB|0.50MB|\n|datadog-agent-x86_64-suse|1.00MB|❌|141.00MB|140.00MB|0.50MB|\n|datadog-agent-aarch64-rpm|1.00MB|❌|141.00MB|140.00MB|0.50MB|\n',
            '|datadog-dogstatsd-amd64-deb|-9.96MB|✅|0.04MB|10.00MB|0.50MB|\n|datadog-dogstatsd-arm64-deb|-9.96MB|✅|0.04MB|10.00MB|0.50MB|\n|datadog-iot-agent-amd64-deb|-9.96MB|✅|0.04MB|10.00MB|0.50MB|\n|datadog-iot-agent-arm64-deb|-9.96MB|✅|0.04MB|10.00MB|0.50MB|\n|datadog-heroku-agent-amd64-deb|-69.96MB|✅|0.04MB|70.00MB|0.50MB|\n|datadog-agent-amd64-deb|-139.96MB|✅|0.04MB|140.00MB|0.50MB|\n|datadog-agent-arm64-deb|-139.96MB|✅|0.04MB|140.00MB|0.50MB|\n',
            '❌ Failed',
        )

    @patch('builtins.print')
    @patch.dict(
        'os.environ',
        {
            'OMNIBUS_PACKAGE_DIR': 'tasks/unit_tests/testdata/packages',
            'OMNIBUS_PACKAGE_DIR_SUSE': 'tasks/unit_tests/testdata/packages',
            'CI_COMMIT_REF_NAME': 'pikachu',
        },
    )
    @patch('tasks.libs.package.size.find_package', new=MagicMock(return_value='datadog-agent'))
    @patch('tasks.package.display_message', new=MagicMock())
    @patch('tasks.package.upload_package_sizes')
    def test_dev_branch_ok(self, upload_mock, print_mock):
        flavor = 'datadog-agent'
        c = MockContext(
            run={
                'git merge-base HEAD origin/main': Result('25'),
                f"dpkg-deb --info {flavor} | grep Installed-Size | cut -d : -f 2 | xargs": Result(42),
                f"rpm -qip {flavor} | grep Size | cut -d : -f 2 | xargs": Result(10500000),
            }
        )
        check_size(c, filename='tasks/unit_tests/testdata/package_sizes_real.json', dry_run=True)
        print_mock.assert_called()
        self.assertEqual(print_mock.call_count, 16)
        upload_mock.assert_not_called()

    @patch.dict(
        'os.environ',
        {
            'OMNIBUS_PACKAGE_DIR': 'tasks/unit_tests/testdata/packages',
            'OMNIBUS_PACKAGE_DIR_SUSE': 'tasks/unit_tests/testdata/packages',
            'CI_COMMIT_REF_NAME': 'pikachu',
        },
        clear=True,
    )
    @patch('tasks.libs.package.size.find_package', new=MagicMock(return_value='datadog-agent'))
    @patch('tasks.package.upload_package_sizes', new=MagicMock())
    @patch('tasks.package.display_message')
    def test_dev_no_pr_defined(self, display_mock):
        flavor = 'datadog-agent'
        display_mock.side_effect = AssertionError('CI_COMMIT_BRANCH')
        c = MockContext(
            run={
                'git merge-base HEAD origin/main': Result('25'),
                f"dpkg-deb --info {flavor} | grep Installed-Size | cut -d : -f 2 | xargs": Result(42),
                f"rpm -qip {flavor} | grep Size | cut -d : -f 2 | xargs": Result(10500000),
            }
        )
        check_size(c, filename='tasks/unit_tests/testdata/package_sizes_real.json', dry_run=True)
        display_mock.assert_not_called()

    @patch.dict(
        'os.environ',
        {
            'OMNIBUS_PACKAGE_DIR': 'tasks/unit_tests/testdata/packages',
            'OMNIBUS_PACKAGE_DIR_SUSE': 'tasks/unit_tests/testdata/packages',
            'CI_COMMIT_REF_NAME': 'main',
        },
    )
    @patch('tasks.libs.package.size.find_package', new=MagicMock(return_value='datadog-agent'))
    @patch('tasks.package.display_message', new=MagicMock())
    def test_main_branch_ok(self):
        flavor = 'datadog-agent'
        c = MockContext(
            run={
                'git merge-base HEAD origin/main': Result('25'),
                f"dpkg-deb --info {flavor} | grep Installed-Size | cut -d : -f 2 | xargs": Result(42),
                f"rpm -qip {flavor} | grep Size | cut -d : -f 2 | xargs": Result(20000000),
            }
        )
        check_size(c, filename='tasks/unit_tests/testdata/package_sizes_real.json', dry_run=True)
        with open('tasks/unit_tests/testdata/package_sizes_real.json') as f:
            new_sizes = json.load(f)
        self.assertIn('25', new_sizes)
        self.assertEqual(new_sizes['25']['x86_64']['datadog-agent']['rpm'], 20000000)
        self.assertEqual(new_sizes['25']['arm64']['datadog-iot-agent']['deb'], 43008)
        ctx = Context()
        ctx.run("git checkout -- tasks/unit_tests/testdata/package_sizes_real.json")
