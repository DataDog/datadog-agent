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

    def test_render_tables_indexes_teams(self):
        self.assertEqual(
            eo.render_tables(eo._rules(self.path)),
            "team-default\nteam-logs\nteam-sender\nteam-win\x00"
            "0 *\n1 /pkg/logs/\n2 /pkg/logs/sender/\n- /docs/*.md\n3 /cmd/**/*_windows*.go",
        )

    def test_render_go_embeds_the_deflated_tables(self):
        rules = eo._rules(self.path)
        out = eo.render_go(rules)
        self.assertIn("const ownerTablesDeflated = \"", out)
        self.assertEqual(eo.go_tables(out), eo.render_tables(rules))
        self.assertNotIn("team-default", out, "the tables are stored compressed")

    def test_go_tables_rejects_missing_or_corrupt_data(self):
        self.assertIsNone(eo.go_tables("package x\n"))
        self.assertIsNone(eo.go_tables('const ownerTablesDeflated = "\\x00\\x01"\n'))

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

    def test_check_go_compares_what_the_file_says_not_its_bytes(self):
        rules = eo._rules(self.path)
        fd, go_path = tempfile.mkstemp()
        os.close(fd)
        self.addCleanup(os.remove, go_path)
        out = eo.render_go(rules)

        other = eo.zlib.compressobj(0, eo.zlib.DEFLATED, -15)  # stored blocks: always other bytes
        blob = other.compress(eo.render_tables(rules).encode()) + other.flush()
        self.assertNotEqual(blob, eo.deflate(eo.render_tables(rules)))
        recompressed = out.split(eo._CONST)[0] + eo._CONST + " " + eo._go_literal(blob) + "\n"
        with open(go_path, "w") as f:
            f.write(recompressed)
        eo._check_go(go_path, rules)

        # Different tables, or a different surrounding file: stale.
        with open(go_path, "w") as f:
            f.write(eo.render_go(rules[:-1]))
        with self.assertRaises(Exit):
            eo._check_go(go_path, rules)
        with open(go_path, "w") as f:
            f.write(recompressed.replace("DO NOT EDIT", "edited by hand"))
        with self.assertRaises(Exit):
            eo._check_go(go_path, rules)


if __name__ == "__main__":
    unittest.main()
