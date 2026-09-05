import importlib.util
import re
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def load_links():
    """Load the documentation link utilities without depending on the DDA extension path."""
    path = ROOT / ".dda" / "extend" / "pythonpath" / "utils" / "docs" / "links.py"
    spec = importlib.util.spec_from_file_location("docs_links", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


links = load_links()


class TestLycheeCommand(unittest.TestCase):
    def test_published_site_urls_are_mapped_to_the_local_build(self):
        with tempfile.TemporaryDirectory() as directory:
            site_dir = Path(directory, "site with spaces")
            config_path = Path(directory, "mkdocs.yml")
            config_path.write_text(
                """site_url: https://example.test/docs
slugify: !!python/object/apply:module.function
  kwds:
    case: lower
""",
                encoding="utf-8",
            )
            command = links._lychee_command(site_dir, config_path)
            remap = command[command.index("--remap") + 1]
            pattern, replacement = remap.split(" ", 1)

            published_page = "https://example.test/docs/how-to/test/e2e/running/"
            expected_page = f"{site_dir.resolve().as_uri()}/how-to/test/e2e/running/"
            self.assertEqual(re.sub(pattern, replacement, published_page), expected_page)
            self.assertEqual(command[-1], str(site_dir))

    def test_missing_site_url_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            config_path = Path(directory, "mkdocs.yml")
            config_path.write_text("site_name: Documentation\n", encoding="utf-8")

            with self.assertRaisesRegex(ValueError, "`site_url` must be an absolute HTTP\\(S\\) base URL"):
                links._published_site_url(config_path)

    def test_site_url_with_a_fragment_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            config_path = Path(directory, "mkdocs.yml")
            config_path.write_text("site_url: https://example.test/docs/#fragment\n", encoding="utf-8")

            with self.assertRaisesRegex(ValueError, "`site_url` must be an absolute HTTP\\(S\\) base URL"):
                links._published_site_url(config_path)


if __name__ == "__main__":
    unittest.main()
