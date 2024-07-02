import unittest

from invoke import MockContext, Result

from tasks.libs.ciproviders.gitlab_api import (
    generate_gitlab_full_configuration,
    gitlab_configuration_is_modified,
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


class TestGitlabConfigurationIsModified(unittest.TestCase):
    def test_needs_one_line(self):
        file = "tasks/unit-tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..e9a74219ba 100644\n--- a/{file}\n+++ b/{file}\n@@ -4,7 +4,7 @@\n \n .linux_tests:\n   stage: source_test\n-  needs: ["go_deps", "go_tools_deps"]\n+  needs: ["go_deps", "go_tools_deps", "new"]\n   rules:\n     - !reference [.except_disable_unit_tests]\n     - !reference [.fast_on_dev_branch_only]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertTrue(gitlab_configuration_is_modified(c))

    def test_needs_removed(self):
        file = "tasks/unit-tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..e9a74219ba 100644\n--- a/{file}\n+++ b/{file}\n@@ -4,6 +4,6 @@\n \n .linux_tests:\n   stage: source_test\n-  needs: ["go_deps", "go_tools_deps"]\n   rules:\n     - !reference [.except_disable_unit_tests]\n     - !reference [.fast_on_dev_branch_only]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertFalse(gitlab_configuration_is_modified(c))

    def test_artifacts_modified_and_needs_above(self):
        file = "tasks/unit-tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..e9a74219ba 100644\n--- a/{file}\n+++ b/{file}\n@@ -12,6 +12,6 @@\n \n  artifacts:\n    expire_in: 2 years\n-    when: always\n+    when: never\n    paths:\n      - none-shall-pass.txt'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertFalse(gitlab_configuration_is_modified(c))

    def test_needs_multiple_lines(self):
        file = "tasks/unit-tests/testdata/yaml_configurations/needs_several_lines.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..e9a74219ba 100644\n--- a/{file}\n+++ b/{file}\n@@ -6,7 +6,7 @@\n \n    - go_tools_deps\n    - go_go_dancer\n    - go_go_ackman\n+    - go_nagai\n   rules:\n     - !reference [.except_disable_unit_tests]\n     - !reference [.fast_on_dev_branch_only]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertTrue(gitlab_configuration_is_modified(c))

    def test_not_a_needs_multiple_lines(self):
        file = "tasks/unit-tests/testdata/yaml_configurations/no_needs_several_lines.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..e9a74219ba 100644\n--- a/{file}\n+++ b/{file}\n@@ -6,7 +6,7 @@\n \n    - go_tools_deps\n    - go_go_dancer\n    - go_go_ackman\n+    - go_nagai\n   rules:\n     - !reference [.except_disable_unit_tests]\n     - !reference [.fast_on_dev_branch_only]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertFalse(gitlab_configuration_is_modified(c))

    def test_new_reference(self):
        file = "tasks/unit-tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..5e43218090 100644\n--- a/{file}\n+++ b/{file}\n@@ -1,4 +1,11 @@\n ---\n+.rtloader_tests:\n+  stage: source_test\n+  noods: ["go_deps"]\n+  before_script:\n+    - source /root/.bashrc && conda activate $CONDA_ENV\n+  script: ["# Skipping go tests"]\n+\n nerd_tests\n   stage: source_test\n   needs: ["go_deps"]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertFalse(gitlab_configuration_is_modified(c))

    def test_new_reference_with_needs(self):
        file = "tasks/unit-tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..5e43218090 100644\n--- a/{file}\n+++ b/{file}\n@@ -1,4 +1,11 @@\n ---\n+.rtloader_tests:\n+  stage: source_test\n+  needs: ["go_deps"]\n+  before_script:\n+    - source /root/.bashrc && conda activate $CONDA_ENV\n+  script: ["# Skipping go tests"]\n+\n nerd_tests\n   stage: source_test\n   needs: ["go_deps"]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertTrue(gitlab_configuration_is_modified(c))

    def test_new_reference_with_dependencies(self):
        file = "tasks/unit-tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..5e43218090 100644\n--- a/{file}\n+++ b/{file}\n@@ -1,4 +1,11 @@\n ---\n+.rtloader_tests:\n+  stage: source_test\n+  dependencies: ["go_deps"]\n+  before_script:\n+    - source /root/.bashrc && conda activate $CONDA_ENV\n+  script: ["# Skipping go tests"]\n+\n nerd_tests\n   stage: source_test\n   needs: ["go_deps"]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertTrue(gitlab_configuration_is_modified(c))

    def test_new_job(self):
        file = "tasks/unit-tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..5e43218090 100644\n--- a/{file}\n+++ b/{file}\n@@ -1,4 +1,11 @@\n ---\n+rtloader_tests:\n+  stage: source_test\n+  noods: ["go_deps"]\n+  before_script:\n+    - source /root/.bashrc && conda activate $CONDA_ENV\n+  script: ["# Skipping go tests"]\n+\n nerd_tests\n   stage: source_test\n   needs: ["go_deps"]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertTrue(gitlab_configuration_is_modified(c))

    def test_ignored_file(self):
        file = "tasks/unit-tests/testdata/d.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..5e43218090 100644\n--- a/{file}\n+++ b/{file}\n@@ -1,4 +1,11 @@\n ---\n+rtloader_tests:\n+  stage: source_test\n+  needs: ["go_deps"]\n+  before_script:\n+    - source /root/.bashrc && conda activate $CONDA_ENV\n+  script: ["# Skipping go tests"]\n+\n nerd_tests\n   stage: source_test\n   needs: ["go_deps"]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertFalse(gitlab_configuration_is_modified(c))

    def test_two_modified_files(self):
        file = "tasks/unit-tests/testdata/d.yml"
        yaml = "tasks/unit-tests/testdata/yaml_configurations/needs_one_line.yml"
        diff = f'diff --git a/{file} b/{file}\nindex 561eb1a201..5e43218090 100644\n--- a/{file}\n+++ b/{file}\n@@ -1,4 +1,11 @@\n ---\n+rtloader_tests:\n+  stage: source_test\n+  needs: ["go_deps"]\n+  before_script:\n+    - source /root/.bashrc && conda activate $CONDA_ENV\n+  script: ["# Skipping go tests"]\n+\n nerd_tests\n   stage: source_test\n   needs: ["go_deps"]\ndiff --git a/{yaml} b/{yaml}\nindex 561eb1a201..5e43218090 100644\n--- a/{yaml}\n+++ b/{yaml}\n@@ -1,4 +1,11 @@\n ---\n+rtloader_tests:\n+  stage: source_test\n+  noods: ["go_deps"]\n+  before_script:\n+    - source /root/.bashrc && conda activate $CONDA_ENV\n+  script: ["# Skipping go tests"]\n+\n nerd_tests\n   stage: source_test\n   needs: ["go_deps"]'
        c = MockContext(run={"git diff HEAD^1..HEAD": Result(diff)})
        self.assertTrue(gitlab_configuration_is_modified(c))
