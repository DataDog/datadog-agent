import unittest

from tasks.libs.common.shipped_files import is_shipped_file, touches_shipped_files


class TestIsShippedFile(unittest.TestCase):
    def test_shipped(self):
        for path in [
            'pkg/collector/check.go',
            'cmd/agent/dist/conf.d/disk.d/conf.yaml.default',
            'omnibus/config/software/datadog-agent.rb',
            'Dockerfiles/agent/DOCKERHUB.md',
            'packages/installer/README.md',
            'go.mod',
            'release.json',
            '.gitlab-ci.yml',
            '.gitlab/deploy/container_build/docker_linux.yml',
            'tasks/agent.py',
            'tasks/build_tags.py',
            'tasks/libs/ciproviders/github_api.py',
            'tools/windows/DatadogAgentInstaller/CustomActions/CustomAction.cs',
            'pkg/security/tests_helper.go',
            'unknown/path',
        ]:
            self.assertTrue(is_shipped_file(path), path)

    def test_not_shipped(self):
        for path in [
            'pkg/collector/check_test.go',
            'pkg/collector/testdata/config.yaml',
            'test/new-e2e/tests/agent-subcommands/status_test.go',
            'pkg/security/tests/module_tester.go',
            'pkg/collector/README.md',
            'AGENTS.md',
            'doc/how-to/test/e2e/index.md',
            'releasenotes/notes/fix-something-0123456789abcdef.yaml',
            '.github/CODEOWNERS',
            '.github/workflows/label-analysis.yml',
            '.gitlab/test/e2e/e2e.yml',
            '.gitlab/build/lint/technical_linters.yml',
            '.gitlab/JOBOWNERS',
            'tasks/unit_tests/github_tasks_tests.py',
            'tasks/github_tasks.py',
            'tasks/e2e_framework/tool.py',
            '.agents/skills/create-pr/SKILL.md',
            '.claude/settings.json',
            '.golangci.yml',
            'flakes.yaml',
        ]:
            self.assertFalse(is_shipped_file(path), path)


class TestTouchesShippedFiles(unittest.TestCase):
    def test_no_files(self):
        self.assertFalse(touches_shipped_files([]))

    def test_only_not_shipped(self):
        self.assertFalse(touches_shipped_files(['pkg/collector/check_test.go', 'doc/index.md']))

    def test_one_shipped(self):
        self.assertTrue(touches_shipped_files(['pkg/collector/check_test.go', 'pkg/collector/check.go']))
