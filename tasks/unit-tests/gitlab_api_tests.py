import unittest

from tasks.libs.ciproviders.gitlab_api import (
    GitlabCIDiff,
    clean_gitlab_ci_configuration,
    filter_gitlab_ci_configuration,
    generate_gitlab_full_configuration,
    read_includes,
)


class TestReadIncludes(unittest.TestCase):
    def test_with_includes(self):
        includes = []
        read_includes("tasks/unit-tests/testdata/in.yml", includes)
        self.assertEqual(len(includes), 4)

    def test_without_includes(self):
        includes = []
        read_includes("tasks/unit-tests/testdata/b.yml", includes)
        self.assertEqual(len(includes), 1)


class TestGenerateGitlabFullConfiguration(unittest.TestCase):
    def test_nominal(self):
        full_configuration = generate_gitlab_full_configuration("tasks/unit-tests/testdata/in.yml")
        with open("tasks/unit-tests/testdata/out.yml") as f:
            expected = f.read()
        self.assertEqual(full_configuration, expected)

    def test_yaml_with_reference(self):
        full_configuration = generate_gitlab_full_configuration(
            "tasks/unit-tests/testdata/ci_config_with_reference.yml"
        )
        with open("tasks/unit-tests/testdata/expected.yml") as f:
            expected = f.read()
        self.assertEqual(full_configuration, expected)


class TestGitlabYaml(unittest.TestCase):
    def make_test(self, file):
        config = generate_gitlab_full_configuration(file, return_dump=False, apply_postprocessing=True)

        self.assertDictEqual(config['target'], config['expected'])

    def test_reference(self):
        self.make_test("tasks/unit-tests/testdata/yaml_reference.yml")

    def test_extends(self):
        self.make_test("tasks/unit-tests/testdata/yaml_extends.yml")

    def test_extends_reference(self):
        self.make_test("tasks/unit-tests/testdata/yaml_extends_reference.yml")


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
