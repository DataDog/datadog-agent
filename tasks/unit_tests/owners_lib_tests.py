import unittest

from tasks.libs.owners.parsing import search_owners


class TestSearchCodeOwners(unittest.TestCase):
    CODEOWNERS_FILE = './tasks/unit_tests/testdata/codeowners.txt'

    # TODO(@agent-devx): Remove these tests once `dda info owners code` is available
    def test_search_codeowners(self):
        self.assertListEqual(search_owners("no_owners/file", self.CODEOWNERS_FILE), [])
        self.assertListEqual(search_owners(".dotfile", self.CODEOWNERS_FILE), ["@DataDog/team-everything"])
        self.assertListEqual(search_owners("doc.md", self.CODEOWNERS_FILE), ["@DataDog/team-a", "@DataDog/team-doc"])
        self.assertListEqual(search_owners(".gitlab/security.yml", self.CODEOWNERS_FILE), ["@DataDog/team-b"])
