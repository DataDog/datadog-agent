import unittest
from unittest.mock import MagicMock

from tasks.libs.issue.assign import guess_from_keywords, guess_from_labels


# We must define this class as we cannot override the name attribute in MagicMock
class Label:
    def __init__(self, name):
        self.name = name


class TestGuessFromLabels(unittest.TestCase):
    def test_with_team(self):
        issue = MagicMock(labels=[Label(name="team/triage"), Label(name="team/core")])

        self.assertEqual(guess_from_labels(issue), "core")

    def test_without_team(self):
        issue = MagicMock(labels=[Label(name="team/triage"), Label(name="team:burton")])

        self.assertEqual(guess_from_labels(issue), "triage")


class TestGuessFromKeywords(unittest.TestCase):
    def test_from_simple_match(self):
        issue = MagicMock(title="I have an issue", body="I can't get any logs from the agent.")
        self.assertEqual(guess_from_keywords(issue), "agent-log-pipelines")

    def test_with_a_file(self):
        issue = MagicMock(title="fix bug", body="It comes from the file pkg/agent/build.py")
        self.assertEqual(guess_from_keywords(issue), "agent-runtimes")

    def test_no_match(self):
        issue = MagicMock(title="fix bug", body="It comes from the file... hm I don't know.")
        self.assertEqual(guess_from_keywords(issue), "triage")
