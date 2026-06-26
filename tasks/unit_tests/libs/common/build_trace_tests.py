import json
import os
import tempfile
import unittest
from unittest.mock import patch

from tasks.libs.common import build_trace


class TestBuildTrace(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.env = patch.dict(
            os.environ,
            {
                "CI_PROJECT_DIR": self.tmp.name,
                "CI_JOB_NAME_SLUG": "datadog-agent-7-x64",
                "BUILD_TRACE_SEGMENT": "omnibus",
                "PACKAGE_ARCH": "amd64",
            },
            clear=False,
        )
        self.env.start()
        self.addCleanup(self.env.stop)

    def _fragment(self):
        with open(build_trace._fragment_path()) as f:
            return json.load(f)

    def test_records_spans_with_metadata(self):
        build_trace.record_span("bundle-install", 28.0)
        build_trace.record_span("python", 210.345678, cached=False)
        build_trace.record_span("openssl", 35.1, cached=True, meta={"size": 10})

        frag = self._fragment()
        self.assertEqual(frag["schema_version"], build_trace.SCHEMA_VERSION)
        self.assertEqual(frag["job"], "datadog-agent-7-x64")
        self.assertEqual(frag["segment"], "omnibus")
        self.assertEqual(frag["arch"], "amd64")
        self.assertEqual([s["name"] for s in frag["spans"]], ["bundle-install", "python", "openssl"])
        # duration is rounded to ms precision
        self.assertEqual(frag["spans"][1]["duration_s"], 210.346)
        # cached only emitted when set
        self.assertNotIn("cached", frag["spans"][0])
        self.assertIs(frag["spans"][1]["cached"], False)
        self.assertIs(frag["spans"][2]["cached"], True)
        self.assertEqual(frag["spans"][2]["meta"], {"size": 10})

    def test_spans_accumulate_across_calls(self):
        """Successive `dda inv` invocations in the same job append, not overwrite."""
        build_trace.record_span("a", 1.0)
        build_trace.record_span("b", 2.0)
        self.assertEqual([s["name"] for s in self._fragment()["spans"]], ["a", "b"])

    def test_trace_span_context_manager(self):
        with build_trace.trace_span("work"):
            pass
        spans = self._fragment()["spans"]
        self.assertEqual(spans[0]["name"], "work")
        self.assertGreaterEqual(spans[0]["duration_s"], 0.0)

    def test_recovers_from_corrupt_fragment(self):
        os.makedirs(os.path.dirname(build_trace._fragment_path()), exist_ok=True)
        with open(build_trace._fragment_path(), "w") as f:
            f.write("{not valid json")
        build_trace.record_span("recovered", 1.0)
        self.assertEqual([s["name"] for s in self._fragment()["spans"]], ["recovered"])

    def test_never_raises(self):
        """A tracing failure must not break the build it measures."""
        with patch("tasks.libs.common.build_trace._fragment_path", side_effect=OSError("boom")):
            # Should swallow the error rather than propagate.
            build_trace.record_span("x", 1.0)


if __name__ == "__main__":
    unittest.main()
