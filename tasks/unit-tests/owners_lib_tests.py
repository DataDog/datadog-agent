import unittest

from tasks.libs.owners.parsing import search_owners


class TestSearchCodeOwners(unittest.TestCase):
    CODEOWNERS_FILE = './tasks/unit-tests/testdata/codeowners.txt'
    JOBOWNERS_FILE = './tasks/unit-tests/testdata/jobowners.txt'

    def test_search_codeowners(self):
        self.assertListEqual(search_owners("no_owners/file", self.CODEOWNERS_FILE), [])
        self.assertListEqual(search_owners(".dotfile", self.CODEOWNERS_FILE), ["@DataDog/agent-platform"])
        self.assertListEqual(
            search_owners("doc.md", self.CODEOWNERS_FILE), ["@DataDog/agent-platform", "@DataDog/documentation"]
        )
        self.assertListEqual(search_owners(".gitlab/security.yml", self.CODEOWNERS_FILE), ["@DataDog/agent-security"])

    def test_search_jobowners(self):
        self.assertListEqual(search_owners("default_job", self.JOBOWNERS_FILE), ["@DataDog/agent-ci-experience"])
        self.assertListEqual(search_owners("tests_default", self.JOBOWNERS_FILE), ["@DataDog/multiple"])
        self.assertListEqual(search_owners("tests_ebpf_x64", self.JOBOWNERS_FILE), ["@DataDog/ebpf-platform"])
        self.assertListEqual(
            search_owners("security_go_generate_check", self.JOBOWNERS_FILE), ["@DataDog/agent-security"]
        )
        self.assertListEqual(
            search_owners("security_go_generate_checks", self.JOBOWNERS_FILE), ["@DataDog/agent-ci-experience"]
        )
