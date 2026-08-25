import json
import tempfile
import unittest
import xml.etree.ElementTree as ET
from pathlib import Path

from tasks.bazel import (
    _IMPORT_PREFIX,
    _is_gotestsum_shaped,
    _label_to_import_path,
    _parse_bep,
    _test_output_candidates,
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
        paths = _test_output_candidates("//pkg/foo:bar_test", "file:///tmp/test.xml", "cfg1", None, {}, "test.xml")
        self.assertEqual(paths, [Path("/tmp/test.xml")])

    def test_bytestream_uri_reconstructed_from_testlogs(self):
        paths = _test_output_candidates(
            "//pkg/foo:bar_test",
            "bytestream://example/blobs/abc/123",
            "cfg1",
            "/exec/root",
            {"cfg1": Path("bazel-out/k8-fastbuild/testlogs")},
            "test.xml",
        )
        self.assertEqual(paths, [Path("/exec/root/bazel-out/k8-fastbuild/testlogs/pkg/foo/bar_test/test.xml")])

    def test_both_candidates_in_priority_order(self):
        paths = _test_output_candidates(
            "//pkg/foo:bar_test",
            "file:///tmp/test.xml",
            "cfg1",
            "/exec/root",
            {"cfg1": Path("bazel-out/k8-fastbuild/testlogs")},
            "test.xml",
        )
        self.assertEqual(
            paths,
            [
                Path("/tmp/test.xml"),
                Path("/exec/root/bazel-out/k8-fastbuild/testlogs/pkg/foo/bar_test/test.xml"),
            ],
        )

    def test_no_candidates_when_config_unknown(self):
        paths = _test_output_candidates(
            "//pkg/foo:bar_test", "bytestream://example/blobs/abc/123", "cfg1", "/exec/root", {}, "test.xml"
        )
        self.assertEqual(paths, [])


_JUNIT_XML = """<?xml version="1.0"?>
<testsuite name="pkg/foo" tests="3">
  <testcase name="TestFoo"></testcase>
  <testcase name="TestFoo/SubCase"></testcase>
  <testcase name="TestBar"></testcase>
</testsuite>
"""


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
            reconstructed_log = reconstructed.parent / "test.log"
            reconstructed.parent.mkdir(parents=True)
            reconstructed.write_text(_JUNIT_XML)
            reconstructed_log.write_text("PASS\n")

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
                                        {"name": "test.xml", "uri": "bytestream://example/blobs/abc/123"},
                                        {"name": "test.log", "uri": "bytestream://example/blobs/def/456"},
                                    ],
                                },
                            }
                        ),
                    ]
                )
            )

            self.assertEqual(
                _parse_bep(bep_path),
                {
                    "//pkg/foo:foo_test": {
                        "cached": True,
                        "xml_paths": [reconstructed],
                        "log_paths": [reconstructed_log],
                    }
                },
            )

    def test_local_result_not_duplicated_via_reconstructed_path(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            exec_root = Path(tmpdir) / "exec_root"
            reconstructed = exec_root / "bazel-out/k8-fastbuild/testlogs/pkg/foo/foo_test/test.xml"
            reconstructed_log = reconstructed.parent / "test.log"
            reconstructed.parent.mkdir(parents=True)
            reconstructed.write_text(_JUNIT_XML)
            reconstructed_log.write_text("PASS\n")
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
                                    "testActionOutput": [
                                        {"name": "test.xml", "uri": file_uri_path.as_uri()},
                                        {"name": "test.log", "uri": reconstructed_log.as_uri()},
                                    ]
                                },
                            }
                        ),
                    ]
                )
            )

            artifacts = _parse_bep(bep_path)
            self.assertEqual(artifacts["//pkg/foo:foo_test"]["xml_paths"], [file_uri_path])
            self.assertEqual(artifacts["//pkg/foo:foo_test"]["log_paths"], [reconstructed_log])

    def test_repeated_test_result_for_same_label_all_kept(self):
        # A sharded or retried target reports multiple testResult events for
        # the same label, each with its own test.xml/test.log; none should be dropped.
        with tempfile.TemporaryDirectory() as tmpdir:
            first = Path(tmpdir) / "shard_0.xml"
            second = Path(tmpdir) / "shard_1.xml"
            first_log = Path(tmpdir) / "shard_0.log"
            second_log = Path(tmpdir) / "shard_1.log"
            first.write_text(_JUNIT_XML)
            second.write_text(_JUNIT_XML)
            first_log.write_text("PASS\n")
            second_log.write_text("PASS\n")

            bep_path = Path(tmpdir) / "bep.json"
            bep_path.write_text(
                "".join(
                    [
                        _bep_line(
                            {
                                "id": {"testResult": {"label": "//pkg/foo:foo_test", "configuration": {"id": "cfg1"}}},
                                "testResult": {
                                    "testActionOutput": [
                                        {"name": "test.xml", "uri": first.as_uri()},
                                        {"name": "test.log", "uri": first_log.as_uri()},
                                    ]
                                },
                            }
                        ),
                        _bep_line(
                            {
                                "id": {"testResult": {"label": "//pkg/foo:foo_test", "configuration": {"id": "cfg1"}}},
                                "testResult": {
                                    "testActionOutput": [
                                        {"name": "test.xml", "uri": second.as_uri()},
                                        {"name": "test.log", "uri": second_log.as_uri()},
                                    ]
                                },
                            }
                        ),
                    ]
                )
            )

            artifacts = _parse_bep(bep_path)
            self.assertEqual(sorted(artifacts["//pkg/foo:foo_test"]["xml_paths"]), sorted([first, second]))
            self.assertEqual(sorted(artifacts["//pkg/foo:foo_test"]["log_paths"]), sorted([first_log, second_log]))

    def test_no_test_result_events_produces_nothing(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            bep_path = Path(tmpdir) / "bep.json"
            bep_path.write_text(_bep_line({"id": {"workspace": {}}, "workspaceInfo": {"localExecRoot": "/exec/root"}}))
            self.assertEqual(_parse_bep(bep_path), {})

    def test_missing_log_is_error(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            xml_path = Path(tmpdir) / "test.xml"
            xml_path.write_text(_JUNIT_XML)
            bep_path = Path(tmpdir) / "bep.json"
            bep_path.write_text(
                _bep_line(
                    {
                        "id": {"testResult": {"label": "//pkg/foo:foo_test", "configuration": {"id": "cfg1"}}},
                        "testResult": {"testActionOutput": [{"name": "test.xml", "uri": xml_path.as_uri()}]},
                    }
                )
            )

            with self.assertRaisesRegex(RuntimeError, "did not include .*test.log"):
                _parse_bep(bep_path)


if __name__ == "__main__":
    unittest.main()
