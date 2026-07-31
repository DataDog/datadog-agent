import importlib.util
import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
DOCS_DIR = ROOT / "docs" / "public"


def load_hook():
    """Load the documentation macros, which import nothing outside the standard library."""
    path = DOCS_DIR / ".hooks" / "inject_variables.py"
    spec = importlib.util.spec_from_file_location("inject_variables", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


hook = load_hook()


class TestResolveRepoPath(unittest.TestCase):
    """Cover the branches that the paths written in docs/public do not reach."""

    # Resolved against a fixture tree rather than the repository, so that editing a real file cannot
    # fail these tests and every case can be expressed rather than scavenged for.
    @classmethod
    def setUpClass(cls):
        directory = tempfile.TemporaryDirectory()
        cls.addClassCleanup(directory.cleanup)
        cls.root = Path(directory.name)
        cls.docs_dir = cls.root / "docs" / "public"

        (cls.docs_dir / ".snippets").mkdir(parents=True)
        (cls.docs_dir / "index.md").write_text("# Index\n", encoding="utf-8")
        (cls.docs_dir / ".snippets" / "links.txt").write_text("[a]: https://example.com\n", encoding="utf-8")
        (cls.root / "pkg" / "rules").mkdir(parents=True)
        # Two lines match `  - `, so that an ambiguous match has something to find.
        (cls.root / "config.yml").write_text("site_name: Datadog\nnav:\n  - a\n  - b\nstages:\n", encoding="utf-8")
        # GitHub answers 400 for a raw `%%` in a URL.
        (cls.root / "weird-%%-name.yaml").write_text("x\n", encoding="utf-8")

        cls.symlink = None
        try:
            os.symlink(cls.root / "config.yml", cls.root / "alias.yml")
        except OSError:
            # Creating one needs a privilege that Windows does not grant by default.
            pass
        else:
            cls.symlink = "alias.yml"

    def resolve(self, path, match=None, ref="main"):
        return hook.resolve_repo_path(self.root, self.docs_dir, ref, "page.md", path, match)

    def assert_rejected(self, path, match=None, *, because):
        with self.assertRaises(hook.RepoPathError) as caught:
            self.resolve(path, match)
        self.assertIn(because, str(caught.exception))
        # Every failure names the page so that a build error is actionable.
        self.assertTrue(str(caught.exception).startswith("page.md: "))

    def test_file_links_to_a_blob(self):
        self.assertEqual(self.resolve("config.yml"), "blob/main/config.yml")

    def test_directory_links_to_a_tree(self):
        self.assertEqual(self.resolve("pkg/rules"), "tree/main/pkg/rules")

    def test_ref_is_used_verbatim(self):
        self.assertEqual(self.resolve("config.yml", ref="abc123"), "blob/abc123/config.yml")

    def test_reserved_characters_are_percent_encoded(self):
        self.assertEqual(self.resolve("weird-%%-name.yaml"), "blob/main/weird-%25%25-name.yaml")

    def test_the_repository_itself_still_resolves(self):
        # The fixture tree cannot catch the hook drifting from how the real checkout is laid out.
        self.assertEqual(
            hook.resolve_repo_path(ROOT, DOCS_DIR, "main", "page.md", "mkdocs.yml", None), "blob/main/mkdocs.yml"
        )

    def test_missing_path_is_rejected(self):
        self.assert_rejected("pkg/does-not-exist", because="path does not exist")

    def test_unnormalized_paths_are_rejected(self):
        for path in ("../secrets", "/etc/passwd", "pkg/rules/", "pkg//rules", "./config.yml"):
            self.assert_rejected(path, because="must be normalized")

    def test_absolute_and_windows_paths_are_rejected(self):
        for path in ("C:/Windows", "pkg\\rules", "https://example.com", " config.yml"):
            self.assert_rejected(path, because="must be relative to the repository root")

    def test_empty_path_is_rejected(self):
        self.assert_rejected("", because="empty repository path")

    def test_wrong_casing_is_rejected(self):
        # Rejected on a case-insensitive file system by comparing against the directory listing, and
        # on a case-sensitive one because the path simply is not there, so the message differs.
        with self.assertRaises(hook.RepoPathError):
            self.resolve("PKG/rules")

    def test_documentation_pages_must_be_linked_relatively(self):
        self.assert_rejected("docs/public/index.md", because="link documentation pages relatively")

    def test_documentation_assets_may_be_linked(self):
        self.assertEqual(self.resolve("docs/public/.snippets/links.txt"), "blob/main/docs/public/.snippets/links.txt")

    def test_anchor_shapes_are_validated(self):
        for anchor in ("top", "L0", "L1x"):
            self.assert_rejected(f"config.yml#{anchor}", because="line anchor must look like")

    def test_anchor_past_end_of_file_is_rejected(self):
        self.assert_rejected("config.yml#L99999", because="but the anchor refers to line 99999")

    def test_reversed_anchor_range_is_rejected(self):
        # Only checking the last bound would let this through with a dead anchor.
        self.assert_rejected("config.yml#L99999-L1", because="must not run backwards")

    def test_anchor_range_is_bounds_checked(self):
        self.assert_rejected("config.yml#L1-L99999", because="but the anchor refers to line 99999")

    def test_anchor_within_the_file_is_kept(self):
        self.assertEqual(self.resolve("config.yml#L2-L4"), "blob/main/config.yml#L2-L4")

    def test_anchor_on_a_directory_is_rejected(self):
        self.assert_rejected("pkg/rules#L1", because="line anchors require a file")

    def test_anchor_and_match_together_are_rejected(self):
        self.assert_rejected("config.yml#L1", "site_name", because="either a line anchor or `match`")

    def test_match_resolves_a_line_number(self):
        self.assertEqual(self.resolve("config.yml", "^stages:"), "blob/main/config.yml#L5")

    def test_match_is_an_unanchored_regular_expression(self):
        self.assertEqual(self.resolve("config.yml", r"^site_name:\s+Datadog"), "blob/main/config.yml#L1")

    def test_ambiguous_match_is_rejected(self):
        self.assert_rejected("config.yml", "  - ", because="found 2 (lines 3, 4)")

    def test_unmatched_expression_is_rejected(self):
        self.assert_rejected("config.yml", "nowhere to be found", because="found 0")

    def test_invalid_expression_is_rejected(self):
        self.assert_rejected("config.yml", "package(", because="invalid `match` expression")

    def test_symlinks_cannot_carry_anchors(self):
        # GitHub serves a symlink as a one-line blob, so a line number derived from its target lies.
        if self.symlink is None:
            self.skipTest("this platform does not allow creating a symlink")
        self.assert_rejected(self.symlink, "anything", because="rather than a symlink")


class TestSourceLines(unittest.TestCase):
    def lines(self, content):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory, "probe.txt")
            path.write_bytes(content)
            return hook.source_lines(path)

    def test_trailing_newline_does_not_add_a_line(self):
        self.assertEqual(self.lines(b"one\ntwo\n"), ["one", "two"])

    def test_missing_trailing_newline_still_counts_the_last_line(self):
        self.assertEqual(self.lines(b"one\ntwo"), ["one", "two"])

    def test_form_feed_is_not_a_line_break(self):
        # str.splitlines() would split here, while GitHub numbers by newlines alone.
        self.assertEqual(self.lines(b"one\ntwo\x0cthree\n"), ["one", "two\x0cthree"])

    def test_carriage_returns_are_stripped(self):
        # .gitattributes keeps files such as `*.bat` and `*.ps1` in CRLF, where a trailing `\r`
        # would stop a `$` anchored expression from ever matching.
        self.assertEqual(self.lines(b"one\r\ntwo\r\n"), ["one", "two"])


class TestRepoRef(unittest.TestCase):
    def setUp(self):
        self.enterContext(mock.patch.dict(os.environ))
        # The ref is memoized for the process, so every case has to start from an empty cache.
        hook.CACHE.pop("ref_stamp", None)
        self.addCleanup(hook.CACHE.pop, "ref_stamp", None)

    def test_environment_overrides_git(self):
        os.environ["DOCS_REF"] = "release/7.60.x"
        self.assertEqual(hook.repo_ref(ROOT), "release/7.60.x")

    def test_reserved_characters_in_the_ref_are_encoded(self):
        # A branch name may contain `#`, which would otherwise truncate every link at the fragment.
        os.environ["DOCS_REF"] = "fix/#54123-flare"
        self.assertEqual(hook.repo_ref(ROOT), "fix/%2354123-flare")

    def test_git_supplies_the_ref_when_the_environment_does_not(self):
        os.environ.pop("DOCS_REF", None)
        self.assertTrue(hook.repo_ref(ROOT))


if __name__ == "__main__":
    unittest.main()
