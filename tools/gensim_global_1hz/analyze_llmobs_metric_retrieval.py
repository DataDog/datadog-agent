#!/usr/bin/env python3
"""Derive deterministic metric-retrieval records from a Bits LLMObs trace export."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any, Sequence


METRIC_TOOL_NAME = "get_datadog_metric"
RESOLUTION_LANGUAGE = re.compile(
    r"\b(?:per[- ]second|1[- ]?second|one[- ]second|10[- ]?second|ten[- ]second|interval|resolution|point(?:s)?)\b",
    re.IGNORECASE,
)


@dataclass(frozen=True)
class ClassificationRule:
    pattern: str
    classification: str
    reason: str

    def matches(self, expression: str) -> bool:
        return re.search(self.pattern, expression) is not None


class ClassificationPolicy:
    def __init__(self, rules: Sequence[ClassificationRule], default: str = "unknown") -> None:
        self.rules = tuple(rules)
        self.default = default

    def classify(self, expression: str) -> tuple[str, str | None]:
        for rule in self.rules:
            if rule.matches(expression):
                return rule.classification, rule.reason
        return self.default, None

    @classmethod
    def from_dict(cls, value: dict[str, Any]) -> "ClassificationPolicy":
        rules = [
            ClassificationRule(
                pattern=rule["pattern"],
                classification=rule["classification"],
                reason=rule.get("reason", ""),
            )
            for rule in value.get("rules", [])
        ]
        return cls(rules, default=value.get("default", "unknown"))


def _unwrap(value: Any) -> Any:
    if isinstance(value, dict) and "value" in value:
        return value["value"]
    return value


def _parse_iso8601(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def _format_iso8601(value: datetime) -> str:
    utc = value.astimezone(timezone.utc)
    text = utc.isoformat(timespec="milliseconds")
    if text.endswith(".000+00:00"):
        text = text[: -len(".000+00:00")] + "Z"
    elif text.endswith("+00:00"):
        text = text[: -len("+00:00")] + "Z"
    return text


def _window_ms(request: dict[str, Any]) -> int | None:
    try:
        start = _parse_iso8601(request["from"])
        end = _parse_iso8601(request["to"])
    except (KeyError, TypeError, ValueError):
        return None
    return int((end - start).total_seconds() * 1000)


def _json_data(output: str) -> tuple[Any | None, str | None]:
    match = re.search(r"<JSON_DATA>\s*(.*?)\s*</JSON_DATA>", output, re.DOTALL)
    if not match:
        return None, "missing JSON_DATA section"
    payload = match.group(1).strip()
    if not payload:
        return [], None
    try:
        return json.loads(payload), None
    except json.JSONDecodeError as error:
        return None, f"invalid JSON_DATA: {error.msg}"


def _metric_name(expression: str) -> str:
    body = expression.split(":", 1)[1] if ":" in expression else expression
    return re.split(r"[\{\s]", body, maxsplit=1)[0]


def _diagnostic_use(expression: str, call_start_ns: int, spans: Sequence[dict[str, Any]]) -> str:
    metric_name = _metric_name(expression)
    if not metric_name:
        return "not_detected"
    metric_mentioned = False
    resolution_mentioned = False
    for span in spans:
        attributes = span.get("attributes") or {}
        try:
            start_ns = int(attributes.get("start_ns") or 0)
        except (TypeError, ValueError):
            start_ns = 0
        if start_ns <= call_start_ns:
            continue
        output = _unwrap(attributes.get("output"))
        if not isinstance(output, str) or metric_name not in output:
            continue
        metric_mentioned = True
        if RESOLUTION_LANGUAGE.search(output):
            resolution_mentioned = True
    if metric_mentioned and resolution_mentioned:
        return "metric_and_resolution_mentioned"
    if metric_mentioned:
        return "metric_mentioned"
    return "not_detected"


def _interval_override(requested: Any, effective: Any, response_format: str) -> str:
    if requested is None:
        return "not_requested"
    if response_format == "no_data":
        return "no_data"
    if effective is None:
        return "not_available"
    if effective == requested:
        return "matches_requested"
    if effective > requested:
        return "coarser_than_requested"
    return "finer_than_requested"


def _analyze_series(
    row: dict[str, Any],
    request: dict[str, Any],
    response_format: str,
    policy: ClassificationPolicy,
) -> dict[str, Any]:
    expression = str(row.get("expression") or "")
    classification, reason = policy.classify(expression)
    effective = row.get("interval_ms")
    reported_point_count = row.get("point_count")
    raw_values = row.get("values")
    values = raw_values.split(",") if isinstance(raw_values, str) and raw_values else []
    null_count = sum(value.strip().lower() in {"", "null", "nan"} for value in values)
    value_count = len(values)
    window_ms = _window_ms(request)
    expected = None
    if isinstance(window_ms, int) and isinstance(effective, int) and effective > 0:
        expected = window_ms // effective
    structural_gaps = None if expected is None else max(expected - value_count, 0)

    start_time = row.get("start_time")
    last_point_time = None
    if isinstance(start_time, str) and isinstance(effective, int) and value_count:
        try:
            last_point_time = _format_iso8601(
                _parse_iso8601(start_time) + timedelta(milliseconds=effective * (value_count - 1))
            )
        except ValueError:
            pass

    binned = row.get("binned")
    bin_count = len(binned) if isinstance(binned, list) else None
    genuine_one_second = (
        response_format == "raw_points"
        and effective == 1000
        and value_count >= 2
        and null_count == 0
    )
    return {
        "expression": expression,
        "source_classification": classification,
        "source_classification_reason": reason,
        "effective_interval_ms": effective,
        "reported_point_count": reported_point_count,
        "value_count": value_count if response_format == "raw_points" else None,
        "bin_count": bin_count,
        "null_point_count": null_count if response_format == "raw_points" else None,
        "expected_full_window_point_count": expected,
        "structural_gap_count": structural_gaps,
        "start_time": start_time,
        "last_point_time": last_point_time,
        "adjacent_spacing_ms": effective if response_format == "raw_points" and value_count >= 2 else None,
        "interval_override": _interval_override(request.get("interval"), effective, response_format),
        "genuine_one_second_points": genuine_one_second,
    }


def _response_format(payload: Any, output: str) -> str:
    if payload == [] and "<NO_DATA_WARNING>" in output:
        return "no_data"
    if not isinstance(payload, list):
        return "unparseable"
    if not payload:
        return "no_data"
    if any(isinstance(row, dict) and "binned" in row for row in payload):
        return "binned_statistics"
    if any(isinstance(row, dict) and "values" in row for row in payload):
        return "raw_points"
    return "unknown_json"


def analyze_trace(trace: dict[str, Any], policy: ClassificationPolicy) -> dict[str, Any]:
    spans = trace.get("data")
    if not isinstance(spans, list):
        raise ValueError("trace export must contain a data array")

    calls: list[dict[str, Any]] = []
    for span in sorted(spans, key=lambda item: int((item.get("attributes") or {}).get("start_ns") or 0)):
        attributes = span.get("attributes") or {}
        name = str(attributes.get("name") or "")
        if METRIC_TOOL_NAME not in name.lower():
            continue

        errors: list[str] = []
        request_raw = _unwrap(attributes.get("input"))
        try:
            request = json.loads(request_raw) if isinstance(request_raw, str) else dict(request_raw or {})
        except (json.JSONDecodeError, TypeError, ValueError) as error:
            request = {}
            errors.append(f"invalid tool input: {error}")

        output = _unwrap(attributes.get("output"))
        if not isinstance(output, str):
            output = ""
            errors.append("tool output is not text")
        payload, payload_error = _json_data(output)
        if payload_error:
            errors.append(payload_error)
        response_format = _response_format(payload, output)
        if response_format == "unparseable" and payload_error is None:
            errors.append("JSON_DATA is not an array")

        rows = [row for row in payload if isinstance(row, dict)] if isinstance(payload, list) else []
        series = [_analyze_series(row, request, response_format, policy) for row in rows]
        queries = request.get("queries") if isinstance(request.get("queries"), list) else []
        expression_for_use = series[0]["expression"] if series else (str(queries[0]) if queries else "")
        try:
            call_start_ns = int(attributes.get("start_ns") or 0)
        except (TypeError, ValueError):
            call_start_ns = 0

        calls.append(
            {
                "call_index": len(calls) + 1,
                "span_id": attributes.get("span_id"),
                "status": attributes.get("status"),
                "request": {
                    "queries": queries,
                    "from": request.get("from"),
                    "to": request.get("to"),
                    "window_ms": _window_ms(request),
                    "interval_ms": request.get("interval"),
                    "raw_data": request.get("raw_data"),
                    "response_format": request.get("response_format"),
                    "max_tokens": request.get("max_tokens"),
                },
                "response": {
                    "format": response_format,
                    "no_data": response_format == "no_data",
                    "truncated": bool(re.search(r"<is_truncated>\s*true\s*</is_truncated>", output, re.IGNORECASE)),
                    "has_no_data_warning": "<NO_DATA_WARNING>" in output,
                },
                "series": series,
                "diagnostic_use": _diagnostic_use(expression_for_use, call_start_ns, spans),
                "errors": errors,
            }
        )

    return {
        "schema_version": 1,
        "trace_id": trace.get("trace_id"),
        "span_count": len(spans),
        "metric_call_count": len(calls),
        "calls": calls,
        "summary": {
            "requests_interval_1000": sum(call["request"]["interval_ms"] == 1000 for call in calls),
            "requests_raw_data_true": sum(call["request"]["raw_data"] is True for call in calls),
            "genuine_one_second_series": sum(
                series["genuine_one_second_points"] for call in calls for series in call["series"]
            ),
            "coarser_than_requested_series": sum(
                series["interval_override"] == "coarser_than_requested"
                for call in calls
                for series in call["series"]
            ),
            "no_data_calls": sum(call["response"]["no_data"] for call in calls),
            "truncated_calls": sum(call["response"]["truncated"] for call in calls),
            "calls_with_errors": sum(bool(call["errors"]) for call in calls),
        },
    }


def _load_policy(path: Path | None) -> ClassificationPolicy:
    if path is None:
        return ClassificationPolicy([])
    return ClassificationPolicy.from_dict(json.loads(path.read_text()))


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("trace", type=Path, help="LLMObs trace export JSON")
    parser.add_argument("--policy", type=Path, help="Optional regex-based source classification policy JSON")
    parser.add_argument("--output", type=Path, help="Write analysis JSON here; stdout when omitted")
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    args = _build_parser().parse_args(argv)
    raw = args.trace.read_bytes()
    trace = json.loads(raw)
    result = analyze_trace(trace, _load_policy(args.policy))
    result["source"] = {
        "path": str(args.trace.resolve()),
        "sha256": hashlib.sha256(raw).hexdigest(),
    }
    rendered = json.dumps(result, indent=2, sort_keys=True) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(rendered)
    else:
        sys.stdout.write(rendered)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
