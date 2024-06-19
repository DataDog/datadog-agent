import unittest

from tasks.libs.ciproviders.gitlab_api import generate_gitlab_full_configuration, read_includes


class TestReadIncludes(unittest.TestCase):
    def test_with_includes(self):
        includes = []
        read_includes("tasks/tests/testdata/in.yml", includes)
        self.assertEqual(len(includes), 4)

    def test_without_includes(self):
        includes = []
        read_includes("tasks/tests/testdata/b.yml", includes)
        self.assertEqual(len(includes), 1)


class TestGenerateGitlabFullConfiguration(unittest.TestCase):
    def test_nominal(self):
        full_configuration = generate_gitlab_full_configuration("tasks/tests/testdata/in.yml")
        with open("tasks/tests/testdata/out.yml") as f:
            expected = f.read()
        self.assertEqual(full_configuration, expected)

    def test_yaml_with_reference(self):
        full_configuration = generate_gitlab_full_configuration("tasks/tests/testdata/ci_config_with_reference.yml")
        with open("tasks/tests/testdata/expected.yml") as f:
            expected = f.read()
        self.assertEqual(full_configuration, expected)


class TestGitlabYaml(unittest.TestCase):
    def make_test(self, file):
        config = generate_gitlab_full_configuration(file, return_dump=False, apply_postprocessing=True)

        self.assertDictEqual(config['target'], config['expected'])

    def test_reference(self):
        self.make_test("tasks/tests/testdata/yaml_reference.yml")

    def test_extends(self):
        self.make_test("tasks/tests/testdata/yaml_extends.yml")

    def test_extends_reference(self):
        self.make_test("tasks/tests/testdata/yaml_extends_reference.yml")
