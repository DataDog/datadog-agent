import unittest

from tasks.libs.owners.parsing import search_owners


class TestSearchCodeOwners(unittest.TestCase):
    CODEOWNERS_FILE = './tasks/unit-tests/testdata/codeowners.txt'
    JOBOWNERS_FILE = './tasks/unit-tests/testdata/jobowners.txt'

    def test_search_codeowners(self):
        self.assertListEqual(search_owners("no_owners/file", self.CODEOWNERS_FILE), [])
        self.assertListEqual(search_owners(".dotfile", self.CODEOWNERS_FILE), ["@DataDog/team-everything"])
        self.assertListEqual(search_owners("doc.md", self.CODEOWNERS_FILE), ["@DataDog/team-a", "@DataDog/team-doc"])
        self.assertListEqual(search_owners(".gitlab/security.yml", self.CODEOWNERS_FILE), ["@DataDog/team-b"])

    def test_search_jobowners(self):
        self.assertListEqual(search_owners("default_job", self.JOBOWNERS_FILE), ["@DataDog/team-everything"])
        self.assertListEqual(search_owners("tests_team_a_42", self.JOBOWNERS_FILE), ["@DataDog/team-a"])
        self.assertListEqual(search_owners("tests_team_b_1618", self.JOBOWNERS_FILE), ["@DataDog/team-b"])
        self.assertListEqual(
            search_owners("tests_letters_314", self.JOBOWNERS_FILE), ["@DataDog/team-a", "@DataDog/team-b"]
        )
