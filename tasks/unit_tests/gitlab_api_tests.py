import unittest
from collections import OrderedDict

from invoke.context import MockContext

from tasks.libs.ciproviders.gitlab_api import (
    GitlabCIDiff,
    clean_gitlab_ci_configuration,
    expand_matrix_jobs,
    filter_gitlab_ci_configuration,
    read_includes,
    retrieve_all_paths,
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


class TestGitlabCiDiff(unittest.TestCase):
    def test_make_diff(self):
        before = {
            'job1': {
                'script': [
                    'echo "hello"',
                    'echo "hello?"',
                    'echo "hello!"',
                ]
            },
            'job2': {
                'script': 'echo "world"',
            },
            'job3': {
                'script': 'echo "!"',
            },
            'job4': {
                'script': 'echo "?"',
            },
        }
        after = {
            'job1': {
                'script': [
                    'echo "hello"',
                    'echo "bonjour?"',
                    'echo "hello!"',
                ]
            },
            'job2_renamed': {
                'script': 'echo "world"',
            },
            'job3': {
                'script': 'echo "!"',
            },
            'job5': {
                'script': 'echo "???"',
            },
        }
        diff = GitlabCIDiff(before, after)
        self.assertSetEqual(diff.modified, {'job1'})
        self.assertSetEqual(set(diff.modified_diffs.keys()), {'job1'})
        self.assertSetEqual(diff.removed, {'job4'})
        self.assertSetEqual(diff.added, {'job5'})
        self.assertSetEqual(diff.renamed, {('job2', 'job2_renamed')})


class TestRetrieveAllPaths(unittest.TestCase):
    def test_all_configs(self):
        yml = {
            'stark': {'changes': ['eddard', 'catelyn', 'robb']},
            'lannister': [
                ['tywin', {'cersei': ['joffrey', 'myrcella', {'tommen': {'changes': ['casterly_rock']}}]}],
                'jaime',
                {'tyrion': {'changes': {'paths': ['hand_of_the_king']}}},
            ],
            'targaeryen': [{'daenerys': {'changes': {'compare_to': 'dragons'}}}],
        }
        paths = list(retrieve_all_paths(yml))

        expected_paths = [
            'eddard',
            'catelyn',
            'robb',
            'casterly_rock',
            'hand_of_the_king',
        ]
        self.assertListEqual(paths, expected_paths)


class TestExpandMatrixJobs(unittest.TestCase):
    def test_single(self):
        yml = {
            'job': {
                'script': 'echo hello',
                'parallel': {
                    'matrix': [
                        {
                            'VAR1': 'a',
                        },
                        {
                            'VAR1': 'b',
                        },
                    ]
                },
            }
        }
        expected_yml = {
            'job: [a]': {'script': 'echo hello', 'variables': {'VAR1': 'a'}},
            'job: [b]': {'script': 'echo hello', 'variables': {'VAR1': 'b'}},
        }

        res = expand_matrix_jobs(yml)

        self.assertDictEqual(res, expected_yml)

    def test_single2(self):
        yml = {
            'job': {
                'script': 'echo hello',
                'parallel': {
                    'matrix': [
                        # Used OrderedDict to ensure order is preserved and the name is deterministic
                        OrderedDict(
                            [
                                ('VAR1', 'a'),
                                ('VAR2', 'b'),
                            ]
                        ),
                        OrderedDict(
                            [
                                ('VAR1', 'c'),
                                ('VAR2', 'd'),
                            ]
                        ),
                    ]
                },
            }
        }
        expected_yml = {
            'job: [a, b]': {'script': 'echo hello', 'variables': {'VAR1': 'a', 'VAR2': 'b'}},
            'job: [c, d]': {'script': 'echo hello', 'variables': {'VAR1': 'c', 'VAR2': 'd'}},
        }

        res = expand_matrix_jobs(yml)

        self.assertDictEqual(res, expected_yml)

    def test_multiple(self):
        yml = {
            'job': {
                'script': 'echo hello',
                'parallel': {
                    'matrix': [
                        OrderedDict(
                            [
                                ('VAR1', ['a', 'b']),
                                ('VAR2', 'x'),
                            ]
                        )
                    ]
                },
            }
        }
        expected_yml = {
            'job: [a, x]': {'script': 'echo hello', 'variables': {'VAR1': 'a', 'VAR2': 'x'}},
            'job: [b, x]': {'script': 'echo hello', 'variables': {'VAR1': 'b', 'VAR2': 'x'}},
        }

        res = expand_matrix_jobs(yml)

        self.assertDictEqual(res, expected_yml)

    def test_multiple2(self):
        yml = {
            'job': {
                'script': 'echo hello',
                'parallel': {
                    'matrix': [
                        OrderedDict(
                            [
                                ('VAR1', ['a', 'b']),
                                ('VAR2', ['x', 'y']),
                            ]
                        )
                    ]
                },
            }
        }
        expected_yml = {
            'job: [a, x]': {'script': 'echo hello', 'variables': {'VAR1': 'a', 'VAR2': 'x'}},
            'job: [b, x]': {'script': 'echo hello', 'variables': {'VAR1': 'b', 'VAR2': 'x'}},
            'job: [a, y]': {'script': 'echo hello', 'variables': {'VAR1': 'a', 'VAR2': 'y'}},
            'job: [b, y]': {'script': 'echo hello', 'variables': {'VAR1': 'b', 'VAR2': 'y'}},
        }

        res = expand_matrix_jobs(yml)

        self.assertDictEqual(res, expected_yml)

    def test_many(self):
        yml = {
            'job': {
                'script': 'echo hello',
                'parallel': {
                    'matrix': [
                        OrderedDict(
                            [
                                ('VAR1', ['a', 'b']),
                                ('VAR2', ['x', 'y']),
                            ]
                        ),
                        OrderedDict(
                            [
                                ('VAR1', ['alpha', 'beta']),
                                ('VAR2', ['x', 'y']),
                            ]
                        ),
                    ]
                },
            }
        }
        expected_yml = {
            'job: [a, x]': {'script': 'echo hello', 'variables': {'VAR1': 'a', 'VAR2': 'x'}},
            'job: [b, x]': {'script': 'echo hello', 'variables': {'VAR1': 'b', 'VAR2': 'x'}},
            'job: [a, y]': {'script': 'echo hello', 'variables': {'VAR1': 'a', 'VAR2': 'y'}},
            'job: [b, y]': {'script': 'echo hello', 'variables': {'VAR1': 'b', 'VAR2': 'y'}},
            'job: [alpha, x]': {'script': 'echo hello', 'variables': {'VAR1': 'alpha', 'VAR2': 'x'}},
            'job: [beta, x]': {'script': 'echo hello', 'variables': {'VAR1': 'beta', 'VAR2': 'x'}},
            'job: [alpha, y]': {'script': 'echo hello', 'variables': {'VAR1': 'alpha', 'VAR2': 'y'}},
            'job: [beta, y]': {'script': 'echo hello', 'variables': {'VAR1': 'beta', 'VAR2': 'y'}},
        }

        res = expand_matrix_jobs(yml)

        self.assertDictEqual(res, expected_yml)
