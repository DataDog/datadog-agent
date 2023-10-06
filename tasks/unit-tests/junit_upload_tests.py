import os
import sys
import unittest
from unittest.mock import MagicMock

from ..libs import junit_upload as jup


class TestRetrieveJira(unittest.TestCase):
    @staticmethod
    def set_mock(ret_value, error=False):
        child_attrs = {"project.return_value": MagicMock(name="EVEN")}
        if error:
            child_attrs["search_issues.side_effect"] = Exception
        else:
            child_attrs["search_issues.return_value"] = ret_value
        attrs = {"JIRA.return_value": MagicMock(**child_attrs)}
        sys.modules['jira'] = MagicMock(**attrs)

    def test_first_occurrence(self):
        jira_cache = {}
        self.set_mock([])
        test = "fist/occurrence"
        os.environ['JIRA_TOKEN'] = "p4ssw0rd"
        card = jup.retrieve_jira_card(test, "EVEN", jira_cache)
        os.environ.pop("JIRA_TOKEN")
        self.assertFalse(card)
        self.assertIn(test, jira_cache)
        self.assertEqual("", jira_cache[test])

    def test_use_and_feed_cache(self):
        jira_cache = {"rorschach": "BAT-3"}
        card = jup.retrieve_jira_card("rorschach", "BAT", jira_cache)
        self.assertEqual("BAT-3", card)

    def test_missing_jira_token(self):
        card = jup.retrieve_jira_card("tu/peux/pas/test", "TPPT", {})
        self.assertEqual("ERROR-TOKEN", card)

    def test_several_jira_issues(self):
        self.set_mock([MagicMock(key="EVEN-4"), MagicMock(key="EVEN-2")])
        jira_cache = {}
        os.environ['JIRA_TOKEN'] = "p4ssw0rd"
        test = "hello/world"
        card = jup.retrieve_jira_card(test, "EVEN", jira_cache)
        os.environ.pop("JIRA_TOKEN")
        self.assertEqual("EVEN-2", card)
        self.assertIn(test, jira_cache)
        self.assertEqual("EVEN-2", jira_cache[test])

    def test_one_exact_match(self):
        self.set_mock([MagicMock(key="EVEN-4")])
        jira_cache = {}
        os.environ['JIRA_TOKEN'] = "p4ssw0rd"
        test = "one/match"
        card = jup.retrieve_jira_card(test, "EVEN", jira_cache)
        os.environ.pop("JIRA_TOKEN")
        self.assertEqual("EVEN-4", card)
        self.assertIn(test, jira_cache)
        self.assertEqual("EVEN-4", jira_cache[test])

    def test_exception_calling_jira(self):
        self.set_mock(MagicMock(key="EVEN-4"), error=True)
        jira_cache = {}
        os.environ['JIRA_TOKEN'] = "p4ssw0rd"
        test = "exception"
        card = jup.retrieve_jira_card(test, "EVEN", jira_cache)
        os.environ.pop("JIRA_TOKEN")
        self.assertEqual("ERROR-API", card)
        self.assertNotIn(test, jira_cache)


if __name__ == '__main__':
    unittest.main()
