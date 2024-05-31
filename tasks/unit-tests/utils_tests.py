import os
import unittest
from unittest.mock import MagicMock, patch

from invoke import MockContext, Result

from tasks.libs.common.utils import clean_nested_paths, guess_from_keywords, guess_from_labels, query_version


class TestUtils(unittest.TestCase):
    def test_clean_nested_paths_1(self):
        paths = [
            "./pkg/utils/toto",
            "./pkg/utils/",
            "./pkg",
            "./toto/pkg",
            "./pkg/utils/tata",
            "./comp",
            "./component",
            "./comp/toto",
        ]
        expected_paths = ["./comp", "./component", "./pkg", "./toto/pkg"]
        self.assertEqual(clean_nested_paths(paths), expected_paths)

    def test_clean_nested_paths_2(self):
        paths = [
            ".",
            "./pkg/utils/toto",
            "./pkg/utils/",
            "./pkg",
            "./toto/pkg",
            "./pkg/utils/tata",
            "./comp",
            "./component",
            "./comp/toto",
        ]
        expected_paths = ["."]
        self.assertEqual(clean_nested_paths(paths), expected_paths)


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
        self.assertEqual(guess_from_keywords(issue), "agent-metrics-logs")

    def test_with_a_file(self):
        issue = MagicMock(title="fix bug", body="It comes from the file pkg/agent/build.py")
        self.assertEqual(guess_from_keywords(issue), "agent-shared-components")

    def test_no_match(self):
        issue = MagicMock(title="fix bug", body="It comes from the file... hm I don't know.")
        self.assertEqual(guess_from_keywords(issue), "triage")


class TestQueryVersion(unittest.TestCase):
    @patch.dict(os.environ, {"BUCKET_BRANCH": "dev"}, clear=True)
    def test_on_dev_bucket(self):
        major_version = "7"
        c = MockContext(
            run={
                r'git describe --tags --candidates=50 --match "7\.*" --abbrev=7': Result(
                    "7.54.0-dbm-mongo-0.1-163-g315e3a2"
                )
            }
        )
        v, p, c, g, _ = query_version(c, major_version)
        self.assertEqual(v, "7.54.0")
        self.assertEqual(p, "dbm-mongo-0.1")
        self.assertEqual(c, 163)
        self.assertEqual(g, "315e3a2")

    @patch.dict(os.environ, {"BUCKET_BRANCH": "nightly"}, clear=True)
    def test_on_nightly_bucket(self):
        major_version = "7"
        c = MockContext(
            run={
                rf"git tag --list | grep -E '^{major_version}\.[0-9]+\.[0-9]+(-rc.*|-devel.*)?$' | sort -rV | head -1": Result(
                    "7.55.0-devel"
                ),
                'git describe --tags --candidates=50 --match "7.55.0-devel" --abbrev=7': Result(
                    "7.55.0-devel-543-g315e3a2"
                ),
            }
        )
        v, p, c, g, _ = query_version(c, major_version)
        self.assertEqual(v, "7.55.0")
        self.assertEqual(p, "devel")
        self.assertEqual(c, 543)
        self.assertEqual(g, "315e3a2")
