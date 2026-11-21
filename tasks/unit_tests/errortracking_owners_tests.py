import os
import tempfile
import unittest
from unittest import mock

from invoke.exceptions import Exit

from tasks import errortracking_owners as eo

CODEOWNERS = """\
# comment
* @DataDog/team-default
/pkg/logs/ @DataDog/team-logs @DataDog/team-other
/pkg/logs/sender/ @DataDog/team-sender
/docs/*.md
/cmd/**/*_windows*.go @DataDog/team-win
"""


class TestErrortrackingOwners(unittest.TestCase):
    def setUp(self):
        fd, self.path = tempfile.mkstemp()
        with os.fdopen(fd, "w") as f:
            f.write(CODEOWNERS)
        self.addCleanup(os.remove, self.path)

    def test_rules_keep_file_order_and_primary_owner(self):
        self.assertEqual(
            eo._rules(self.path),
            [
                ("*", "team-default"),
                ("/pkg/logs/", "team-logs"),
                ("/pkg/logs/sender/", "team-sender"),
                ("/docs/*.md", ""),
                ("/cmd/**/*_windows*.go", "team-win"),
            ],
        )

    def test_render_go_indexes_teams(self):
        out = eo.render_go(eo._rules(self.path))
        self.assertIn("const ownerTeams = `team-default\nteam-logs\nteam-sender\nteam-win`", out)
        self.assertIn("0 *\n1 /pkg/logs/\n2 /pkg/logs/sender/\n- /docs/*.md\n3 /cmd/**/*_windows*.go`", out)

    def test_golden_matches_python_parser(self):
        golden = dict(line.split("\t") for line in eo.render_golden(self.path, eo._rules(self.path)).splitlines())
        self.assertEqual(golden["pkg/logs/sender/z.go"], "team-sender")
        self.assertEqual(golden["cmd/a/b/x_windowsx.go"], "team-win")
        self.assertEqual(golden["docs/x.md"], "")
        self.assertEqual(golden["sub/x/z.go"], "team-default")

    def test_unsupported_pattern_is_rejected(self):
        with mock.patch.object(eo, "read_owners") as read_owners:
            read_owners.return_value.paths = [(None, "/a[bc]/", [], 1, None)]
            with self.assertRaises(Exit):
                eo._rules("unused")

    def test_check_detects_stale_file(self):
        with self.assertRaises(Exit):
            eo._check(self.path, "something else\n")
        eo._check(self.path, CODEOWNERS)


if __name__ == "__main__":
    unittest.main()
