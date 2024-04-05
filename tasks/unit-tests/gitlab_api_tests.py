import unittest

from tasks.libs.common.gitlab_api import generate_gitlab_full_configuration, read_includes


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
