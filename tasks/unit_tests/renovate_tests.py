from __future__ import annotations

import unittest
from pathlib import Path

from tasks.renovate import (
    RepositoryRuleCall,
    _parse_repository_rule_calls_from_text,
    _replace_sha256_in_rule_block,
    _strip_use_repo_rule_assignments,
)

try:
    import starlark  # noqa: F401

    HAS_STARLARK = True
except ModuleNotFoundError:
    HAS_STARLARK = False


ROOT = Path("/repo")


@unittest.skipUnless(HAS_STARLARK, "starlark-pyo3 is only available in the Bazel Python toolchain")
class TestParseRepositoryRuleCalls(unittest.TestCase):
    def test_resolves_top_level_version(self):
        path = ROOT / "deps" / "demo" / "demo.MODULE.bazel"
        text = """
http_archive = use_repo_rule("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

VERSION = "1.2.3"

http_archive(
    name = "demo",
    sha256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    strip_prefix = "demo-{}".format(VERSION),
    url = "https://example.com/demo-{}.tar.gz".format(VERSION),
)
"""

        calls = _parse_repository_rule_calls_from_text(path, text, ROOT)
        call = calls[("deps/demo/demo.MODULE.bazel", "demo")]

        self.assertEqual(call.kind, "http_archive")
        self.assertEqual(call.name, "demo")
        self.assertEqual(call.sha256, "a" * 64)
        self.assertEqual(call.urls, ("https://example.com/demo-1.2.3.tar.gz",))
        self.assertIn("demo-1.2.3", repr(call.identity))

    def test_resolves_loop_emitted_names_and_urls(self):
        path = ROOT / "deps" / "cpython" / "cpython.MODULE.bazel"
        text = """
http_archive = use_repo_rule("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

python_src_deps = {
    "xz": ("5.2.5", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
    "zlib": ("1.3.1", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
}

[
    http_archive(
        name = "{}_win".format(name),
        sha256 = sha256,
        strip_prefix = "cpython-source-deps-{name}-{version}".format(
            name = name,
            version = version,
        ),
        url = "https://github.com/python/cpython-source-deps/archive/{name}-{version}.zip".format(
            name = name,
            version = version,
        ),
    )
    for name, (version, sha256) in python_src_deps.items()
]
"""

        calls = _parse_repository_rule_calls_from_text(path, text, ROOT)

        self.assertEqual(
            calls[("deps/cpython/cpython.MODULE.bazel", "xz_win")].urls,
            ("https://github.com/python/cpython-source-deps/archive/xz-5.2.5.zip",),
        )
        self.assertEqual(
            calls[("deps/cpython/cpython.MODULE.bazel", "zlib_win")].urls,
            ("https://github.com/python/cpython-source-deps/archive/zlib-1.3.1.zip",),
        )

    def test_http_file_is_recorded(self):
        path = ROOT / "deps" / "gstatus" / "gstatus.MODULE.bazel"
        text = """
http_file = use_repo_rule("@bazel_tools//tools/build_defs/repo:http.bzl", "http_file")

VERSION = "1.0.9"

http_file(
    name = "gstatus_binary",
    executable = True,
    sha256 = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
    urls = [
        "https://github.com/gluster/gstatus/releases/download/v{version}/gstatus".format(version = VERSION),
    ],
)
"""

        calls = _parse_repository_rule_calls_from_text(path, text, ROOT)
        call = calls[("deps/gstatus/gstatus.MODULE.bazel", "gstatus_binary")]

        self.assertEqual(call.kind, "http_file")
        self.assertEqual(call.urls, ("https://github.com/gluster/gstatus/releases/download/v1.0.9/gstatus",))


class TestUseRepoRuleStripping(unittest.TestCase):
    def test_strips_multiline_assignment(self):
        text = """
http_archive = use_repo_rule(
    "@bazel_tools//tools/build_defs/repo:http.bzl",
    "http_archive",
)

http_archive(name = "x")
"""

        stripped = _strip_use_repo_rule_assignments(text)

        self.assertNotIn("use_repo_rule", stripped)
        self.assertIn('http_archive(name = "x")', stripped)


class TestReplaceSha256(unittest.TestCase):
    def test_replaces_http_file_sha256(self):
        text = """
http_file(
    name = "gstatus_binary",
    executable = True,
    sha256 = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
    urls = ["https://example.com/gstatus"],
)
"""
        call = RepositoryRuleCall(
            kind="http_file",
            name="gstatus_binary",
            path=ROOT / "deps" / "gstatus" / "gstatus.MODULE.bazel",
            relative_path="deps/gstatus/gstatus.MODULE.bazel",
            sha256="d" * 64,
            urls=("https://example.com/gstatus",),
            identity=(),
        )

        updated = _replace_sha256_in_rule_block(text, call, "e" * 64)

        self.assertIn(f'sha256 = "{"e" * 64}"', updated)
        self.assertNotIn(f'sha256 = "{"d" * 64}"', updated)

    def test_templated_name_replacement_fails_loudly(self):
        text = """
[
    http_archive(
        name = "{}_win".format(name),
        sha256 = sha256,
        url = "https://example.com/{}.zip".format(name),
    )
    for name, sha256 in {"xz": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}.items()
]
"""
        call = RepositoryRuleCall(
            kind="http_archive",
            name="xz_win",
            path=ROOT / "deps" / "cpython" / "cpython.MODULE.bazel",
            relative_path="deps/cpython/cpython.MODULE.bazel",
            sha256="d" * 64,
            urls=("https://example.com/xz.zip",),
            identity=(),
        )

        with self.assertRaisesRegex(Exception, "Templated names"):
            _replace_sha256_in_rule_block(text, call, "e" * 64)


if __name__ == "__main__":
    unittest.main()
