import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from tasks.bazel import (
    _IMPORT_PREFIX,
    _bazel_test_funcs_from_bep,
    _go_test_packages,
    _label_to_import_path,
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


def _bep_line(event: dict) -> str:
    return json.dumps(event) + "\n"


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
    def test_empty_import_paths_skips_subprocess(self):
        with patch("tasks.bazel.subprocess.run") as mock_run:
            self.assertEqual(_go_test_packages(["race"], set()), {})
            mock_run.assert_not_called()

    def test_nonzero_returncode_raises(self):
        with patch("tasks.bazel.subprocess.run") as mock_run:
            mock_run.return_value = subprocess.CompletedProcess(args=[], returncode=1, stdout="", stderr="boom")
            with self.assertRaises(ChildProcessError):
                _go_test_packages(["race"], {"example.com/pkg/foo"})

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

            with patch("tasks.bazel.subprocess.run") as mock_run:
                mock_run.return_value = subprocess.CompletedProcess(args=[], returncode=0, stdout=stdout, stderr="")
                result = _go_test_packages(["race"], {"example.com/pkg/foo"})

            # "example.com/pkg/bar" is filtered out: not in import_paths.
            # TestMain is discarded even though it matched the Test* pattern.
            self.assertEqual(result, {"example.com/pkg/foo": {"TestFoo"}})


if __name__ == "__main__":
    unittest.main()
