import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import analyze_llmobs_metric_retrieval as analyzer


def span(name, start_ns, input_value=None, output_value=None, span_id="span"):
    attributes = {
        "name": name,
        "start_ns": str(start_ns),
        "span_id": span_id,
        "trace_id": "trace",
        "status": "ok",
    }
    if input_value is not None:
        attributes["input"] = {"value": json.dumps(input_value)}
    if output_value is not None:
        attributes["output"] = {"value": output_value}
    return {"attributes": attributes}


def metric_output(payload, metadata=""):
    return f"<METADATA>{metadata}</METADATA>\n<JSON_DATA>{payload}</JSON_DATA>"


class AnalyzeTraceTest(unittest.TestCase):
    def test_binned_response_preserves_omitted_request_fields(self):
        trace = {
            "trace_id": "trace",
            "data": [
                span(
                    "mcp_datadog_get_datadog_metric",
                    1,
                    {
                        "from": "2026-08-16T17:00:00Z",
                        "to": "2026-08-16T17:10:00Z",
                        "queries": ["sum:example.hits{env:test}.as_count()"],
                    },
                    metric_output(
                        json.dumps(
                            [
                                {
                                    "expression": "sum:example.hits{env:test}.as_count()",
                                    "binned": [{"start": 1, "end": 2, "value": 3}],
                                    "overall_stats": {"min": 0, "max": 3},
                                    "time_range": {"from": 1, "to": 2},
                                    "scope": "*",
                                }
                            ]
                        )
                    ),
                )
            ],
        }

        result = analyzer.analyze_trace(trace, analyzer.ClassificationPolicy([]))

        self.assertEqual(1, result["metric_call_count"])
        call = result["calls"][0]
        self.assertIsNone(call["request"]["interval_ms"])
        self.assertIsNone(call["request"]["raw_data"])
        self.assertEqual("binned_statistics", call["response"]["format"])
        self.assertEqual("not_requested", call["series"][0]["interval_override"])
        self.assertFalse(call["series"][0]["genuine_one_second_points"])

    def test_raw_series_derives_spacing_completeness_and_nulls(self):
        query = "sum:notification_router.job.enqueue.hits{env:test}.as_count()"
        trace = {
            "trace_id": "trace",
            "data": [
                span(
                    "mcp_datadog_get_datadog_metric",
                    1,
                    {
                        "from": "2026-08-16T17:00:00Z",
                        "to": "2026-08-16T17:00:05Z",
                        "queries": [query],
                        "interval": 1000,
                        "raw_data": True,
                    },
                    metric_output(
                        json.dumps(
                            [
                                {
                                    "expression": query,
                                    "values": "1,2,0,3,4",
                                    "start_time": "2026-08-16T17:00:00Z",
                                    "interval_ms": 1000,
                                    "point_count": 5,
                                    "scope": "*",
                                }
                            ]
                        )
                    ),
                )
            ],
        }
        policy = analyzer.ClassificationPolicy(
            [analyzer.ClassificationRule("notification_router\\.job", "eligible_agent_owned", "qualified")]
        )

        call = analyzer.analyze_trace(trace, policy)["calls"][0]
        series = call["series"][0]

        self.assertEqual("raw_points", call["response"]["format"])
        self.assertEqual("eligible_agent_owned", series["source_classification"])
        self.assertEqual("matches_requested", series["interval_override"])
        self.assertEqual(1000, series["effective_interval_ms"])
        self.assertEqual(5, series["value_count"])
        self.assertEqual(0, series["null_point_count"])
        self.assertEqual(5, series["expected_full_window_point_count"])
        self.assertEqual(0, series["structural_gap_count"])
        self.assertEqual("2026-08-16T17:00:04Z", series["last_point_time"])
        self.assertTrue(series["genuine_one_second_points"])

    def test_coarser_effective_interval_and_reasoning_use_are_reported(self):
        query = "sum:trace.flask.request.hits{service:router}.as_count()"
        trace = {
            "trace_id": "trace",
            "data": [
                span(
                    "mcp_datadog_get_datadog_metric",
                    10,
                    {
                        "from": "2026-08-16T17:00:00Z",
                        "to": "2026-08-16T17:00:30Z",
                        "queries": [query],
                        "interval": 1000,
                        "raw_data": True,
                    },
                    metric_output(
                        json.dumps(
                            [
                                {
                                    "expression": query,
                                    "values": "7,8,9",
                                    "start_time": "2026-08-16T17:00:00Z",
                                    "interval_ms": 10000,
                                    "point_count": 3,
                                    "scope": "*",
                                }
                            ]
                        )
                    ),
                ),
                span(
                    "react_run_next",
                    20,
                    output_value="trace.flask.request.hits returned three points at 10 second intervals.",
                ),
            ],
        }
        policy = analyzer.ClassificationPolicy(
            [analyzer.ClassificationRule(r"(^|:)trace\.", "excluded_trace_stat", "not Agent-owned")]
        )

        call = analyzer.analyze_trace(trace, policy)["calls"][0]
        series = call["series"][0]

        self.assertEqual("coarser_than_requested", series["interval_override"])
        self.assertEqual("excluded_trace_stat", series["source_classification"])
        self.assertFalse(series["genuine_one_second_points"])
        self.assertEqual("metric_and_resolution_mentioned", call["diagnostic_use"])

    def test_empty_json_data_is_no_data_not_a_parse_failure(self):
        query = "sum:missing.metric{*}"
        trace = {
            "trace_id": "trace",
            "data": [
                span(
                    "mcp_datadog_get_datadog_metric",
                    1,
                    {
                        "from": "2026-08-16T17:00:00Z",
                        "to": "2026-08-16T17:05:00Z",
                        "queries": [query],
                        "interval": 1000,
                        "raw_data": True,
                    },
                    "<NO_DATA_WARNING>No matching data.</NO_DATA_WARNING><JSON_DATA></JSON_DATA>",
                )
            ],
        }

        call = analyzer.analyze_trace(trace, analyzer.ClassificationPolicy([]))["calls"][0]

        self.assertEqual("no_data", call["response"]["format"])
        self.assertTrue(call["response"]["no_data"])
        self.assertEqual([], call["series"])
        self.assertEqual([], call["errors"])

    def test_malformed_tool_input_is_preserved_as_an_error(self):
        trace = {
            "trace_id": "trace",
            "data": [
                {
                    "attributes": {
                        "name": "mcp_datadog_get_datadog_metric",
                        "start_ns": "1",
                        "input": {"value": "not-json"},
                        "output": {"value": "not-marked-up"},
                    }
                }
            ],
        }

        call = analyzer.analyze_trace(trace, analyzer.ClassificationPolicy([]))["calls"][0]

        self.assertEqual("unparseable", call["response"]["format"])
        self.assertGreaterEqual(len(call["errors"]), 2)


class CliTest(unittest.TestCase):
    def test_cli_writes_json_output(self):
        trace = {"trace_id": "empty", "data": []}
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "trace.json"
            output = Path(directory) / "analysis.json"
            source.write_text(json.dumps(trace))

            exit_code = analyzer.main([str(source), "--output", str(output)])

            self.assertEqual(0, exit_code)
            saved = json.loads(output.read_text())
            self.assertEqual("empty", saved["trace_id"])
            self.assertEqual(0, saved["metric_call_count"])


if __name__ == "__main__":
    unittest.main()
