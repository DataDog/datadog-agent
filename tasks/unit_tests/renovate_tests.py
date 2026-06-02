from __future__ import annotations

import unittest

from tasks.renovate import _extract_urls, _parse_top_level_namespace


class TestParseTopLevelNamespace(unittest.TestCase):
    def test_simple_string(self):
        text = 'foo = "bar"\nhttp_archive(\n    name = "x",\n)\n'
        ns = _parse_top_level_namespace(text)
        self.assertEqual(ns["foo"], "bar")

    def test_tuple(self):
        text = 'ver = ("3", "45", "00")\nhttp_archive(\n    name = "x",\n)\n'
        ns = _parse_top_level_namespace(text)
        self.assertEqual(ns["ver"], ("3", "45", "00"))

    def test_derived_variable(self):
        text = (
            'ver = ("3", "45", "00")\n'
            'amalgamation = "sqlite-amalgamation-{}00".format("".join(ver))\n'
            'http_archive(\n    name = "x",\n)\n'
        )
        ns = _parse_top_level_namespace(text)
        # "".join(("3","45","00")) = "34500", then + "00" suffix = "3450000"
        self.assertEqual(ns["amalgamation"], "sqlite-amalgamation-3450000")

    def test_stops_before_http_archive(self):
        # Variables defined inside a block must not bleed into the namespace.
        text = 'top = "yes"\n' 'http_archive(\n' '    name = "x",\n' '    inside = "no",\n' ')\n'
        ns = _parse_top_level_namespace(text)
        self.assertIn("top", ns)
        self.assertNotIn("inside", ns)

    def test_stops_before_http_file(self):
        text = 'top = "yes"\nhttp_file(\n    name = "x",\n)\n'
        ns = _parse_top_level_namespace(text)
        self.assertIn("top", ns)

    def test_invalid_expression_skipped(self):
        # An unresolvable expression must not raise — just be absent from namespace.
        text = 'good = "ok"\nbad = unknown_var + 1\nhttp_archive(\n    name = "x",\n)\n'
        ns = _parse_top_level_namespace(text)
        self.assertEqual(ns["good"], "ok")
        self.assertNotIn("bad", ns)

    def test_empty_preamble(self):
        text = 'http_archive(\n    name = "x",\n    url = "https://example.com/x.tar.gz",\n)\n'
        ns = _parse_top_level_namespace(text)
        self.assertEqual(ns, {})


class TestExtractUrls(unittest.TestCase):
    def test_plain_literal_url(self):
        body = '    url = "https://example.com/pkg-1.0.tar.gz",\n'
        self.assertEqual(_extract_urls(body), ["https://example.com/pkg-1.0.tar.gz"])

    def test_multiple_literal_urls(self):
        body = (
            '    urls = [\n'
            '        "https://mirror.example.com/pkg-1.0.tar.gz",\n'
            '        "https://github.com/org/pkg/releases/download/v1.0/pkg-1.0.tar.gz",\n'
            '    ],\n'
        )
        urls = _extract_urls(body)
        self.assertEqual(len(urls), 2)
        self.assertIn("https://mirror.example.com/pkg-1.0.tar.gz", urls)
        self.assertIn("https://github.com/org/pkg/releases/download/v1.0/pkg-1.0.tar.gz", urls)

    def test_no_urls_no_namespace(self):
        body = '    url = "https://example.com/{}.zip".format(ver),\n'
        self.assertEqual(_extract_urls(body), [])

    def test_template_url_resolved_via_namespace(self):
        # Mirrors the sqlite3 pattern: url = ".../{}.zip".format(amalgamation)
        ns = {"amalgamation": "sqlite-amalgamation-345000"}
        body = '    url = "https://www.sqlite.org/2025/{}.zip".format(amalgamation),\n'
        urls = _extract_urls(body, ns)
        self.assertEqual(urls, ["https://www.sqlite.org/2025/sqlite-amalgamation-345000.zip"])

    def test_template_urls_list_comprehension_resolved(self):
        # Mirrors the sqlite3_license pattern:
        #   urls = [url.format(ver_str) for url in ("https://.../{}/LICENSE.md",)]
        ns = {"ver_str": "3.45.0"}
        body = (
            '    urls = [u.format(ver_str) for u in (\n'
            '        "https://raw.githubusercontent.com/sqlite/sqlite/version-{}/LICENSE.md",\n'
            '    )],\n'
        )
        urls = _extract_urls(body, ns)
        self.assertEqual(
            urls,
            ["https://raw.githubusercontent.com/sqlite/sqlite/version-3.45.0/LICENSE.md"],
        )

    def test_template_url_unresolvable_skipped(self):
        # Unknown variable — must not raise, must return empty list.
        ns: dict = {}
        body = '    url = "https://example.com/{}".format(unknown_var),\n'
        self.assertEqual(_extract_urls(body, ns), [])

    def test_literal_takes_priority_over_namespace(self):
        # If there are plain literals, we never try to eval anything.
        ns = {"ver": "1.0"}
        body = '    url = "https://example.com/pkg-1.0.tar.gz",\n'
        self.assertEqual(_extract_urls(body, ns), ["https://example.com/pkg-1.0.tar.gz"])


class TestSqliteIntegration(unittest.TestCase):
    """End-to-end: namespace parsed from a sqlite-style preamble resolves both blocks."""

    PREAMBLE = (
        'sqlite_ver = ("3", "45", "00")\n'
        'sqlite_amalgamation = "sqlite-amalgamation-{}00".format("".join(sqlite_ver))\n'
    )

    SQLITE3_BODY = (
        '    name = "sqlite3",\n'
        '    strip_prefix = sqlite_amalgamation,\n'
        '    url = "https://www.sqlite.org/2025/{}.zip".format(sqlite_amalgamation),\n'
    )

    SQLITE3_LICENSE_BODY = (
        '    name = "sqlite3_license",\n'
        '    urls = [url.format(".".join([v.replace("00", "0") for v in sqlite_ver])) for url in (\n'
        '        "https://raw.githubusercontent.com/sqlite/sqlite/version-{}/LICENSE.md",\n'
        '    )],\n'
    )

    def _ns(self):
        return _parse_top_level_namespace(self.PREAMBLE + "http_archive(\n)\n")

    def test_sqlite3_url_resolved(self):
        ns = self._ns()
        urls = _extract_urls(self.SQLITE3_BODY, ns)
        # amalgamation = "sqlite-amalgamation-3450000" (join("3","45","00") + "00")
        self.assertEqual(urls, ["https://www.sqlite.org/2025/sqlite-amalgamation-3450000.zip"])

    def test_sqlite3_license_url_resolved(self):
        ns = self._ns()
        urls = _extract_urls(self.SQLITE3_LICENSE_BODY, ns)
        # ".".join(["3","45","0"]) = "3.45.0"
        self.assertEqual(
            urls,
            ["https://raw.githubusercontent.com/sqlite/sqlite/version-3.45.0/LICENSE.md"],
        )


if __name__ == "__main__":
    unittest.main()
