import unittest

from invoke.context import MockContext

from tasks.libs.ciproviders.gitlab_api import (
    clean_gitlab_ci_configuration,
    filter_gitlab_ci_configuration,
    read_includes,
)


class TestReadIncludes(unittest.TestCase):
    def test_with_includes(self):
        includes = []
        read_includes(MockContext(), "tasks/unit_tests/testdata/in.yml", includes)
        self.assertEqual(len(includes), 4)

    def test_without_includes(self):
        includes = []
        read_includes(MockContext(), "tasks/unit_tests/testdata/b.yml", includes)
        self.assertEqual(len(includes), 1)


class TestGitlabCiConfig(unittest.TestCase):
    def test_filter(self):
        yml = {
            '.wrapper': {'before_script': 'echo "start"'},
            'job1': {'script': 'echo "hello"'},
            'job2': {'script': 'echo "world"'},
        }
        expected_yml = {
            'job1': {'script': 'echo "hello"'},
            'job2': {'script': 'echo "world"'},
        }

        res = filter_gitlab_ci_configuration(yml)

        self.assertDictEqual(res, expected_yml)

    def test_filter_job(self):
        yml = {
            '.wrapper': {'before_script': 'echo "start"'},
            'job1': {'script': 'echo "hello"'},
            'job2': {'script': 'echo "world"'},
        }
        expected_yml = {
            'job1': {'script': 'echo "hello"'},
        }

        res = filter_gitlab_ci_configuration(yml, job='job1')

        self.assertDictEqual(res, expected_yml)

    def test_clean_nop(self):
        yml = {
            'job': {'script': ['echo hello']},
        }
        expected_yml = {
            'job': {'script': ['echo hello']},
        }
        res = clean_gitlab_ci_configuration(yml)

        self.assertDictEqual(res, expected_yml)

    def test_clean_flatten_nest1(self):
        yml = {
            'job': {
                'script': [
                    [
                        'echo hello',
                        'echo world',
                    ],
                    'echo "!"',
                ]
            },
        }
        expected_yml = {
            'job': {
                'script': [
                    'echo hello',
                    'echo world',
                    'echo "!"',
                ]
            },
        }
        res = clean_gitlab_ci_configuration(yml)

        self.assertDictEqual(res, expected_yml)

    def test_clean_flatten_nest2(self):
        yml = {
            'job': {
                'script': [
                    [
                        [['echo i am nested']],
                        'echo hello',
                        'echo world',
                    ],
                    'echo "!"',
                ]
            },
        }
        expected_yml = {
            'job': {
                'script': [
                    'echo i am nested',
                    'echo hello',
                    'echo world',
                    'echo "!"',
                ]
            },
        }
        res = clean_gitlab_ci_configuration(yml)

        self.assertDictEqual(res, expected_yml)

    def test_clean_extends(self):
        yml = {
            'job': {'extends': '.mywrapper', 'script': ['echo hello']},
        }
        expected_yml = {
            'job': {'script': ['echo hello']},
        }
        res = clean_gitlab_ci_configuration(yml)

        self.assertDictEqual(res, expected_yml)
