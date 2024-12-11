import unittest
from collections import OrderedDict
from unittest.mock import MagicMock, patch

import yaml
from invoke import MockContext, Result

from tasks.libs.ciproviders.gitlab_api import (
    GitlabCIDiff,
    MultiGitlabCIDiff,
    ReferenceTag,
    clean_gitlab_ci_configuration,
    expand_matrix_jobs,
    filter_gitlab_ci_configuration,
    find_buildimages,
    gitlab_configuration_is_modified,
    read_includes,
    retrieve_all_paths,
    update_gitlab_config,
    update_image_tag,
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
        diff = GitlabCIDiff.from_contents(before, after)
        self.assertSetEqual(diff.modified, {'job1'})
        self.assertSetEqual(set(diff.modified_diffs.keys()), {'job1'})
        self.assertSetEqual(diff.removed, {'job4'})
        self.assertSetEqual(diff.added, {'job5'})
        self.assertSetEqual(diff.renamed, {('job2', 'job2_renamed')})

    def test_serialization(self):
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
        diff = MultiGitlabCIDiff.from_contents({'file': before}, {'file': after})
        dict_diff = diff.to_dict()
        diff_from_dict = MultiGitlabCIDiff.from_dict(dict_diff)

        self.assertDictEqual(diff_from_dict.before, diff.before)


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


class TestGitlabConfigurationIsModified(unittest.TestCase):
    @patch("tasks.libs.ciproviders.gitlab_api.get_current_branch", new=MagicMock(return_value="main"))
    def test_needs_one_line(self):
        file = "tasks/unit_tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..e9a74219ba 100644\n--- a/{file}\n+++ b/{file}\n@@ -4,7 +4,7 @@\n \n .linux_tests:\n   stage: source_test\n-  needs: ["go_deps", "go_tools_deps"]\n+  needs: ["go_deps", "go_tools_deps", "new"]\n   rules:\n     - !reference [.except_disable_unit_tests]\n     - !reference [.fast_on_dev_branch_only]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertTrue(gitlab_configuration_is_modified(c))

    @patch("tasks.libs.ciproviders.gitlab_api.get_current_branch", new=MagicMock(return_value="main"))
    def test_reference_one_line(self):
        file = "tasks/unit_tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..e9a74219ba 100644\n--- a/{file}\n+++ b/{file}\n@@ -4,7 +4,7 @@\n \n .linux_tests:\n   stage: source_test\n  needs: ["go_deps", "go_tools_deps"]\n   rules:\n+     - !reference [.adding_new_reference]\n     - !reference [.except_disable_unit_tests]\n     - !reference [.fast_on_dev_branch_only]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertTrue(gitlab_configuration_is_modified(c))

    @patch("tasks.libs.ciproviders.gitlab_api.get_current_branch", new=MagicMock(return_value="main"))
    def test_needs_removed(self):
        file = "tasks/unit_tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..e9a74219ba 100644\n--- a/{file}\n+++ b/{file}\n@@ -4,6 +4,6 @@\n \n .linux_tests:\n   stage: source_test\n-  needs: ["go_deps", "go_tools_deps"]\n   rules:\n     - !reference [.except_disable_unit_tests]\n     - !reference [.fast_on_dev_branch_only]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertFalse(gitlab_configuration_is_modified(c))

    @patch("tasks.libs.ciproviders.gitlab_api.get_current_branch", new=MagicMock(return_value="main"))
    def test_artifacts_modified_and_needs_above(self):
        file = "tasks/unit_tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..e9a74219ba 100644\n--- a/{file}\n+++ b/{file}\n@@ -12,6 +12,6 @@\n \n  artifacts:\n    expire_in: 2 years\n-    when: always\n+    when: never\n    paths:\n      - none-shall-pass.txt'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertFalse(gitlab_configuration_is_modified(c))

    @patch("tasks.libs.ciproviders.gitlab_api.get_current_branch", new=MagicMock(return_value="main"))
    def test_needs_multiple_lines(self):
        file = "tasks/unit_tests/testdata/yaml_configurations/needs_several_lines.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..e9a74219ba 100644\n--- a/{file}\n+++ b/{file}\n@@ -6,7 +6,7 @@\n \n    - go_tools_deps\n    - go_go_dancer\n    - go_go_ackman\n+    - go_nagai\n   rules:\n     - !reference [.except_disable_unit_tests]\n     - !reference [.fast_on_dev_branch_only]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertTrue(gitlab_configuration_is_modified(c))

    @patch("tasks.libs.ciproviders.gitlab_api.get_current_branch", new=MagicMock(return_value="main"))
    def test_not_a_needs_multiple_lines(self):
        file = "tasks/unit_tests/testdata/yaml_configurations/no_needs_several_lines.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..e9a74219ba 100644\n--- a/{file}\n+++ b/{file}\n@@ -6,7 +6,7 @@\n \n    - go_tools_deps\n    - go_go_dancer\n    - go_go_ackman\n+    - go_nagai\n   rules:\n     - !reference [.except_disable_unit_tests]\n     - !reference [.fast_on_dev_branch_only]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertFalse(gitlab_configuration_is_modified(c))

    @patch("tasks.libs.ciproviders.gitlab_api.get_current_branch", new=MagicMock(return_value="main"))
    def test_new_reference(self):
        file = "tasks/unit_tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..5e43218090 100644\n--- a/{file}\n+++ b/{file}\n@@ -1,4 +1,11 @@\n ---\n+.rtloader_tests:\n+  stage: source_test\n+  noods: ["go_deps"]\n+  before_script:\n+    - source /root/.bashrc && conda activate $CONDA_ENV\n+  script: ["# Skipping go tests"]\n+\n nerd_tests\n   stage: source_test\n   needs: ["go_deps"]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertFalse(gitlab_configuration_is_modified(c))

    @patch("tasks.libs.ciproviders.gitlab_api.get_current_branch", new=MagicMock(return_value="main"))
    def test_new_reference_with_needs(self):
        file = "tasks/unit_tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..5e43218090 100644\n--- a/{file}\n+++ b/{file}\n@@ -1,4 +1,11 @@\n ---\n+.rtloader_tests:\n+  stage: source_test\n+  needs: ["go_deps"]\n+  before_script:\n+    - source /root/.bashrc && conda activate $CONDA_ENV\n+  script: ["# Skipping go tests"]\n+\n nerd_tests\n   stage: source_test\n   needs: ["go_deps"]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertTrue(gitlab_configuration_is_modified(c))

    @patch("tasks.libs.ciproviders.gitlab_api.get_current_branch", new=MagicMock(return_value="main"))
    def test_new_reference_with_dependencies(self):
        file = "tasks/unit_tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..5e43218090 100644\n--- a/{file}\n+++ b/{file}\n@@ -1,4 +1,11 @@\n ---\n+.rtloader_tests:\n+  stage: source_test\n+  dependencies: ["go_deps"]\n+  before_script:\n+    - source /root/.bashrc && conda activate $CONDA_ENV\n+  script: ["# Skipping go tests"]\n+\n nerd_tests\n   stage: source_test\n   needs: ["go_deps"]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertTrue(gitlab_configuration_is_modified(c))

    @patch("tasks.libs.ciproviders.gitlab_api.get_current_branch", new=MagicMock(return_value="main"))
    def test_new_job(self):
        file = "tasks/unit_tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..5e43218090 100644\n--- a/{file}\n+++ b/{file}\n@@ -1,4 +1,11 @@\n ---\n+rtloader_tests:\n+  stage: source_test\n+  noods: ["go_deps"]\n+  before_script:\n+    - source /root/.bashrc && conda activate $CONDA_ENV\n+  script: ["# Skipping go tests"]\n+\n nerd_tests\n   stage: source_test\n   needs: ["go_deps"]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertTrue(gitlab_configuration_is_modified(c))

    @patch("tasks.libs.ciproviders.gitlab_api.get_current_branch", new=MagicMock(return_value="main"))
    def test_ignored_file(self):
        file = "tasks/unit_tests/testdata/d.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..5e43218090 100644\n--- a/{file}\n+++ b/{file}\n@@ -1,4 +1,11 @@\n ---\n+rtloader_tests:\n+  stage: source_test\n+  needs: ["go_deps"]\n+  before_script:\n+    - source /root/.bashrc && conda activate $CONDA_ENV\n+  script: ["# Skipping go tests"]\n+\n nerd_tests\n   stage: source_test\n   needs: ["go_deps"]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertFalse(gitlab_configuration_is_modified(c))

    @patch("tasks.libs.ciproviders.gitlab_api.get_current_branch", new=MagicMock(return_value="main"))
    def test_two_modified_files(self):
        file = "tasks/unit_tests/testdata/d.yml"
        yaml = "tasks/unit_tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..5e43218090 100644\n--- a/{file}\n+++ b/{file}\n@@ -1,4 +1,11 @@\n ---\n+rtloader_tests:\n+  stage: source_test\n+  needs: ["go_deps"]\n+  before_script:\n+    - source /root/.bashrc && conda activate $CONDA_ENV\n+  script: ["# Skipping go tests"]\n+\n nerd_tests\n   stage: source_test\n   needs: ["go_deps"]\ndiff --git a/{yaml} b/{yaml}\nindex 561eb1a201..5e43218090 100644\n--- a/{yaml}\n+++ b/{yaml}\n@@ -1,4 +1,11 @@\n ---\n+rtloader_tests:\n+  stage: source_test\n+  noods: ["go_deps"]\n+  before_script:\n+    - source /root/.bashrc && conda activate $CONDA_ENV\n+  script: ["# Skipping go tests"]\n+\n nerd_tests\n   stage: source_test\n   needs: ["go_deps"]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertTrue(gitlab_configuration_is_modified(c))


class TestFilterVariables(unittest.TestCase):
    def test_no_images(self):
        variables = {
            'DATADOG_AGENT_BUILDIMAGES_SUFFIX': '',
            'DATADOG_AGENT_BUILDIMAGES': 'haddock',
            'DATADOG_AGENT_SYSPROBE_BUILDIMAGES_SUFFIX': '',
            'DATADOG_AGENT_SYSPROBE_BUILDIMAGES': 'v46542806-c7a4a6be',
            'CI_IMAGE_AGENT': 'tintin',
            'CI_IMAGE_AGENT_SUFFIX': '',
            'OTHER_VARIABLE_SUFFIX': '',
            'OTHER_VARIABLE': 'lampion',
        }
        self.assertEqual(
            list(find_buildimages(variables)), ['DATADOG_AGENT_BUILDIMAGES', 'DATADOG_AGENT_SYSPROBE_BUILDIMAGES']
        )

    def test_one_image(self):
        variables = {
            'DATADOG_AGENT_BUILDIMAGES_SUFFIX': '',
            'DATADOG_AGENT_BUILDIMAGES': 'haddock',
            'CI_IMAGE_AGENT': 'tintin',
            'CI_IMAGE_AGENT_SUFFIX': '',
            'CI_IMAGE_OWNER': 'tournesol',
            'CI_IMAGE_OWNER_SUFFIX': '',
            'OTHER_VARIABLE_SUFFIX': '',
            'OTHER_VARIABLE': 'lampion',
        }
        self.assertEqual(list(find_buildimages(variables, "agent", "CI_IMAGE")), ['CI_IMAGE_AGENT'])
        self.assertEqual(list(find_buildimages(variables, "AGENT", "CI_IMAGE")), ['CI_IMAGE_AGENT'])

    def test_multi_match(self):
        variables = {
            'DATADOG_AGENT_BUILDIMAGES_SUFFIX': '',
            'DATADOG_AGENT_BUILDIMAGES': 'haddock',
            'CI_IMAGE_AGENT': 'tintin',
            'CI_IMAGE_AGENT_SUFFIX': '',
            'CI_IMAGE_AGENT_42': 'tournesol',
            'CI_IMAGE_AGENT_42_SUFFIX': '',
        }
        self.assertEqual(
            list(find_buildimages(variables, "agent", "CI_IMAGE")), ['CI_IMAGE_AGENT', 'CI_IMAGE_AGENT_42']
        )
        self.assertEqual(
            list(find_buildimages(variables, "AGENT", "CI_IMAGE")), ['CI_IMAGE_AGENT', 'CI_IMAGE_AGENT_42']
        )

    def test_all_images(self):
        variables = {
            'DATADOG_AGENT_BUILDIMAGES_SUFFIX': '',
            'DATADOG_AGENT_BUILDIMAGES': 'haddock',
            'CI_IMAGE_AGENT': 'tintin',
            'CI_IMAGE_AGENT_SUFFIX': '',
            'CI_IMAGE_AGENT_42': 'tournesol',
            'CI_IMAGE_AGENT_42_SUFFIX': '',
            'CI_IMAGE_OWNER': 'tapioca',
            'CI_IMAGE_OWNER_SUFFIX': '',
        }
        self.assertEqual(
            list(find_buildimages(variables, "", "CI_IMAGE")), ['CI_IMAGE_AGENT', 'CI_IMAGE_AGENT_42', 'CI_IMAGE_OWNER']
        )
        self.assertEqual(
            list(find_buildimages(variables, "", "CI_IMAGE")), ['CI_IMAGE_AGENT', 'CI_IMAGE_AGENT_42', 'CI_IMAGE_OWNER']
        )


class TestModifyContent(unittest.TestCase):
    gitlab_ci = None

    def setUp(self) -> None:
        with open("tasks/unit_tests/testdata/variables.yml") as gl:
            self.gitlab_ci = gl.readlines()
        return super().setUp()

    def test_all_buildimages(self):
        prefix = 'DATADOG_AGENT_'
        images = [
            'DATADOG_AGENT_BUILDIMAGES',
            'DATADOG_AGENT_WINBUILDIMAGES',
            'DATADOG_AGENT_ARMBUILDIMAGES',
            'DATADOG_AGENT_SYSPROBE_BUILDIMAGES',
            'DATADOG_AGENT_BTF_GEN_BUILDIMAGES',
        ]
        modified = update_image_tag(self.gitlab_ci, "1mageV3rsi0n", images)
        yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
        config = yaml.safe_load("".join(modified))
        self.assertEqual(
            5, sum(1 for k, v in config["variables"].items() if k.startswith(prefix) and v == "_test_only")
        )
        self.assertEqual(
            5, sum(1 for k, v in config["variables"].items() if k.startswith(prefix) and v == "1mageV3rsi0n")
        )

    def test_one_buildimage(self):
        prefix = 'DATADOG_AGENT_'
        images = ['DATADOG_AGENT_BTF_GEN_BUILDIMAGES']
        modified = update_image_tag(self.gitlab_ci, "1mageV3rsi0n", images)
        yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
        config = yaml.safe_load("".join(modified))
        self.assertEqual(
            1, sum(1 for k, v in config["variables"].items() if k.startswith(prefix) and v == "_test_only")
        )
        self.assertEqual(
            1, sum(1 for k, v in config["variables"].items() if k.startswith(prefix) and v == "1mageV3rsi0n")
        )

    def test_one_image(self):
        prefix = "CI_IMAGE_"
        images = ['CI_IMAGE_DEB_X64']
        modified = update_image_tag(self.gitlab_ci, "1mageV3rsi0n", images)
        yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
        config = yaml.safe_load("".join(modified))
        self.assertEqual(
            1, sum(1 for k, v in config["variables"].items() if k.startswith(prefix) and v == "_test_only")
        )
        self.assertEqual(
            1, sum(1 for k, v in config["variables"].items() if k.startswith(prefix) and v == "1mageV3rsi0n")
        )

    def test_several_images(self):
        prefix = "CI_IMAGE_"
        images = ['CI_IMAGE_DEB_X64', 'CI_IMAGE_RPM_ARMHF']
        modified = update_image_tag(self.gitlab_ci, "1mageV3rsi0n", images)
        yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
        config = yaml.safe_load("".join(modified))
        self.assertEqual(
            2, sum(1 for k, v in config["variables"].items() if k.startswith(prefix) and v == "_test_only")
        )
        self.assertEqual(
            2, sum(1 for k, v in config["variables"].items() if k.startswith(prefix) and v == "1mageV3rsi0n")
        )

    def test_multimatch(self):
        prefix = "CI_IMAGE_"
        images = ['X64']
        modified = update_image_tag(self.gitlab_ci, "1mageV3rsi0n", images)
        yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
        config = yaml.safe_load("".join(modified))
        self.assertEqual(
            7, sum(1 for k, v in config["variables"].items() if k.startswith(prefix) and v == "_test_only")
        )
        self.assertEqual(
            7, sum(1 for k, v in config["variables"].items() if k.startswith(prefix) and v == "1mageV3rsi0n")
        )

    def test_update_no_test(self):
        prefix = "CI_IMAGE_"
        images = [
            'GITLAB_AGENT_DEPLOY',
            'CI_IMAGE_BTF_GEN',
            'DEB',
            'DD_AGENT_TESTING',
            'DOCKER',
            'GLIBC',
            'SYSTEM_PROBE',
            'RPM',
            'WIN',
        ]
        modified = update_image_tag(self.gitlab_ci, "1mageV3rsi0n", images, test=False)
        yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
        config = yaml.safe_load("".join(modified))
        self.assertEqual(
            0, sum(1 for k, v in config["variables"].items() if k.startswith(prefix) and v == "_test_only")
        )
        self.assertEqual(
            17, sum(1 for k, v in config["variables"].items() if k.startswith(prefix) and v == "1mageV3rsi0n")
        )


class TestUpdateGitlabConfig(unittest.TestCase):
    def test_old_images(self):
        self.assertEqual(
            len(update_gitlab_config(".gitlab-ci.yml", tag="gru", images="", test=False, update=False)), 22
        )

    def test_multi_update(self):
        self.assertEqual(
            len(update_gitlab_config(".gitlab-ci.yml", tag="gru", images="deb,rpm", test=False, update=False)), 11
        )
