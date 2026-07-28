import json
import tempfile
import unittest
import xml.etree.ElementTree as ET
from pathlib import Path
from unittest.mock import MagicMock, patch

from tasks.bazel import (
    _IMPORT_PREFIX,
    _bazel_test_funcs_from_bep,
    _go_test_packages,
    _is_gotestsum_shaped,
    _label_to_import_path,
    _parse_bep,
    _test_xml_candidates,
    _test_xml_funcs,
)


class TestLabelToImportPath(unittest.TestCase):
    def test_regular_package(self):
        self.assertEqual(
            _label_to_import_path("//pkg/util/kernel:kernel_test_iot"),
            f"{_IMPORT_PREFIX}/pkg/util/kernel",
        )

    def test_root_package(self):
        self.assertEqual(_label_to_import_path("//:root_test"), _IMPORT_PREFIX)


class TestTestXmlCandidates(unittest.TestCase):
    def test_file_uri_only(self):
        paths = _test_xml_candidates("//pkg/foo:bar_test", "file:///tmp/test.xml", "cfg1", None, {})
        self.assertEqual(paths, [Path("/tmp/test.xml")])

    def test_bytestream_uri_reconstructed_from_testlogs(self):
        paths = _test_xml_candidates(
            "//pkg/foo:bar_test",
            "bytestream://example/blobs/abc/123",
            "cfg1",
            "/exec/root",
            {"cfg1": Path("bazel-out/k8-fastbuild/testlogs")},
        )
        self.assertEqual(paths, [Path("/exec/root/bazel-out/k8-fastbuild/testlogs/pkg/foo/bar_test/test.xml")])

    def test_both_candidates_in_priority_order(self):
        paths = _test_xml_candidates(
            "//pkg/foo:bar_test",
            "file:///tmp/test.xml",
            "cfg1",
            "/exec/root",
            {"cfg1": Path("bazel-out/k8-fastbuild/testlogs")},
        )
        self.assertEqual(
            paths,
            [
                Path("/tmp/test.xml"),
                Path("/exec/root/bazel-out/k8-fastbuild/testlogs/pkg/foo/bar_test/test.xml"),
            ],
        )

    def test_no_candidates_when_config_unknown(self):
        paths = _test_xml_candidates(
            "//pkg/foo:bar_test", "bytestream://example/blobs/abc/123", "cfg1", "/exec/root", {}
        )
        self.assertEqual(paths, [])


_JUNIT_XML = """<?xml version="1.0"?>
<testsuite name="pkg/foo" tests="3">
  <testcase name="TestFoo"></testcase>
  <testcase name="TestFoo/SubCase"></testcase>
  <testcase name="TestBar"></testcase>
</testsuite>
"""


class TestTestXmlFuncs(unittest.TestCase):
    def test_top_level_funcs_only(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            xml_path = Path(tmpdir) / "test.xml"
            xml_path.write_text(_JUNIT_XML)
            self.assertEqual(_test_xml_funcs([xml_path]), {"TestFoo", "TestBar"})

    def test_falls_through_unreadable_and_empty_candidates(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            missing = Path(tmpdir) / "missing.xml"
            empty = Path(tmpdir) / "empty.xml"
            empty.write_text("")
            real = Path(tmpdir) / "test.xml"
            real.write_text(_JUNIT_XML)
            self.assertEqual(_test_xml_funcs([missing, empty, real]), {"TestFoo", "TestBar"})

    def test_raises_when_nothing_readable(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            missing = Path(tmpdir) / "missing.xml"
            with self.assertRaises(FileNotFoundError):
                _test_xml_funcs([missing])


class TestIsGotestsumShaped(unittest.TestCase):
    def test_true_when_every_testcase_has_classname(self):
        suite = ET.fromstring(
            '<testsuite tests="2">'
            '<testcase name="TestFoo" classname="pkg/foo"></testcase>'
            '<testcase name="TestBar" classname="pkg/foo"></testcase>'
            "</testsuite>"
        )
        self.assertTrue(_is_gotestsum_shaped(suite))

    def test_false_when_classname_missing(self):
        # Shape Bazel synthesizes for a test rule with no JUnit XML of its own
        # (diff_test, sh_test, rust tests, ...): one testcase, no classname.
        suite = ET.fromstring('<testsuite tests="1"><testcase name="some_check" status="run"></testcase></testsuite>')
        self.assertFalse(_is_gotestsum_shaped(suite))


def _bep_line(event: dict) -> str:
    return json.dumps(event) + "\n"


class TestParseBep(unittest.TestCase):
    def test_reconstructs_cached_result_via_testlogs(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            exec_root = Path(tmpdir) / "exec_root"
            reconstructed = exec_root / "bazel-out/k8-fastbuild/testlogs/pkg/foo/foo_test/test.xml"
            reconstructed.parent.mkdir(parents=True)
            reconstructed.write_text(_JUNIT_XML)

            bep_path = Path(tmpdir) / "bep.json"
            bep_path.write_text(
                "".join(
                    [
                        _bep_line({"id": {"workspace": {}}, "workspaceInfo": {"localExecRoot": str(exec_root)}}),
                        _bep_line(
                            {
                                "id": {"configuration": {"id": "cfg1"}},
                                "configuration": {"makeVariable": {"BINDIR": "bazel-out/k8-fastbuild/bin"}},
                            }
                        ),
                        _bep_line(
                            {
                                "id": {"testResult": {"label": "//pkg/foo:foo_test", "configuration": {"id": "cfg1"}}},
                                "testResult": {
                                    "cachedLocally": True,
                                    "testActionOutput": [
                                        {"name": "test.xml", "uri": "bytestream://example/blobs/abc/123"}
                                    ],
                                },
                            }
                        ),
                    ]
                )
            )

            xml_paths, cache_status = _parse_bep(bep_path)
            self.assertEqual(xml_paths, [reconstructed])
            self.assertEqual(cache_status, {f"{_IMPORT_PREFIX}/pkg/foo": True})

    def test_local_result_not_duplicated_via_reconstructed_path(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            exec_root = Path(tmpdir) / "exec_root"
            reconstructed = exec_root / "bazel-out/k8-fastbuild/testlogs/pkg/foo/foo_test/test.xml"
            reconstructed.parent.mkdir(parents=True)
            reconstructed.write_text(_JUNIT_XML)
            # The file:// URI Bazel reports for a local (non-cached) action
            # points at the same underlying file as the reconstructed path.
            file_uri_path = reconstructed

            bep_path = Path(tmpdir) / "bep.json"
            bep_path.write_text(
                "".join(
                    [
                        _bep_line({"id": {"workspace": {}}, "workspaceInfo": {"localExecRoot": str(exec_root)}}),
                        _bep_line(
                            {
                                "id": {"configuration": {"id": "cfg1"}},
                                "configuration": {"makeVariable": {"BINDIR": "bazel-out/k8-fastbuild/bin"}},
                            }
                        ),
                        _bep_line(
                            {
                                "id": {"testResult": {"label": "//pkg/foo:foo_test", "configuration": {"id": "cfg1"}}},
                                "testResult": {
                                    "testActionOutput": [{"name": "test.xml", "uri": file_uri_path.as_uri()}]
                                },
                            }
                        ),
                    ]
                )
            )

            xml_paths, _ = _parse_bep(bep_path)
            self.assertEqual(xml_paths, [file_uri_path])

    def test_repeated_test_result_for_same_label_all_kept(self):
        # A sharded or retried target reports multiple testResult events for
        # the same label, each with its own test.xml; none should be dropped.
        with tempfile.TemporaryDirectory() as tmpdir:
            first = Path(tmpdir) / "shard_0.xml"
            second = Path(tmpdir) / "shard_1.xml"
            first.write_text(_JUNIT_XML)
            second.write_text(_JUNIT_XML)

            bep_path = Path(tmpdir) / "bep.json"
            bep_path.write_text(
                "".join(
                    [
                        _bep_line(
                            {
                                "id": {"testResult": {"label": "//pkg/foo:foo_test", "configuration": {"id": "cfg1"}}},
                                "testResult": {"testActionOutput": [{"name": "test.xml", "uri": first.as_uri()}]},
                            }
                        ),
                        _bep_line(
                            {
                                "id": {"testResult": {"label": "//pkg/foo:foo_test", "configuration": {"id": "cfg1"}}},
                                "testResult": {"testActionOutput": [{"name": "test.xml", "uri": second.as_uri()}]},
                            }
                        ),
                    ]
                )
            )

            xml_paths, _ = _parse_bep(bep_path)
            self.assertEqual(sorted(xml_paths), sorted([first, second]))

    def test_no_test_result_events_produces_nothing(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            bep_path = Path(tmpdir) / "bep.json"
            bep_path.write_text(_bep_line({"id": {"workspace": {}}, "workspaceInfo": {"localExecRoot": "/exec/root"}}))
            self.assertEqual(_parse_bep(bep_path), ([], {}))


class TestBazelTestFuncsFromBep(unittest.TestCase):
    def test_extracts_funcs_for_dd_agent_go_test_targets_only(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            xml_path = Path(tmpdir) / "test.xml"
            xml_path.write_text(_JUNIT_XML)

            bep_path = Path(tmpdir) / "bep.json"
            bep_path.write_text(
                "".join(
                    [
                        _bep_line({"id": {"workspace": {}}, "workspaceInfo": {"localExecRoot": "/exec/root"}}),
                        _bep_line(
                            {
                                "id": {"targetConfigured": {"label": "//pkg/foo:foo_test"}},
                                "configured": {"targetKind": "go_test rule", "tag": ["dd_agent_go_test"]},
                            }
                        ),
                        _bep_line(
                            {
                                "id": {"targetConfigured": {"label": "//pkg/foo:foo_other_test"}},
                                "configured": {"targetKind": "go_test rule", "tag": []},
                            }
                        ),
                        _bep_line(
                            {
                                "id": {"targetConfigured": {"label": "//pkg/foo:not_a_go_binary"}},
                                "configured": {"targetKind": "sh_test rule", "tag": ["dd_agent_go_test"]},
                            }
                        ),
                        _bep_line(
                            {
                                "id": {"testResult": {"label": "//pkg/foo:foo_test", "configuration": {"id": "cfg1"}}},
                                "testResult": {"testActionOutput": [{"name": "test.xml", "uri": xml_path.as_uri()}]},
                            }
                        ),
                    ]
                )
            )

            covered = _bazel_test_funcs_from_bep(bep_path)
            self.assertEqual(covered, {f"{_IMPORT_PREFIX}/pkg/foo": {"TestFoo", "TestBar"}})

    def test_configured_but_never_run_is_skipped(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            bep_path = Path(tmpdir) / "bep.json"
            bep_path.write_text(
                _bep_line(
                    {
                        "id": {"targetConfigured": {"label": "//pkg/foo:foo_test"}},
                        "configured": {"targetKind": "go_test rule", "tag": ["dd_agent_go_test"]},
                    }
                )
            )
            self.assertEqual(_bazel_test_funcs_from_bep(bep_path), {})


class TestGoTestPackages(unittest.TestCase):
    def test_empty_import_paths_skips_bazel(self):
        with patch("tasks.bazel.bazel") as mock_bazel:
            self.assertEqual(_go_test_packages(MagicMock(), ["race"], set()), {})
            mock_bazel.assert_not_called()

    def test_bazel_failure_raises(self):
        with patch("tasks.bazel.bazel", side_effect=RuntimeError("boom")):
            with self.assertRaises(RuntimeError):
                _go_test_packages(MagicMock(), ["race"], {"example.com/pkg/foo"})

    def test_parses_concatenated_json_and_filters_by_import_path(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            pkg_dir = Path(tmpdir) / "foo"
            pkg_dir.mkdir()
            (pkg_dir / "foo_test.go").write_text(
                "package foo\nfunc TestFoo(t *testing.T) {}\nfunc TestMain(m *testing.M) {}\n"
            )

            stdout = "".join(
                [
                    json.dumps(
                        {
                            "ImportPath": "example.com/pkg/foo",
                            "Dir": str(pkg_dir),
                            "TestGoFiles": ["foo_test.go"],
                        }
                    ),
                    "\n",
                    json.dumps({"ImportPath": "example.com/pkg/bar", "Dir": str(tmpdir)}),
                    "\n",
                ]
            )

            with patch("tasks.bazel.bazel", return_value=stdout):
                result = _go_test_packages(MagicMock(), ["race"], {"example.com/pkg/foo"})

            # "example.com/pkg/bar" is filtered out: not in import_paths.
            # TestMain is discarded even though it matched the Test* pattern.
            self.assertEqual(result, {"example.com/pkg/foo": {"TestFoo"}})


if __name__ == "__main__":
    unittest.main()
